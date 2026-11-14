package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"tinygo.org/x/bluetooth"
)

const switchBotCompanyID = 0x0969

var adapter = bluetooth.DefaultAdapter

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n終了シグナルを受信しました。終了します...")
		_ = adapter.StopScan()
		os.Exit(0)
	}()

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("SwitchBot Hub2")
	systray.SetTooltip("SwitchBot ハブ2 監視中...")

	mStatus := systray.AddMenuItem("温湿度データ待機中...", "最新の温湿度")
	mStatus.Disable()

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("終了", "アプリを終了します")

	go startBLEScanner(mStatus)

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

func startBLEScanner(mStatus *systray.MenuItem) {
	targetMAC := os.Getenv("SWITCHBOT_MAC")
	if targetMAC == "" {
		fmt.Println("[警告] 環境変数 SWITCHBOT_MAC が設定されていません。周辺の全 SwitchBot 機器をスキャンします。")
	} else {
		fmt.Printf("[情報] 指定MACアドレス (%s) のハブ2をターゲットにスキャンします。\n", targetMAC)
	}

	if err := adapter.Enable(); err != nil {
		log.Fatalf("Bluetoothアダプターの有効化に失敗しました: %v", err)
	}

	// 前回記録した値を保持（変化検知用）
	var lastTemp float64 = -999
	var lastHum int = -1

	for {
		err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
			addr := device.Address.String()
			if targetMAC != "" && !strings.EqualFold(addr, targetMAC) {
				return
			}

			for _, mfg := range device.ManufacturerData() {
				if mfg.CompanyID == switchBotCompanyID {
					temp, hum, ok := parseHub2Data(mfg.Data)
					if !ok {
						continue
					}

					// 温度または湿度が変化した場合のみ処理を実行
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
