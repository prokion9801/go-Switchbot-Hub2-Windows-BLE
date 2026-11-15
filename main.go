package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/getlantern/systray"
	"tinygo.org/x/bluetooth"
)

const (
	switchBotCompanyID = 0x0969
	mutexName          = "Local\\SwitchBotHub2WindowsBLETrayMutex"
	errorAlreadyExists = 183 // Windows API: ERROR_ALREADY_EXISTS
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	procCloseHandle = kernel32.NewProc("CloseHandle")

	adapter = bluetooth.DefaultAdapter

	// 環境変数から読み込む設定値（機密情報）
	targetMAC     string
	gasWebhookURL string
	bleAuthToken  string
)

// SwitchBot Webhook互換＋ローカル識別子付きのJSON構造体
type BleWebhookPayload struct {
	EventType string `json:"eventType"`
	Source    string `json:"source"`
	Token     string `json:"token"`
	Context   struct {
		DeviceMac    string  `json:"deviceMac"`
		Temperature  float64 `json:"temperature"`
		Humidity     int     `json:"humidity"`
		AbsHumidity  float64 `json:"absHumidity"`
		TimeOfSample int64   `json:"timeOfSample"`
	} `json:"context"`
}

func main() {
	// 1. CreateMutexW による重複起動チェック
	mutexHandle, alreadyRunning, err := acquireSingleInstanceMutex(mutexName)
	if err != nil {
		log.Fatalf("ミューテックスの作成に失敗しました: %v", err)
	}
	if alreadyRunning {
		fmt.Println("すでに別のインスタンスが起動しています。重複起動を防止して終了します。")
		os.Exit(0)
	}
	defer procCloseHandle.Call(uintptr(mutexHandle))

	// 2. 必須環境変数のチェック（未設定時は強制終了）
	targetMAC = strings.TrimSpace(os.Getenv("SWITCHBOT_MAC"))
	gasWebhookURL = strings.TrimSpace(os.Getenv("GAS_WEBHOOK_URL"))
	bleAuthToken = strings.TrimSpace(os.Getenv("BLE_AUTH_TOKEN"))

	if targetMAC == "" || gasWebhookURL == "" || bleAuthToken == "" {
		log.Fatal("必須環境変数 (SWITCHBOT_MAC, GAS_WEBHOOK_URL, BLE_AUTH_TOKEN) のいずれかが設定されていません。終了します。")
	}

	// 3. 終了シグナル（Ctrl+C等）のハンドリング
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n終了シグナルを受信しました。スキャンを停止して終了します...")
		_ = adapter.StopScan()
		os.Exit(0)
	}()

	// 4. タスクトレイ常駐開始
	systray.Run(onReady, onExit)
}

// acquireSingleInstanceMutex は Windows API の CreateMutexW を呼び出して二重起動を検知する
func acquireSingleInstanceMutex(name string) (syscall.Handle, bool, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, false, err
	}

	ret, _, callErr := procCreateMutex.Call(
		0,
		0,
		uintptr(unsafe.Pointer(namePtr)),
	)
	if ret == 0 {
		return 0, false, callErr
	}

	if callErr == syscall.Errno(errorAlreadyExists) {
		_, _, _ = procCloseHandle.Call(ret)
		return 0, true, nil
	}

	return syscall.Handle(ret), false, nil
}

func onReady() {
	systray.SetTitle("SwitchBot Hub2")
	systray.SetTooltip("SwitchBot ハブ2 監視中...")

	mStatus := systray.AddMenuItem("温湿度データ待機中...", "最新の温湿度")
	mStatus.Disable()

	mGasStatus := systray.AddMenuItem("GAS送信: 待機中...", "最終GAS送信ステータス")
	mGasStatus.Disable()

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("終了", "アプリを終了します")

	go startBLEScanner(mStatus, mGasStatus)

	go func() {
		for range mQuit.ClickedCh {
			fmt.Println("トレイメニューから終了が選択されました。")
			_ = adapter.StopScan()
			systray.Quit()
			os.Exit(0)
		}
	}()
}

func onExit() {
	_ = adapter.StopScan()
}

