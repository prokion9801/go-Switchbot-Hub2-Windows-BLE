# go-Switchbot-Hub2-Windows-BLE

Windows 11 ネイティブ環境で動作する、SwitchBot ハブ2 専用の軽量タスクトレイ常駐型温湿度モニター ＆ GAS Webhook 連携クライアントです。
BLE（Bluetooth Low Energy）のアドバタイズパケットを直接受信・解析し、本体液晶には表示されない**0.1℃単位の正確な温度**・**相対湿度**・**容積絶対湿度（g/m³）**をリアルタイムで監視するとともに、変化検知時に Google Apps Script（GAS）へ即時 POST 送信します。

## 主な特徴

* **CGO不要・Pure Go実装**: `tinygo.org/x/bluetooth` と `github.com/getlantern/systray` を使用し、Windows WinRT API 経由で単体バイナリ（`.exe`）として動作します。
* **高精度なパケット解析**: SwitchBot 固有の Manufacturer Data (`0x0969`) から整数部・小数点部・符号を正確に抽出し、Tetens の式を用いて容積絶対湿度を算出します。
* **GAS Webhook 即時連携（ハイブリッド制御対応）**: アプリ起動直後および温度・湿度に変化があったタイミングで、GAS の `doPost` エンドポイントへ非同期で JSON データを POST 送信します。クラウド経由の遅延（約5分）を回避し、エアコン自動制御等の応答性を大幅に向上させます。
* **重複起動防止（Single Instance）**: Windows API（`CreateMutexW`）による排他制御を実装しており、スタートアップ失敗時の手動再起動などによる二重起動や重複 POST 送信を自動的にブロックします。
* **セキュアな設定管理**: 機器固有の MAC アドレス、GAS Webhook URL、認証トークンはソースコードに一切記述せず、すべて環境変数から動的に読み込みます。

## 動作環境

* OS: Windows 10 / Windows 11 (64bit)
* Bluetooth: BLE 対応アダプター（PC内蔵またはUSBドングル）
* Go: 1.21 以上（ソースからビルドする場合）

## 使い方

### 1. 必須環境変数の設定

本アプリを起動するには、以下の **3つの環境変数** がすべて設定されている必要があります（未設定の場合はエラー終了します）。

| 環境変数名 | 説明 | 設定例 |
| :--- | :--- | :--- |
| `SWITCHBOT_MAC` | 監視対象とする SwitchBot ハブ2 の MAC アドレス | `XX:XX:XX:XX:XX:XX` |
| `GAS_WEBHOOK_URL` | 送信先となる Google Apps Script ウェブアプリの URL | `https://script.google.com/macros/s/.../exec` |
| `BLE_AUTH_TOKEN` | GAS 側で送信元を検証するための認証トークン | `your_secret_token` |

**Windows ユーザー環境変数に永続登録する場合（.exe 常駐・スタートアップ用）:**
```powershell
[System.Environment]::SetEnvironmentVariable("SWITCHBOT_MAC", "XX:XX:XX:XX:XX:XX", "User")
[System.Environment]::SetEnvironmentVariable("GAS_WEBHOOK_URL", "[https://script.google.com/macros/s/.../exec](https://script.google.com/macros/s/.../exec)", "User")
[System.Environment]::SetEnvironmentVariable("BLE_AUTH_TOKEN", "your_secret_token", "User")
