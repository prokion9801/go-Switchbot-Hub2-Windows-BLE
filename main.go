package main

import (
	"fmt"
	"log"
	"strings"

	"tinygo.org/x/bluetooth"
)

// SwitchBot (Woan Technology) の Bluetooth Company ID
const switchBotCompanyID = 0x0969

// 特定のハブ2だけに絞りたい場合はMACアドレスを指定（空文字なら全SwitchBot機器を表示）
// 例: "AA:BB:CC:DD:EE:FF"
const targetMAC = ""

var adapter = bluetooth.DefaultAdapter

func main() {
	// WindowsのBluetoothアダプターを有効化
	if err := adapter.Enable(); err != nil {
		log.Fatalf("Bluetoothアダプターの有効化に失敗しました: %v", err)
	}

	fmt.Println("SwitchBot ハブ2 のBLEスキャンを開始します... (終了するには Ctrl+C)")

	// BLEアドバタイズパケットのスキャン開始（ブロッキング処理）
	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		addr := device.Address.String()

		// ターゲットMACが指定されている場合はフィルタリング
		if targetMAC != "" && !strings.EqualFold(addr, targetMAC) {
			return
		}

		// Manufacturer Data から SwitchBot (0x0969) のデータを探す
		for _, mfg := range device.ManufacturerData() {
			if mfg.CompanyID == switchBotCompanyID {
				temp, hum, ok := parseHub2ManufacturerData(mfg.Data)
				if ok {
					fmt.Printf("[受信] MAC: %s | RSSI: %d dBm | 温度: %.1f ℃ | 湿度: %d %%\n",
						addr, device.RSSI, temp, hum)
				} else {
					// パース対象外（他機種や別フォーマット）のデバッグ確認用
					fmt.Printf("[別パケット] MAC: %s | RawData: %X\n", addr, mfg.Data)
				}
			}
		}
	})

	if err != nil {
		log.Fatalf("スキャンエラー: %v", err)
	}
}

// parseHub2ManufacturerData は SwitchBot Hub 2 の Manufacturer Data から温度と湿度を抽出します
// Hub 2 の Manufacturer Data (0x0969) の一般的なバイト構造:
// Index 0..5: デバイスMACアドレス (6バイト)
// Index 6..7: シーケンス番号・フラグ等
// Index 8: 温度の小数点部 (下位4ビット × 0.1)
// Index 9: 温度の整数部 (下位7ビット) + プラス/マイナス符号 (最上位ビット: 1ならプラス, 0ならマイナス)
// Index 10: 湿度 (下位7ビット)
func parseHub2ManufacturerData(data []byte) (float64, int, bool) {
	if len(data) < 11 {
		return 0, 0, false
	}

	// 温度の計算
	decimal := float64(data[8]&0x0F) * 0.1
	integer := float64(data[9] & 0x7F)
	temp := integer + decimal

	// 最上位ビット(0x80)が0の場合は氷点下（マイナス）
	if (data[9] & 0x80) == 0 {
		temp = -temp
	}

	// 湿度の計算 (下位7ビット)
	hum := int(data[10] & 0x7F)

	// 異常値フィルタ（温湿度計以外のパケット誤爆防止）
	if temp < -40.0 || temp > 80.0 || hum < 0 || hum > 100 {
		return 0, 0, false
	}

	return temp, hum, true
}
