# WireGuide Plus

**支援多隧道與網路感知自動化的 WireGuard 用戶端**

WireGuide Plus 是對開源專案 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)
進行**深度修復與增強**的版本。兩大核心能力：

- **多隧道並發** — 多條 WireGuard 隧道可同時建立、互不干擾地獨立執行；
- **條件自動連接** — 依 Wi-Fi SSID、時間段、系統啟動等條件自動連接對應隧道
  （例如辦公 Wi-Fi 下連隧道 A，家中連隧道 B）。

[English](README.md) | [简体中文](README.zh.md) | **繁體中文** | [한국어](README.ko.md) | [日本語](README.ja.md)

> **Windows 10 / 11（x64、x86 32 位元與 ARM64）以及 macOS（Apple Silicon / arm64）完全支援** —
> macOS 已在 Apple Silicon 實機充分驗證。Linux（x64、arm64）提供**實驗性預覽版** —
> 經 CI 建置、尚未在實機測試（見 [平台支援](#平台支援)）。**不支援 Android / iOS。**

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
  （預設 10 秒，可於設定中調整）後自動關閉。macOS 與 Linux 現已彈出同款氣泡（此前僅 Windows）。
- **隧道管理** — 匯入 / 匯出 `.conf`、連線歷史、快速切換。
- **AmneziaWG（AWG）隧道** — 支援匯入並連接 AmneziaWG（混淆版 WireGuard）設定。AWG 由設定中的 Jc/Jmin/Jmax/S1-S4/H1-H4 混淆參數自動辨識，對應隧道會顯示「AmneziaWG」徽章；可在「設定 → 進階」中關閉支援。
- **隧道編輯器：欄位檢視與腳本鉤子** — 除 conf 文字外，提供逐欄位表單（interface / peer 分組）編輯設定，並支援管理 PreUp / PostUp / PreDown / PostDown 腳本鉤子（選擇檔案、新建空白、編輯程式碼、清除）。隧道連線中也可編輯：儲存時會中斷、套用變更並自動重連。
- **每隧道實體出口綁定** — 開啟「固定介面」後，可將單一隧道的加密流量固定到指定實體網卡（Windows / Linux / macOS）；綁定網卡失效時彈窗提供等待、自動切換或手動指定。
- **設定匯出與匯入** — 將 tunnels、scripts 與 `config.json`（不含日誌）打包為單一壓縮檔，便於遷移到其他機器。
- **工具箱標籤頁** — 內建 **DNS 洩漏測試** 檢查流量是否真的從設定的 DNS 伺服器出去，以及 **路由視覺化** 展示目前路由表，並為每條路由標註 VPN / Direct 徽章，支援區域網路直連過濾與行動網路介面標記。
- **隧道策略** — 每隧道的 DNS 解析路徑、強制指定網域走隧道、流量保護、預設 DNS，以及確定性的衝突裁決。
- **延遲探測** — 每隧道延遲探測支援多目標並行；每個候選（公共解析器 8.8.8.8 / 223.5.5.5、隧道端點，以及任意手填地址或網域）同時探測，並展示其解析 IP、類型與往返時延。分流隧道下手填目標會在儲存前按隧道 AllowedIPs 覆蓋校驗。目標儲存後會立即重新探測；網域端點會在詳情頁頂部顯示目前解析到的位址。

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
6. **AmneziaWG（AWG）協定支援** — 新增以 amneziawg-go 為基礎的 AmneziaWG 協定後端，支援 DPI 抗辨識混淆隧道。設定自動辨識、介面顯示徽章，並可在「設定 → 進階」中一鍵關閉。

## 自動化規則

自動化以**隧道為單位**獨立設定（任一個隧道的 `…` 選單 →「自動化…」）。每個隧道擁有完全獨立的規則集合，因此「公司 Wi-Fi 連接辦公 VPN、公司 Wi-Fi 中斷家庭 VPN、回家自動連接家庭 VPN」這類組合可以同時存在且互不干擾。

編輯器頂部的即時網路看板只描述**目前隧道**的網路環境，每類資訊一行：所有偵測到的實體硬體介面（標註「使用中」或「未使用」）、Wi-Fi SSID、閘道 MAC、閘道 IP 與子網路，並顯示目前隧道中哪些條件比對到這些值。虛擬網卡會排除；未連線 Wi-Fi 時 SSID 顯示「Wi-Fi 未連線」。看板與規則說明皆可收合，整個編輯器支援捲動，規則較多時仍方便操作。

「固定介面」現已支援全部桌面平台：Windows 透過 `IP_UNICAST_IF` 將隧道的 UDP socket 綁定到指定網卡，Linux 使用 `ip route … dev <網卡>` 繞道路由，macOS 以 `-ifscope` 限定繞道路由。指定實體出口屬於作業系統路由策略（而非 WireGuard 協定設定）；綁定網卡失效時會彈窗讓你選擇保持綁定等待、自動切換或手動指定。

### 規則邏輯

- 單條隧道的規則是**一個有序清單**，由上往下依序評估——**規則位置即優先級**，連線與中斷規則可以在整個清單中自由混排和拖曳調整。
- **首條命中生效（規則內 AND）**：同一規則下所有條件皆成立，該規則才算命中；但整個清單中**只有第一條命中**的規則會執行動作。排在已命中規則之後的規則會被標註為「命中但降權」，不會真正執行，因此不會發生同一 SSID 下既 disconnect 又 connect 的自相矛盾。
- **預設狀態兜底**：當**所有規則都不符合**目前網路時，隧道收斂到其**預設狀態**（連線或中斷，在自動化編輯器頂部選擇）。既沒有規則也沒有預設狀態的隧道，自動化不會干預；只設定預設狀態（零規則）時，隧道會始終收斂到該預設狀態。
- **編輯期間即時匹配指示器**：開啟自動化編輯器後，每個條件會即時顯示是否命中目前網路；實際生效的首條命中規則會被亮選為「in use（使用中）」；頂部另有一條判斷條，顯示該隧道最終將執行的動作。每次編輯變更都會立即刷新（≈ 250 ms 防抖，透過 IPC 呼叫與背景 helper 執行控制使用**完全相同**的評估引擎，因此介面顯示與真實行為恆為一致）。同引擎也可在終端使用：`wireguideplus automation`，適用於無圖形介面的環境或除錯。

### 手動覆蓋

隧道詳情頁裡的「連接 / 斷開」按鈕屬於手動控制。**手動連接會強制連接、忽略所有自動化規則**；**手動斷開會強制斷開、忽略所有自動化規則**——直到下一次自動化決策重新評估網路。介面上有一個閂鎖指示器，標明目前隧道動作來自手動覆蓋還是自動化，讓你始終清楚到底是什麼在驅動連接。

### 條件類型

| 條件 | 匹配邏輯 | 典型情境 |
| --- | --- | --- |
| **SSID** | 與目前 Wi-Fi 的 SSID 做**位元組全名精確比較**（區分大小寫，中間空白與特殊字元全部參與比較，符合 802.11 定義）。 | 「連到 `公司 5GHz` 時自動連接辦公 VPN」。 |
| **子網路（Subnet）** | 本機實體網卡 IP 是否落在指定 CIDR 區段（例如 `192.168.178.0/24`）。 | 家用/辦公路由器 LAN 子網固定、但 SSID 會變動時。 |
| **閘道 MAC** | 目前預設閘道（路由器）的 MAC 位址 —— 即使 SSID 或網段很通用，也能鎖定某個具體網路。 | 「咖啡店那台路由器**絕對不要**自動連接。」 |
| **閘道 IP** | 目前實體網路的預設閘道 IP。 | 公司到處 SSID 都一樣、但每個樓層閘道 IP 不同時。 |
| **網卡介面（Interface）** | 系統做為上行路由的網卡介面名稱。下拉選單列出本機**全部**網路介面——有線與無線網卡都在內，也包含**尚未連線**的裝置，方便提前為擴充塢、USB 網卡、雷電網卡、Wi-Fi 網卡等尚未啟用裝置撰寫規則。 | 「只有插入公司擴充塊上的有線網卡時才連接辦公 VPN。」 |
| **時間段** | 一組星期幾 + 起止時間（本地時鐘）。 | 「週一至週五 09:00–18:00，辦公 VPN 必須保持連線。」 |

一條規則可任意組合以上條件：例如「SSID = 公司 AND 時間段 = 週一至週五 09–18」就是一條擁有兩個 AND 條件的規則。每個隧道的 disconnect / connect 兩組皆支援任意數量的 AND 規則。

## 平台支援

| 平台 | 狀態 |
| --- | --- |
| Windows 10 / 11（x64、x86 32 位元、ARM64） | ✅ 完全支援（多隧道並發 + SSID 自動連接，含 AmneziaWG） |
| macOS（Apple Silicon / arm64） | ✅ 完全支援 — 已在 Apple Silicon 實機充分驗證 |
| Linux（x64、arm64） | 🚧 實驗性 — 經 CI 建置，尚未在實機測試 |
| Android / iOS | ❌ **不支援**（無法同時執行多條隧道，也無法依 Wi-Fi SSID 自動切換隧道） |

### 為何沒有行動版？

本專案的核心能力是**多隧道並發**與**規則式自動連接（例如依 Wi-Fi SSID）**。在
Android / iOS 上，系統核心與權限限制使 WireGuard 實作無法**同時執行多條隧道**或
**依 Wi-Fi SSID 自動切換隧道** — 行動平台上兩項核心目標皆無法達成。因此本專案**明確
不鎖定行動裝置**；行動用戶若只需單一隧道，請使用官方 WireGuard App 及其隨選（On-Demand）
功能。

## 下載與安裝

每個 Release 都會為每個受支援的平台發布**安裝程式（建議）**與**免安裝版**：下方依作業系統說明。macOS 提供 `.dmg`/`.zip`，Linux 提供 `.deb`/`.tar.gz`，Windows 即下文的安裝程式/免安裝版。所有 Release 還會附帶一份 Ed25519 簽署的 `SHA256SUMS`（及 `SHA256SUMS.sig`），應用程式內更新器在套用任何更新前都會校驗它。

### Windows

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



### macOS（Apple Silicon）

每個 Release 提供兩個產物：

- `WireGuidePlus-darwin-arm64.dmg` — 拖曳至「應用程式」的安裝器。
- `WireGuidePlus-darwin-arm64.zip` — 免安裝 `.app` 套件。

開啟 `.dmg`，將 **WireGuide Plus** 拖入「應用程式」，再從聚焦或啟動台開啟。免安裝版 `.zip` 解壓後即為 `wireguideplus.app`，可直接執行。

注意：

- **僅支援 Apple Silicon。** CI 只建置 `arm64` 單一架構；Intel Mac 需自行[從原始碼建置](docs/DEVELOPMENT.md)。
- **僅本地臨時簽章（ad-hoc），未經 Apple 公證。** 首次開啟時 macOS Gatekeeper 會攔截。可右鍵 →「開啟」，或執行一次以下指令清除隔離屬性：
  ```sh
  xattr -dr com.apple.quarantine /Applications/wireguideplus.app
  ```
- 同時提供 Homebrew cask（arm64 版）：`brew install --cask wireguideplus` 取得的也是同一個 `WireGuidePlus-darwin-arm64.zip`。
- **位置服務授權（用於依 SSID 自動化）**。依 SSID 自動連接會讀取目前 Wi-Fi SSID，而在 macOS 上這需要 **位置服務** 權限。請在 **系統設定 → 隱私權與安全性 → 位置服務** 中開啟（先打開總開關，再允許 WireGuide Plus）；否則 SSID 條件無法評估，基於 SSID 的規則不會觸發。

### Linux（實驗性）

每個架構（`amd64`、`arm64`）提供兩個產物：

- `WireGuidePlus-linux-<arch>.deb` — Debian / Ubuntu 安裝套件。
- `WireGuidePlus-linux-<arch>-portable.tar.gz` — 免安裝二進位。

用套件管理員安裝 `.deb`，例如 `sudo apt install ./WireGuidePlus-linux-amd64.deb`；或解壓免安裝包後執行 `./wireguideplus`。免安裝版需要 GTK3 / WebKitGTK 執行期函式庫，`.deb` 會自動安裝；裸系統請先安裝：

```sh
sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0 libayatana-appindicator3-1
```

> Linux 建置為**實驗性**——僅經 CI 建置、尚未在實機驗證，且需要桌面工作階段（不支援無頭伺服器）。

## 程式碼簽署

程式碼簽署因平台而異。在 **Windows** 上，每個發布的**安裝程式**都經過 Authenticode 簽署（啟用 SignPath 簽署時），可驗證**完整性**——二進位檔自簽署後未被修改，且首次執行觸發的 Windows SmartScreen 警告更少。在 **macOS** 上，`.app` 為**本地臨時簽章（ad-hoc）**（無 Apple 開發者簽章），故首次啟動會被 Gatekeeper 攔截（見上方 macOS 安裝說明）。在 **Linux** 上，`.deb` 與免安裝版均為**未簽章**。

簽章本身證明的是**由誰簽署**，並不能單獨證明**安裝程式是如何建置出來的**。建置來源、核准
流程、帳戶安全與可重現性等資訊，以及隨每個 Release 提供的 SHA-256 校驗和，均記錄在
[SIGNING-POLICY.md](SIGNING-POLICY.md)。

注意：所有平台中，只有 Windows 安裝程式帶有程式碼簽章；macOS 的 `.app` 為本地臨時簽章、Linux 產物均未簽章，因此每個 Release 附帶的 Ed25519 簽署 `SHA256SUMS` 才是通用的完整性校驗方式。

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 建置與開發

建置環境需求、開發 / 發行建置指令（含 x86 + amd64 + arm64 多架構建置）、NSIS 安裝程式注意
事項、版本資源與發行流程，均記載於 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。發布只需
推送版本標籤 — GitHub Actions 管線會自動完成建置、簽署與發布（見 [docs/release.md](docs/release.md)）。

## 資料與日誌

| 平台 | 項目 | 位置 |
| --- | --- | --- |
| Windows | 設定 / 歷史 | `%APPDATA%\wireguideplus\`（`config.json`、`history.json`） |
| Windows | 隧道設定 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| Windows | 隧道腳本 | `%APPDATA%\wireguideplus\scripts\` |
| Windows | 日誌 | `%APPDATA%\wireguideplus\logs\` |
| macOS | 設定 / 歷史 | `~/Library/Application Support/wireguideplus/` |
| macOS | 隧道設定 | `~/Library/Application Support/wireguideplus/tunnels/*.conf` |
| macOS | 隧道腳本 | `~/Library/Application Support/wireguideplus/scripts/` |
| macOS | 日誌 | `~/Library/Logs/wireguideplus/` |
| Linux | 設定 / 歷史 | `~/.config/wireguideplus/`（`$XDG_CONFIG_HOME/wireguideplus/`） |
| Linux | 隧道設定 | `~/.config/wireguideplus/tunnels/*.conf` |
| Linux | 隧道腳本 | `~/.config/wireguideplus/scripts/` |
| Linux | 日誌 | `~/.local/share/wireguideplus/`（`$XDG_DATA_HOME/wireguideplus/`） |

## 解除安裝

- **Windows** — 透過**控制台 → 程式和功能 → WireGuide Plus** 解除安裝，或執行安裝目錄中的解除安裝程式。
- **macOS** — 將 **WireGuide Plus** 從「應用程式」拖入垃圾桶；如需可一併刪除 `~/Library/Application Support/wireguideplus` 與 `~/Library/Preferences/com.imonior.wireguide-plus.plist`。
- **Linux** — `sudo apt remove wireguideplus`（`.deb`），或刪除免安裝二進位與 `~/.config/wireguideplus`。
## 致謝

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 上游開源專案
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