func startBLEScanner(mStatus, mGasStatus *systray.MenuItem) {
	fmt.Printf("[情報] 指定MACアドレス (%s) のハブ2をターゲットにスキャンします。\n", targetMAC)

	if err := adapter.Enable(); err != nil {
		log.Fatalf("Bluetoothアダプターの有効化に失敗しました: %v", err)
	}

	// 前回記録した値（初期値 -999 / -1 により、立ち上がり初回パケットは必ず「変化あり」と判定される）
	var lastTemp float64 = -999
	var lastHum int = -1

	for {
		err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
			addr := device.Address.String()
			if !strings.EqualFold(addr, targetMAC) {
				return
			}

			for _, mfg := range device.ManufacturerData() {
				if mfg.CompanyID == switchBotCompanyID {
					temp, hum, ok := parseHub2Data(mfg.Data)
					if !ok {
						continue
					}

					// 立ち上がり初回、または温度・湿度のいずれかが変化した場合のみ実行
					if temp != lastTemp || hum != lastHum {
						lastTemp = temp
						lastHum = hum

						absHum := calcAbsoluteHumidity(temp, float64(hum))
						now := time.Now().Format("15:04:05")

						// ツールチップとメニュー表示を更新
						tooltipText := fmt.Sprintf("室温: %.1f℃ / 湿度: %d%% / 絶対湿度: %.2f g/m³ (%s)", temp, hum, absHum, now)
						menuText := fmt.Sprintf("🌡️ %.1f℃ | 💧 %d%% | ☁️ %.2f g/m³ (%s)", temp, hum, absHum, now)

						systray.SetTooltip(tooltipText)
						mStatus.SetTitle(menuText)

						fmt.Printf("[%s] 温度: %.1f ℃ | 湿度: %d %% | 絶対湿度: %.2f g/m³ (RSSI: %d)\n",
							now, temp, hum, absHum, device.RSSI)

						// GASへ非同期POST送信（BLEスキャンループをブロックしないようgoroutineで実行）
						go func(macAddr string, t float64, h int, ah float64, ts string) {
							if err := sendToGAS(macAddr, t, h, ah); err != nil {
								fmt.Printf("[%s] [GAS送信失敗] %v\n", ts, err)
								mGasStatus.SetTitle(fmt.Sprintf("GAS送信: 失敗 (%s)", ts))
							} else {
								mGasStatus.SetTitle(fmt.Sprintf("GAS送信: 完了 (%s)", ts))
							}
						}(addr, temp, hum, absHum, now)
					}
				}
			}
		})

		if err != nil {
			fmt.Printf("スキャンエラー (再試行します): %v\n", err)
			time.Sleep(3 * time.Second)
		}
	}
}

func sendToGAS(mac string, temp float64, hum int, absHum float64) error {
	payload := BleWebhookPayload{
		EventType: "changeReport",
		Source:    "win_ble",
		Token:     bleAuthToken,
	}
	payload.Context.DeviceMac = mac
	payload.Context.Temperature = temp
	payload.Context.Humidity = hum
	payload.Context.AbsHumidity = math.Round(absHum*100) / 100 // 小数点第2位までに丸め
	payload.Context.TimeOfSample = time.Now().UnixMilli()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON変換エラー: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("POST", gasWebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("リクエスト作成エラー: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP送信エラー: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("[GAS送信完了] Status: %s | 応答: %s\n", resp.Status, string(body))
	return nil
}

func parseHub2Data(data []byte) (float64, int, bool) {
	if len(data) < 16 {
		return 0, 0, false
	}

	decInt := int(data[13] & 0x0F)
	intPart := int(data[14] & 0x7F)
	tempTenths := intPart*10 + decInt

	if (data[14] & 0x80) == 0 {
		tempTenths = -tempTenths
	}

	temp := float64(tempTenths) / 10.0
	hum := int(data[15] & 0x7F)

	if temp < -40.0 || temp > 80.0 || hum < 0 || hum > 100 {
		return 0, 0, false
	}

	return temp, hum, true
}

func calcAbsoluteHumidity(temp, relHum float64) float64 {
	svp := 6.1078 * math.Pow(10, (7.5*temp)/(temp+237.3))
	vp := svp * (relHum / 100.0)
	return (216.7 * vp) / (temp + 273.15)
}
