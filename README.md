# go-Switchbot-Hub2-Windows-BLE

Windows 11 ネイティブ環境で動作する、SwitchBot ハブ2 専用の軽量タスクトレイ常駐型温湿度モニターです。
BLE（Bluetooth Low Energy）のアドバタイズパケットを直接受信・解析し、本体液晶には表示されない**0.1℃単位の正確な温度**・**相対湿度**・**容積絶対湿度（g/m³）**をリアルタイムで監視します。

## 主な特徴

* **CGO不要・Pure Go実装**: `tinygo.org/x/bluetooth` と `github.com/getlantern/systray` を使用し、Windows WinRT API 経由で単体バイナリ（`.exe`）として動作します。
* **高精度なパケット解析**: SwitchBot 固有の Manufacturer Data (`0x0969`) から整数部・小数点部・符号を正確に抽出し、Tetens の式を用いて容積絶対湿度を算出します。
* **省リソース設計**: 1秒間に複数回送信されるアドバタイズパケットのうち、温度または湿度に変化があったタイミングのみ画面表示・ツールチップを更新します。
* **セキュアな設定管理**: 機器固有の MAC アドレスはソースコードに記述せず、環境変数 `SWITCHBOT_MAC` から動的に読み込みます。

## 動作環境

* OS: Windows 10 / Windows 11 (64bit)
* Bluetooth: BLE 対応アダプター（PC内蔵またはUSBドングル）
* Go: 1.21 以上（ソースからビルドする場合）

## 使い方

### 1. 環境変数の設定

監視対象とする SwitchBot ハブ2 の MAC アドレスを環境変数 `SWITCHBOT_MAC` に設定します。
（※未設定の場合は周辺の全 SwitchBot 機器をスキャンします）

**PowerShell で一時的に設定する場合:**
```powershell
$env:SWITCHBOT_MAC = "XX:XX:XX:XX:XX:XX"
