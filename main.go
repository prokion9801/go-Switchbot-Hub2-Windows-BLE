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

	"tinygo.org/x/bluetooth"
)

const switchBotCompanyID = 0x0969

var adapter = bluetooth.DefaultAdapter

func main() {
	// 環境変数からターゲットMACアドレスを取得
	targetMAC := os.Getenv("SWITCHBOT_MAC")
	if targetMAC == "" {
		fmt.Println("[警告] 環境変数 SWITCHBOT_MAC が設定されていません。周辺の全 SwitchBot 機器をスキャンします。")
	} else {
		fmt.Printf("[情報] 指定MACアドレス (%s) のハブ2をターゲットにスキャンします。\n", targetMAC)
	}

	if err := adapter.Enable(); err != nil {
		log.Fatalf("Bluetoothアダプターの有効化に失敗しました: %v", err)
	}

	// Ctrl+C (SIGINT) を捕捉して安全にスキャン停止する処理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nスキャンを停止して終了します...")
		_ = adapter.StopScan()
		os.Exit(0)
	}()

	fmt.Println("SwitchBot BLEスキャンを開始します... (終了: Ctrl+C)")

	var lastTemp float64 = -999
	var lastHum int = -1

	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		addr := device.Address.String()

		// ターゲットMACが指定されている場合はフィルタリング（大文字小文字を区別しない）
		if targetMAC != "" && !strings.EqualFold(addr, targetMAC) {
			return
		}

		for _, mfg := range device.ManufacturerData() {
			if mfg.CompanyID == switchBotCompanyID {
				temp, hum, ok := parseHub2Data(mfg.Data)
				if !ok {
					continue
				}

				if temp != lastTemp || hum != lastHum {
					lastTemp = temp
					lastHum = hum
					absHum := calcAbsoluteHumidity(temp, float64(hum))
					now := time.Now().Format("2006-01-02 15:04:05")

					fmt.Printf("[%s] MAC: %s | 温度: %.1f ℃ | 相対湿度: %d %% | 絶対湿度: %.2f g/m³ (RSSI: %d) [Raw12..15: %X]\n",
						now, addr, temp, hum, absHum, device.RSSI, mfg.Data[12:16])
				}
			}
		}
	})

	if err != nil {
		log.Fatalf("スキャンエラー: %v", err)
	}
}

// パース処理（data[13] 小数点部、data[14] 整数部＆符号、data[15] 湿度）
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
