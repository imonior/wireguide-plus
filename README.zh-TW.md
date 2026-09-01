# WireGuide Plus

**一款 Windows 上的多隧道、自動化優先的 WireGuard 用戶端**

WireGuide Plus 是對開源專案 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)
進行**深度修復與增強**的版本。兩大核心能力：

- **多隧道並發** — 多條 WireGuard 隧道可同時建立、互不干擾地獨立執行；
- **條件自動連接** — 依 Wi-Fi SSID、時間段、系統啟動等條件自動連接對應隧道
  （例如辦公 Wi-Fi 下連隧道 A，家中連隧道 B）。

[English](README.md) | [简体中文](README.zh.md) | **繁體中文** | [한국어](README.ko.md) | [日本語](README.ja.md)

> **Windows 10 / 11（x64、x86 32 位元與 ARM64）完全支援**。macOS（Apple Silicon）與
> Linux（x64、arm64）提供**實驗性預覽版** — macOS 已在 Apple Silicon 實機驗證，
> Linux 經 CI 建置、尚未在實機測試（見 [平台支援](#平台支援)）。**不支援 Android / iOS。**

## 功能特色

- **多隧道並發** — 上游一次只能連接一條隧道，本版本可同時執行多條隧道 —
  適合同時存取公司內網與出口網路。
- **條件自動連接** — 以 Wi-Fi SSID / 時間段 / 系統啟動為觸發條件，自動連接或斷開隧道；
  規則支援優先級與互斥。
- **自動重連** — 意外斷線後隧道自動恢復，連線狀態即時顯示。
- **登入時自動啟動** — 登入後自動啟動 WireGuide Plus 並依規則連接（搭配「最小化啟動」
  視窗啟動即收攏）。
- **最小化啟動** — Windows 上啟動後最小化到工作列（工作列圖示保留，主視窗隨時可重新
  開啟）；macOS/Linux 上最小化到系統匣。
- **系統匣連線通知** — 啟動 10 秒後（管理員權限提示處理完成）顯示目前連線狀態；網路變更
  （Wi-Fi 切換、拔除網路線、網路中斷等）導致隧道狀態改變時，也會延遲 10 秒顯示穩定後的
  最新狀態。氣泡通知含有動作選單（開啟主視窗 / 斷開），可手動關閉，或於可設定的停留時間
  （預設 10 秒，可於設定中調整）後自動關閉。
- **隧道管理** — 匯入 / 匯出 `.conf`、連線歷史、快速切換。

## 相對上游 wireguide 的修復與增強

### 修復

1. **完整支援 Wi-Fi 作為網路出口（最關鍵的修復）** — 上游在 Windows 上只會從**有線**
   介面送出流量，導致 Wi-Fi 下出口無法使用。本版本修正預設出口介面的選擇，在 Wi-Fi 下
   流量會正確從無線介面卡送出。
2. **GUI 主題錯誤** — 修正深色 / 淺色主題切換時畫面渲染異常的問題。
3. **Windows 版本資訊標準化** — 修正 exe 內容頁版本資訊空白的問題（目前以 `goversioninfo`
   產生）。
4. **穩定性修復** — 更新檢查排程去重、物理介面卡偵測更精確等
   （見 [CHANGELOG](CHANGELOG.zh-TW.md)）。

### 增強

1. **SSID 下拉選單** — 自動連接規則可透過下拉選單直接選取系統**已儲存的所有 Wi-Fi SSID**，
   避免手動輸入出錯。
2. **透過代理檢查更新** — 可在檢查更新前設定 HTTP(S) 代理，解決因 GitHub 無法連線 /
   被限流導致的更新失敗。
3. **多語言介面** — 简体中文 / English / 日本語 / 한국어 / 繁體中文。
4. **系統整合** — 登入時自動啟動、啟動最小化（Windows 最小化至工作列 / macOS 與 Linux
   最小化至系統匣）、系統匣連線通知（啟動 10 秒後 / 網路變更導致連線狀態改變時顯示，
   預設停留 10 秒，可調整）。
5. **視窗標題與操作體驗最佳化** 等。

## 平台支援

| 平台 | 狀態 |
| --- | --- |
| Windows 10 / 11（x64、x86 32 位元、ARM64） | ✅ 完全支援（多隧道並發 + SSID 自動連接） |
| macOS（Apple Silicon / arm64） | 🚧 實驗性 — 已在 Apple Silicon 實機驗證；你同樣可以嘗試另一款名為 [WireTunnels](https://github.com/FMDigitech/WireTunnels) 的 app |
| Linux（x64、arm64） | 🚧 實驗性 — 經 CI 建置，尚未在實機測試 |
| Android / iOS | ❌ **不支援**（無法同時執行多條隧道，也無法依 Wi-Fi SSID 自動切換隧道） |

> **macOS 替代方案：[WireTunnels](https://github.com/FMDigitech/WireTunnels)** — 原生
> macOS 選單列 WireGuard 用戶端，支援多隧道、監控與控制，可補足上游 `wireguide` 的不足。

### 為何沒有行動版？

本專案的核心能力是**多隧道並發**與**規則式自動連接（例如依 Wi-Fi SSID）**。在
Android / iOS 上，系統核心與權限限制使 WireGuard 實作無法**同時執行多條隧道**或
**依 Wi-Fi SSID 自動切換隧道** — 行動平台上兩項核心目標皆無法達成。因此本專案**明確
不鎖定行動裝置**；行動用戶若只需單一隧道，請使用官方 WireGuard App 及其隨選（On-Demand）
功能。

## 路線圖

- **v2.0（規劃中）**：以 **Windows 系統服務**方式執行 — 無需使用者登入即可自動連接，
  更穩定的網路堆疊與更好的權限控制。

## 下載與安裝

每個 Release 將 Windows 建置分為兩類分別發布：**安裝程式**與**免安裝版（攜帶版）**。

**安裝程式（建議）**

- Windows x64 安裝程式：`wireguideplus-amd64-installer.exe`
- Windows x86（32 位元）安裝程式：`wireguideplus-x86-installer.exe`
- Windows ARM64 安裝程式：`wireguideplus-arm64-installer.exe`

安裝程式檔名內嵌架構（`wireguideplus-<arch>-installer.exe`，arch 為 `x86` / `amd64` /
`arm64`），安裝後程式檔名同樣帶架構（`wireguideplus-<arch>.exe`，在檔案內容→詳細資料中
亦顯示）。64 位元安裝程式預設安裝至 `C:\Program Files\WireGuide Plus`；32 位元安裝程式
預設安裝至 `C:\Program Files (x86)\WireGuide Plus`（32 位元系統為
`C:\Program Files\WireGuide Plus`）。
安裝目錄可於安裝過程中變更。會建立「開始」功能表捷徑（包含「解除安裝 WireGuide Plus」
項目，預設勾選、可取消）與桌面捷徑（一律建立）。安裝程式已內含全部所需檔案，無需額外下載。

**免安裝版（無需安裝）**

- `wireguideplus-amd64.exe` **+ `wintun-amd64.dll`**（32 位元 exe 配 **`wintun-x86.dll`**，
  ARM64 exe 配 **`wintun-arm64.dll`**）— 需同時下載**相同架構**的兩個檔案放在同一資料夾，
  再執行 exe。

免安裝版**並非獨立程式**：執行時需要在同資料夾放置與程式架構相符的驅動 DLL（用於
建立 WireGuard 隧道）。程式會依架構自動載入對應檔案（`wintun-amd64.dll` /
`wintun-x86.dll` / `wintun-arm64.dll`），**無需改名**，依下表選擇即可：

| exe | 相符的驅動 DLL |
| --- | --- |
| `wireguideplus-amd64.exe`（64 位元） | `wintun-amd64.dll` |
| `wireguideplus-x86.exe`（32 位元） | `wintun-x86.dll` |
| `wireguideplus-arm64.exe`（ARM64） | `wintun-arm64.dll` |

驅動 DLL 來自 `wintun-0.14.1.zip`（見
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintun-driver-dll)）。Release 提供打包好的
免安裝 zip（`wireguideplus-amd64-portable.zip` / `wireguideplus-x86-portable.zip` /
`wireguideplus-arm64-portable.zip`），內含 exe **與**相符架構的驅動 DLL——下載後解壓縮
即可執行。Release 不再單獨附驅動 DLL（請使用免安裝 zip 或安裝程式）。缺少相符的驅動
DLL 時無法建立隧道。

## 程式碼簽署

所有發布的 Windows **安裝程式**均經 Authenticode 簽署，可同時驗證**完整性**（二進位檔
在傳輸或磁碟上未被竄改）與**來源**（由本專案建置並發布）。已簽署的二進位檔在首次執行時
也會觸發較少的 Windows SmartScreen 警告。

注意：**僅安裝程式**經過簽署；免安裝版 zip 內為未簽署的建置產物。完整簽署政策（範圍、
核准流程、帳戶安全與可重現性）見 [SIGNING-POLICY.md](SIGNING-POLICY.md)。

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 建置與開發

建置環境需求、開發 / 發行建置指令（含 x86 + amd64 + arm64 多架構建置）、NSIS 安裝程式注意
事項、版本資源與發行流程，均記載於 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。發布只需
推送版本標籤 — GitHub Actions 管線會自動完成建置、簽署與發布（見 [docs/release.md](docs/release.md)）。

## 資料與日誌

| 項目 | 位置 |
| --- | --- |
| 設定 / 歷史 | `%APPDATA%\wireguideplus\`（`config.json`、`history.json`） |
| 隧道設定 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| 日誌 | `%APPDATA%\wireguideplus\logs\` |

## 解除安裝

透過**控制台 → 程式和功能 → WireGuide Plus** 解除安裝，或執行安裝目錄中的解除安裝程式。

## 致謝

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 上游開源專案
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
