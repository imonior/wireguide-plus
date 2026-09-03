# Changelog

All notable changes to WireGuide Plus will be documented in this file.

> 简体中文: [CHANGELOG.md](CHANGELOG.md) · English: [CHANGELOG.en.md](CHANGELOG.en.md) · 日本語: [CHANGELOG.ja.md](CHANGELOG.ja.md) · 한국어: [CHANGELOG.ko.md](CHANGELOG.ko.md)

## [1.7.0] - 2026-09-04

### ✨ 新增

- **Automation 即時網路看板** — 顯示目前隧道的實體硬體介面、Wi-Fi SSID、閘道 MAC、閘道 IP 與子網路，並標示規則比對情況；排除虛擬網卡，區分使用中與未使用的硬體介面。
- **Automation 編輯器互動最佳化** — 說明與看板可分別收合，整個編輯器支援捲動，保留條件拖曳排序。

### 🐛 修復

- **Automation 狀態語義統一** — `match` 表示條件比對，`in use` 表示首條命中規則被選取，`active` 表示隧道實際運行；otherwise 條件即使被降權仍顯示為比對成功。
- **設定錯誤提示多語言化** — Pin Interface、日誌級別、Kill Switch、DNS 保護與健康檢查的失敗及平台不支援提示不再硬編碼中文。

### 📝 文件

- **同步更新 5 種語言 README 與 CHANGELOG** — 補充即時網路看板、介面狀態、編輯器互動及 Pin Interface 的平台支援範圍。

## [1.6.5] - 2026-09-02

### ✨ 新增

- **自動化編輯器：草稿變更即時重判讀** — 每次編輯規則、條件、拖曳排序都會在約 250 ms 防抖後，立即透過 IPC 呼叫後台評估引擎，match / in use / 頂部裁決條不等 3 秒輪詢就能更新；仍然與 helper 實際控制共用同一引擎，UI 與真實行為始終一致。
- **自動化編輯器 UI 緊湊化** — 規則卡片內上下間距縮小，條件列輸入控制、星期按鈕更收斂，同一可視高度約可多顯示 20% 的規則。

### 🐛 修復

- **進階設定四項開關與日誌級別 統一落盤策略** — Kill Switch / DNS 保護 / 固定介面 / 健康檢查 / 日誌級別 由「先樂觀寫盤再回滾」改為統一的「先 IPC 呼叫 helper 即時套用 → 成功才寫記憶體+落盤」：失敗時 settings.json 不再寫入髒值、UI 勾選框自動彈回並顯示失敗原因 toast；日誌級別 select 也套用同樣流程。
- **Wg Scripts 開關「取消」不回寫盤** — 啟用時的安全確認對話框，如果使用者按下取消，先前在確認前的 `scheduleSave()` 已把 `enable_wg_scripts=true` 寫入磁碟，取消只回滾記憶體，導致下次開啟顯示「已啟用」但實際沒生效；現在只有確認/取消按鍵按下後才會寫盤。
- **Pin Interface 開關「點擊滑軌本體不觸發」** — `.toggle input` 缺少顯式 inset，使得 0×0 的透明 input 命中區漂移到 track/slider 視覺邊界外；點擊滑軌/滑桿本體而非外層 `<label for="…">` 文字時偶發無回應（特別是區段最末的設定卡）。現在 input 鋪滿整個 `.toggle` 容器。
- **Automation live matching 指示不穩定** — 同一網路環境下每次開啟編輯器，match / active / 頂部決策標籤顯示內容均不相同：多個非同步刷新入口（onMount、load、輪詢、關閉暫停）之間缺少會話邊界，加上關閉時清空了計時器但再次開啟不會重建；現在使用 `previewEpoch + AbortController` 建立會話冪等鍵，`open && !previewTimer` 反應式守衛自動重啟輪詢，關閉/銷毀時完整清理三件套，杜絕任何過期在途請求污染當前會話。
- **DNS 洩漏測試 public-dns.info 頻繁超時** — 原 HTTP 用戶端逾時 10s 與 UI 層 ctx 逾時同量級，壅塞鏈路上下載 4-8 MB 的 `nameservers.json` 常在 body 讀到一半時觸發「parse JSON: context deadline exceeded」（看起來像解析失敗，其實是傳輸逾時）；用戶端逾時放寬至 30s，`LimitReader` 由 16 MB 收緊到 4 MB（前幾百條高可靠伺服器條目即足夠，其餘多為品質較差的節點），UI 的 10 s ctx 仍可優先取消。

### 📝 文件

- **README（5 種語言）新增「自動化規則」獨立章節**：涵蓋規則語義（規則內 AND、規則間 OR 首條命中、disconnect 先於 connect、otherwise 兜底）、7 種條件類型匹配定義與真實使用場景、編輯中即時匹配指示器，以及 CLI `automation` 命令說明。

## [1.6.0] - 2026-09-02

### ✨ 新增

- **自動化規則新增四類條件** — 每隧道的 connect/disconnect 規則現在支援：閘道 IP（gateway_ip）、網路介面（interface，候選清單包含目前未連線的實體網卡）、在有線網路（ethernet）、時間段（time，起止時刻 + 星期幾）。規則內條件為 AND、規則間為 OR，按落盤順序首條命中生效；disconnect 組先於 connect 組評估，connect 命中會被降權不執行。
- **自動化編輯器即時判讀** — 編輯規則時逐條件顯示是否命中（match）並高亮目前實際生效的規則（in use），頂部裁決指示條同步顯示最終將執行的動作；判讀與 helper 實際控制共用同一引擎，標記與真實行為一致。
- **CLI 新增 `automation` 命令** — 終端查看每條規則的即時命中詳情與裁決結果，新條件類型均有可讀格式。

### 🐛 修正

- **SSID 改為全名精確匹配**（行為變更）— SSID 按位元組全名比較：區分大小寫，中間空格與特殊字元均參與匹配（符合 802.11 對 SSID 的定義）。此前不區分大小寫；規則中填寫的 SSID 必須與實際廣播完全一致，編輯器即時判讀會直接顯示是否匹配。
- **「在有線網路」條件無法保存** — ethernet 條件此前不會被持久化，保存後靜默消失，即時判讀的規則映射也隨之錯位。
- **編輯器指示器多處修正** — 未完成的草稿規則導致其後規則高亮錯位；手動關閉（manual-off）時不再把被抑制的 connect 規則標記為「生效中」；打開編輯器立即重新整理判讀，不再有最長 3 秒的過期資料視窗。
- **Windows 路由衝突檢測修正** — 衝突診斷的路由衝突此前在 Windows 上恆為空（`route print` 輸出解析不可靠），全隧道（0.0.0.0/0）情境下的路由重疊警告因此失效；現改用 iphlpapi `GetIpForwardTable2` 路由表，與「診斷 → 路由」檢視資料一致。

### 🛠 內部

- helper 中規則評估相關檔案按實際職責改名：`wifi_rules.go` → `automation_rules.go`，平台檔案 `wifi_rules_{windows,darwin,linux}.go` → `userdir_{windows,darwin,linux}.go`；實體網卡列舉拆分為 `iface_*.go` 平台實作。

### 📝 文件

- README（5 種語言）：macOS（Apple Silicon）支援狀態由「實驗性」升級為「完全支援」。

## [1.5.3] - 2026-09-02

### 🐛 修正

- **視窗位置記憶真正生效** — 1.5.2 已保存視窗大小與位置，但位置在重啟後總被重置回螢幕居中：Wails 建立視窗時未明確指定定位模式，各平台預設執行居中而忽略保存的 X/Y 座標。現在僅當存在有效的已保存位置時才套用絕對定位，Windows / macOS / Linux 重啟後都能原樣還原視窗位置。
- **Linux 位置恢復偏移** — Linux 在視窗顯示後會把保存的座標當作相對於目前顯示器工作區的偏移重新套用，導致視窗跑偏；現改為視窗顯示後以絕對座標重新設定一次位置。

### 🎨 UI 最佳化

- **更小的視窗下限** — 最小視窗尺寸從 920×640 收窄至 720×560，並允許詳細面板隨視窗收縮（min-width: 0），小螢幕 / 低解析度下視窗可縮得更小而不被內容撐開。

## [1.5.2] - 2026-09-02

### ✨ 新增

- **記住主視窗的位置與大小** — 視窗關閉到托盤、最小化或退出時保存目前幾何狀態，下次啟動（含從托盤恢復）原樣還原，無需每次重新拖曳調整。

### 🛠 內部

- **GitHub Actions 升級至最新主版本** — checkout@v5、setup-go@v6、setup-node@v5、upload/download-artifact@v7、action-gh-release@v3。

## [1.5.1] - 2026-09-02

### 🐛 修正

- **Windows 升級改回靜默安裝** — 撤銷 1.5.0 的互動式安裝精靈：下載完成後只需確認一次 UAC 授權，即自動靜默覆蓋安裝，完成後自動開啟新版本。
- **升級一律安裝到原目錄** — 將目前安裝位置傳給安裝程式，自訂安裝目錄的使用者升級時不再於預設位置（Program Files）產生第二個副本。
- **移除「自動靜默升級」設定**（1.5.0 引入）— 更新行為三端一致：Windows 靜默安裝+自動開啟、Linux 安裝完成自動開啟（僅系統 polkit 授權）、macOS 應用內原位替換（寫入 /Applications 時彈出系統授權）。

## [1.5.0] - 2026-09-02

### ✨ 新增

- **Windows 更新改為互動式安裝精靈**（「立即更新」啟動與一般安裝相同的精靈）。
- **新增「自動靜默升級」設定**。

### 🐛 修正

- **Linux 更新完成後自動開啟軟體** — deb/rpm 套件管理員更新完成後會自動重新啟動應用程式以載入新版本。

> 1.5.0 的互動式安裝與靜默設定項已在 1.5.1 回退為三端一致的靜默更新。

## [1.4.1] - 2026-09-02

### 🐛 修正

- **Windows 應用內更新後自動啟動軟體** — 更新安裝完成後自動重新開啟軟體。此前安裝器以靜默模式執行，會跳過完成頁（「執行」核取方塊只在完成頁上，靜默模式下不生效），導致升級成功卻沒有任何視窗出現；現安裝器偵測到更新專用參數（`/AUTOSTART`）後，於安裝完成時以一般使用者權限（而非 UAC 管理員權杖）啟動新版本。

## [1.4.0] - 2026-09-02

macOS（非 Homebrew）的應用內更新現與 Windows / Linux 完全一致：可直接在軟體內下載新版本並覆蓋安裝，無需再跳轉瀏覽器手動下載。

### ✨ 新功能

- **macOS 應用內覆蓋安裝** — 非 Homebrew 安裝的 macOS 使用者點擊「更新」後，應用會在軟體內下載安裝包（.dmg，含 .zip 備用），經「設定 → 更新」中配置的鏡像 / 代理下載，驗證 SHA256 與 Ed25519 簽章後，再驗證程式碼簽章（`codesign --verify`），隨後自動替換安裝並重新啟動。App 從任何位置（`/Applications`、`~/Applications` 或自訂目錄）執行時都會原位覆蓋，Dock 圖示與 Finder 位置保持不變；安裝在系統目錄時彈出 macOS 系統授權提示（與 Windows UAC、Linux polkit 一致的體驗），並自動移除 quarantine 屬性避免 Gatekeeper 攔截。Homebrew 安裝的 macOS 使用者仍走 `brew upgrade`。
- **鏡像 / 代理涵蓋全部平台** — macOS 應用內更新的下載與 Windows / Linux 一致，全部經由「設定 → 更新」中配置的鏡像（GitHub 加速）或本機代理，全程不依賴瀏覽器。

### 🐛 修復

- **macOS 選單列整理** — 修復 Wails 預設選單列 Help → Learn More 會把 WebView 導向 wails.io、導致無法返回主介面的問題：Learn More 現改為在系統預設瀏覽器中開啟 GitHub 專案頁面，WebView 不再被劫持；同時移除無實際用途的 File / Edit / View / Window 選單（縮放、全螢幕、最小化均可在應用程式介面與視窗標題列完成），僅保留 App 與自訂 Help。Windows / Linux 不顯示該選單列，不受影響。

### 🛠 內部

- 新增 `internal/update/installer_darwin.go`：macOS 應用包原位替換安裝器（dmg 掛載 / zip 解壓 → 程式碼簽章驗證 → 提權指令碼執行 killall + 替換 + 去 quarantine + 重新啟動），與 Windows / Linux 安裝器共用同一套下載、驗證與進度管線。

## [1.3.7] - 2026-09-01

### 🐛 修復

- **Windows 應用內更新安裝器啟動失敗** — 以 Windows ShellExecute `runas` 取代 PowerShell `Start-Process -Verb RunAs`，避免 PowerShell 執行原則/路徑問題導致的 `exit status 1`；同時將下載的安裝器複製到 `%LOCALAPPDATA%\wireguideplus\updates` 持久目錄，防止 `Install` 返回後臨時檔案被清理，導致 UAC 確認後安裝器找不到 exe。
- **Linux 應用內更新健壯性** — 安裝資產同樣先複製到持久目錄（`$XDG_DATA_HOME/wireguideplus/updates`）再啟動，消除 AppImage 非同步啟動與臨時檔案清理的競態；副檔名比對改為大小寫不敏感並支援 `.AppImage`；`.tar.gz` 等未知格式不再被當作可執行檔執行，而是明確失敗並回退到下載頁；`pkexec` 失敗時保留其輸出，便於判斷是否為 polkit 代理缺失。

## [1.3.6] - 2026-09-01

本版本將更名前舊版（"wireguide"）資料的遷移改為**使用者可見的互動式引導**：啟動時掃描舊版遺留的設定、隧道與日誌，由你決定遷移哪些、如何處理重名衝突，並可先比對新舊目錄再動手，不再靜默覆蓋。

### ✨ 新功能

- **舊版資料遷移彈窗** — 啟動時自動偵測更名前 "wireguide" 目錄中的 config.json、history.json、tunnels/*.conf 與日誌；彈窗內按類別顯示數量與重名衝突，可一鍵「全部遷移」，遷移成功即清理舊目錄並記錄狀態，之後不再打擾。
- **新舊目錄比對** — 彈窗內可直接開啟舊版 / 目前設定目錄與日誌目錄，先核對內容再遷移。
- **遷移選項** — 重名衝突時可勾選「覆蓋現有檔案」；日誌預設不遷移，可按需開啟。
- **暫緩與不再提醒** — 「下次啟動再提醒」僅關閉彈窗不寫任何標記，下次啟動重新偵測；「不再提醒」持久化選擇，之後不再彈出，仍可在「設定 → 進階 → 舊資料」重新觸發掃描。

### 🎨 UI 最佳化

- **主題化捲軸** — 全域捲軸改用主題 token 繪製（細捲軸 + 圓角滑塊），長列表（隧道、日誌、歷史）在 Windows WebView 下不再回退到系統預設樣式。
- **彈窗樣式 token 化** — 更新提示等彈窗的背景、卡片、陰影改用主題變數（`--overlay-bg` / `--bg-card` / `--shadow-md`），移除手寫暗色模式 media query，亮/暗主題表現統一。
- **AWG 徽章主題一致** — 隧道列表與詳情中的 AmneziaWG 標識改用主題色 token（`var(--purple)`），取代硬編碼顏色，暗色主題下不再突兀。

### 🛠 內部

- 移除舊版在首次啟動時靜默自動遷移的邏輯（原在 `GetPaths` 中自動複製），改由明確的 `DetectLegacyData` / `MigrateLegacyData` 互動流程驅動；CLI 命令不再自動遷移，首次 GUI 啟動負責引導。

## [1.3.5] - 2026-09-01

本版本新增 **AmneziaWG（AWG）協定支援**——抗 DPI 識別的混淆版 WireGuard。AWG 設定透過混淆參數（Jc/Jmin/Jmax/S1-S4/H1-H4）自動辨識，執行於 amneziawg-go 後端，介面以「AmneziaWG」徽章標示；可在「設定 → 進階」中關閉支援。

### ✨ 新功能

- **AmneziaWG（AWG）協定支援** — 匯入並連接 AmneziaWG 設定；由 Jc/Jmin/Jmax/S1-S4/H1-H4 鍵自動辨識，清單與詳情頁顯示「AmneziaWG」徽章，握手 / 流量等狀態與 WireGuard 隧道一致。
- **啟用 AmneziaWG 設定** — 「設定 → 進階」新增「啟用 AmneziaWG 支援」開關（預設開啟）；關閉後連接 AWG 隧道會直接給出明確錯誤，而非連接中途失敗。

### 🛠 內部

- 以 amneziawg-go 為基礎的新協定後端接入引擎抽象層 — 兩種協定共用同一條連接管線；AWG 隧道狀態一律走程序內 UAPI；Windows socket 固定對兩種後端同樣生效。

## [1.3.1] - 2026-09-01

本版本修復 Windows 應用程式內更新啟動安裝程式前未要求 UAC 權限提升的問題，並正式記錄 macOS 已在 Apple Silicon 實機驗證。

### 🐛 修正

- **Windows 安裝程式 UAC 權限提升** — 應用程式內更新啟動安裝程式前先要求提升權限，與手動雙擊安裝程式的行為一致。

### 🛠 內部

- **macOS 實機驗證** — 平台支援說明更新：macOS（Apple Silicon）已在實機驗證。

## [1.3.0] - 2026-09-01

本版本將應用程式更名為 **WireGuide Plus**：視窗標題、托盤、自動啟動項目、helper 日誌、Homebrew cask、更新暫存檔與 nftables 表格名稱等全鏈路統一為 plus 命名，並在升級時自動清理舊版殘留的啟動項目、守護程序與防火牆表格。同時改進 macOS 托盤圖示（改用應用程式圖示紅色變體）與路由診斷顯示。

### ✨ 新功能

- **macOS 托盤圖示改用應用程式圖示** — 選單列圖示使用應用程式圖示的紅色變體，在淺色 / 深色選單列下都清晰可辨；未內嵌圖示時回退到原單色 W 模板。
- **macOS 路由診斷正規化** — `netstat -rn` 會把 127.0.0.0/8 顯示成 "127"、192.168.1.0/24 顯示成 "192.168.1"；診斷頁現在會展開回正規點分十進位 + 前綴，顯示不再像截斷。

### 🛠 內部

- **全鏈路更名 WireGuide Plus**：macOS 自動啟動 `com.wireguideplus.gui`、LaunchDaemon 與 helper 日誌路徑、pf anchor `com.apple/wireguideplus`、Linux 桌面圖示、Windows 自動啟動登錄值、wintun 介面卡名稱 `WireGuidePlus-<hash>`、FWPM 工作階段 / Provider / SubLayer 名稱、nftables 表格 `wireguideplus` / `wireguideplus_dns`、Homebrew cask `wireguideplus` 與 Caskroom 路徑、更新暫存檔與衝突檢測 socket 路徑、發佈機金鑰目錄 `~/.wireguideplus`、測試環境變數 `WIREGUIDEPLUS_RESOURCE_*`、macOS 授權彈窗文案全部統一。
- **升級相容清理**：升級 / 解除安裝時移除舊版殘留的 `com.wireguide.gui` LaunchAgent、`com.wireguide.helper` LaunchDaemon 與 helper、舊 helper 日誌、舊 pf anchor `com.apple/wireguide`、`wireguide.desktop` 自動啟動項目、舊 wintun 介面卡 `WireGuide-<hash>`、舊 nft 表格與舊 FWPM Provider。
- **發佈產物改名**：macOS zip / dmg 與 Linux deb 資產名稱改為 `WireGuidePlus-*`；NSIS PATH 提示與 MSIX 模板可執行檔名稱同步。
- **測試腳本同步**：systemd unit 與測試 socket 統一為 `wireguideplus-*` 前綴。

## [1.2.5] - 2026-09-01

本版本重構 DNS 洩漏檢測：新增「公共 DNS 交叉驗證」——測試時除本機設定的解析器外，還會向知名公共 DNS 發送探測以交叉核實；系統解析器依來源網卡分類標記「本機 / VPN / 公共」；公共清單支援從網路重新整理與自由編輯。新增「瀏覽器檢測」按鈕，一鍵開啟 browserleaks.com 進行瀏覽器級 DNS 與 WebRTC 洩漏檢測。同時修復 Windows 連線通知彈窗可能凍結無回應的問題，並更新應用程式圖示。

### ✨ 新功能

- **公共 DNS 交叉驗證** — 測試時除本機設定的 DNS 外，還會向知名公共解析器（Google、Cloudflare、OpenDNS、Quad9、阿里、騰訊 DNSPod、114DNS、百度、AdGuard、NextDNS、Comodo 及常用 IPv6 位址）發送探測，交叉核實 DNS 查詢是否仍只經過隧道。公共解析器的回應僅表示「可達」，並非洩漏。
- **從網路取得公共清單** — 點擊「從網路取得」從 public-dns.info 拉取目前可靠性最高的解析器（上限 30 個，10 秒逾時），並快取上次成功取得的清單，離線時仍可使用。
- **自訂公共解析器清單** — 可自由新增 / 刪除 / 編輯公共解析器條目（IP 或網域名稱），儲存在設定中；清空清單會恢復預設交叉驗證清單，公共探測始終開啟。
- **系統解析器分類標記** — 依來源網卡分類：實體網卡（無線 / 有線）標記「本機」，隧道介面標記「VPN」，其餘為「公共」；本機解析器排在最前，並顯示來源介面名稱（Windows 依網卡列舉 DNS，Linux 解析 resolvectl 輸出）。
- **瀏覽器檢測** — 新增「瀏覽器檢測」按鈕，一鍵開啟 browserleaks.com 執行瀏覽器級 DNS 與 WebRTC 洩漏檢測（將開啟預設瀏覽器，檢測資料會傳送給第三方網站）。

### 🐛 修復

- **Windows 通知彈窗凍結** — 彈窗的訊息迴圈之前沒有綁定建立它的作業系統執行緒，goroutine 線上緒間遷移後收不到點擊 / 關閉 / 定時器訊息，彈窗會看起來「卡死」。現已鎖定執行緒，彈窗可正常點擊關閉與自動關閉。
- **通知文字繪製加固** — 彈窗文字繪製改用 `UTF16FromString` 並處理錯誤，避免非法 UTF-16 字串導致崩潰。

### 🛠 內部

- CLI `dnsleak` 指令同步增強：解析器列顯示 `vpn / local / public` 標記與狀態，並讀取設定中的自訂公共清單。
- 洩漏判定修正：只有實體介面（非 VPN）解析器回應才判定為洩漏；VPN 解析器標記為 VPN 狀態；公共解析器回應顯示「正常」而非洩漏。
- 新增 `dnsleak` 探測計畫與解析測試；重新產生 bindings。
- 更新應用程式圖示（各平台），簡化建置任務。

## [1.1.10] - 2026-08-31

本版本修復 1.1.9 回報的三個介面問題並優化設定互動：DNS 洩漏測試頁不再限制顯示寬度並標記本機 DNS；日誌等級篩選改為精確匹配；設定中的通知時長與代理選擇恢復正常儲存與回顯，自訂鏡像 / 本機代理輸入框會記住上次使用的位址。

### 🐛 修復

- **DNS 洩漏測試頁寬度** — 移除頁面內容 640px 的最大寬度限制，與「歷史」「路由」頁面一樣隨視窗自適應鋪滿。
- **本機 DNS 標示** — 測試清單中的每台伺服器都來自系統解析器設定（無論手動設定還是 DHCP 取得），現在每行會顯示「本機」標籤，便於與 VPN 提供的 DNS 區分。
- **日誌等級篩選** — 點擊「DEBUG / INFO / WARN / ERROR」按鈕現在只顯示該等級的記錄（此前是「該等級及以上」，當某等級沒有記錄時看起來像篩選失效）。
- **通知時長設定** — 下拉選項改為與「保留日誌 / 保留歷史 / 語言」一致的動態選項寫法，確保修改後能正確儲存並在下次開啟時回顯。
- **代理模式回顯** — 修復設定頁重新開啟後代理下拉框始終顯示「直接」的問題（Svelte 無法追蹤函式本體讀取的欄位，導致 `<select value={函式()}>` 只在首次求值）。改為響應式計算後，選擇儲存的鏡像 / 手動模式會在重新開啟時正確顯示。
- **代理位址記憶** — 切換為「自訂鏡像」或「本機代理」時，輸入框會自動載入上次儲存過的位址（例如曾經輸入並儲存的鏡像前置）；沒有歷史記錄時顯示空白與提示。

## [1.1.9] - 2026-08-31

本版本修復應用內更新「下載成功卻無法安裝」的問題：更新流程在啟動安裝器之前就刪除了暫存下載檔，導致 Windows 上啟動安裝器時提示「找不到檔案」並回退到瀏覽器頁面。

### 🐛 修復

- **應用內更新無法安裝** — `runUpdateNative` 原先在 `Install` 之前就執行 `os.Remove(path)` 刪除暫存下載的安裝包，而 Windows 安裝流程是直接執行該檔案（`fork/exec …wireguide-update-*.exe: The system cannot find the file specified`），因此下載 100% 後必然啟動失敗。現已調整為安裝器啟動成功後再釋放暫存檔；Windows 上安裝器執行期間檔案通常被鎖定、刪除可能失敗，但由系統暫存目錄自動清理，無影響。
- **需要手動升級一次** — 1.1.7 / 1.1.8 的更新流程存在同樣問題；請從本版本起手動下載安裝一次（設定 → 更新 → 開啟發布頁），此後應用內更新即可正常運作。

## [1.1.8] - 2026-08-31

本版本對齊自動化規則的判定語義與介面引導，並進一步加固編輯器對舊格式規則的相容：規則自上而下、首個比對生效，同一動作的條件之間為「或」關係，「否則」作為兜底應放在最後並執行相反動作；磁碟上缺少條件類型的舊規則不再觸發無謂重載。

### ✨ 優化

- **自動化規則語義引導對齊** — 編輯器說明與「否則」條目文案更新：明確「否則」在上方規則皆未比對時生效、建議放在最後作為兜底、動作通常與上方規則相反（五種語言同步）。判定邏輯本身保持不變：依序、首個比對生效、`otherwise` 無條件兜底——與你期望的行為一致。

### 🐛 修復

- **舊格式規則不再觸發無謂重載** — 編輯器比對磁碟與本地規則時統一使用與載入相同的類型推斷（缺少 `type` 的舊「否則」規則不再回退為 network），避免每次設定變更都誤判為外部修改而觸發一次多餘重載。

### 🛠 內部

- 重新生成 bindings 並驗證與 Go API 完全一致（無差異）。
- 版本號更新至 **1.1.8**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm 全部同步。

## [1.1.7] - 2026-08-31

本版本集中修復 1.1.6 回報的問題：自動化規則不再遺失、DNS 洩漏檢測補全狀態與加密方式、路由表區分 VPN / 直連、日誌過濾修正、通知時長與代理顯示問題；並新增連線歷史保留時長設定與安裝完成後「執行」選項。

### 🐛 修復

- **自動化規則不再遺失（含 otherwise）** — 編輯器載入時不再把缺少條件類型的規則誤判為不完整而丟棄；無法被表單表示的磁碟規則也會原樣保留，杜絕「開啟設定後規則消失」。
- **DNS 洩漏檢測補全結果** — 每台 DNS 伺服器現在正確顯示探測狀態（VPN / 洩漏 / 正常 / 無回應）與延遲；新增「使用中」標記指出當前實際出口 DNS。
- **DNS 加密方式探測** — 檢測每台解析器支援的傳輸：明文 UDP/53、DoT（TCP/853 TLS）、DoH（TCP/443 候選），並在檢測後給出結果解讀與防洩漏建議（使用 VPN DNS、加密 DNS、全隧道模式等）。
- **路由表區分 VPN / 直連** — 後端按活動隧道介面權威標記 `is_vpn`，路由明細正確顯示 VPN / Direct 徽章，不再依賴介面名稱猜測。
- **日誌過濾修正** — 日誌事件補傳 `category` 欄位，分類篩選真正生效；級別/分類按鈕顯示各檔計數，直觀看出當前日誌分佈。
- **通知持續時間設定** — 修復下拉框在部分 Svelte 版本下渲染空白、無法顯示所選時長的問題。
- **代理顯示一致性** — direct 模式下不再殘留代理位址；CLI 修改代理模式後設定介面即時同步。

### ✨ 優化

- **連線歷史保留時長** — 設定 → 進階新增「歷史記錄保留時長」（預設 7 天，可關閉），超出自動滾動清理（仍保留 200 條硬上限）。
- **安裝完成提示執行** — Windows 安裝器完成頁新增「執行 WireGuide Plus」選項（預設勾選）。

### 🛠 內部

- 版本號更新至 **1.1.7**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm 全部同步。

## [1.1.6] - 2026-08-30

本版本升級更新機制：Windows / Linux 支援在應用內直接下載並安裝更新（不再只能跳轉 GitHub 頁面），更新通知提供「直接升級」與「打開發布頁」雙按鈕並顯示即時下載進度；鏡像模式下資產下載同樣走加速鏡像。

### ✨ 新功能

- **應用內直接升級（Windows / Linux）** — 更新通知新增「直接升級」按鈕：下載完成後自動驗證 SHA256（發布版含 Ed25519 簽名），通過後啟動安裝並結束應用；macOS 的 Homebrew 安裝仍走 `brew upgrade`。
- **「打開發布頁」備選按鈕** — 下載失敗、驗證不通過或想查看發布說明時，一鍵在瀏覽器開啟對應版本的 GitHub Release 頁面。
- **即時下載進度** — 升級過程顯示已下載 / 總大小與進度百分比（基於 GitHub API 報告的資產大小，分塊傳輸時同樣準確）。
- **鏡像模式涵蓋資產下載** — 選擇 GitHub 加速鏡像（mirror）後，資產與校驗和檔案的下載同樣經鏡像前綴重寫（此前僅 API 檢查走鏡像，二進位仍直連 GitHub）。

### 🛠 內部

- 下載或安裝失敗時不再靜默：記錄日誌並回退到打開發布頁，保證始終有可用路徑。
- 新增下載進度回呼、鏡像下載重寫與 `RunUpdate` 防禦分支的單元測試。
- 版本提升至 **1.1.6**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.5] - 2026-08-30

本版本全面強化日誌系統（更新檢查、設定稽核、分類分級、保留期限清理），修復若干設定問題，並重新加入預設關閉的 WireGuard 腳本支援。

### ✨ 新功能

- **更新檢查完整日誌** — 手動與自動檢查均記錄實際請求的 endpoint、本機版本、線上版本、`not_modified` 以及錯誤/重試資訊；失敗（403、逾時等）帶 `category=update`，可在 Log 介面檢視與篩選。
- **設定變更稽核日誌** — 每次儲存都會記錄哪些設定被修改（代理模式、kill switch 等）及關鍵值；代理憑證會遮罩（`http://***@host`）。
- **日誌分類與篩選** — `ipc.LogEntry` 新增 `category` 欄位（app / update / settings / tunnel / network / system）；Log 介面新增分類篩選列（All 在最前、預設選中），每筆日誌顯示分類，複製時也攜帶分類。
- **日誌保留期限（預設 7 天）** — 按天輪替儲存（`wireguideplus-YYYY-MM-DD.log`），超過可設定保留期限自動清理。
- **WireGuard 腳本支援（PreUp / PostUp / PreDown / PostDown，預設關閉）** — 與 wg-quick 行為一致（Unix 用 `sh -c`，Windows 用 `cmd.exe /C`），在 helper 內以 30 秒逾時執行，輸出截斷到 1000 字元。預設關閉（設定 → 進階），開啟時顯示明顯的安全警告，因為指令以完整系統權限執行；PostUp 失敗不會中斷連線。
- **DNS leak test 增強** — 每台 DNS 伺服器顯示探測狀態（vpn / ok / leak / timeout）與延遲；Windows 收集 DNS 時同時包含 IPv4 與 IPv6。
- **開啟資料夾捷徑** — 設定中新增可點擊連結，直接開啟隧道設定資料夾與日誌儲存資料夾（跨平台）。

### 🐛 錯誤修正

- **通知持續時間設定無法儲存** — 離開設定再進入時不再重置。
- **設定中日誌分級缺少 All** — 下拉新增 `All`（與 Log 介面預設一致），源頭不再過濾任何記錄。

### 🛠 內部

- **日誌級別 All 全鏈路生效** — helper 與 GUI 日誌處理器均支援 `all`（`slog.Level(-8)`），不會丟棄任何記錄。
- 版本提升至 **1.1.5**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.3] - 2026-08-30

本版本修復 Windows 用戶端自動更新失效的問題：自 v1.1.0 資產改名以來，Windows 發行資產（`wireguideplus-<arch>-installer.exe` / `wireguideplus-<arch>-portable.zip`）命名不含作業系統識別，而更新檢查器要求資產名稱同時攜帶「OS 識別 + 架構」，導致 Windows 平台永遠比對不到自己的發行資產，已安裝用戶只會看到「發現新版本但無相符資產」，無法自動更新。

### 🐛 錯誤修正

- **修復 Windows 自動更新資產比對失效** — `matchAsset`（`internal/update/checker.go`）在 Windows 平台下額外接受「架構錨定 + Windows 專屬副檔名」（`.exe` / `.msi` / `.zip`）的資產名稱，無需 OS 識別；macOS / Linux 資產仍須攜帶各自 OS 識別（`darwin` / `linux`），因此不會誤比對 Windows 的無識別資產。新增回歸測試涵蓋三種架構的正常比對，以及 Linux / macOS 不得接受無識別 Windows 資產名稱的反向斷言。

### 🛠 內部

- 版本提升至 **1.1.3**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.2] - 2026-08-30

本版本修復 Windows 安裝包檔案版本錯位的問題：先前發布的 1.1.1 安裝包中，執行程序（`wireguideplus-<arch>.exe`）在檔案總管內容頁顯示的「檔案版本」為 **1.1.0.1**（應為 **1.1.1.0**）。

### 🐛 錯誤修正

- **修復 Windows 執行程序檔案版本錯位** — 根因：`goversioninfo v1.7` 將 `FixedFileInfo` 結構宣告為 `Major/Minor/Patch/Build` 順序（與 Windows 標準配置的 Build/Patch 相反），向 JSON 顯式寫入數字版本會得到被交換的二元版本（`1.1.1.0` 變成 `1.1.0.1`）。現在 `build/windows/versioninfo.json` 的 `FixedFileInfo` 數字固定為 0，僅以 `StringFileInfo` 四段版本字串為唯一輸入，由 goversioninfo 推導二元版本（與配置無關、始終相符）；`tools/genverinfo` 只渲染字串版本，`tools/bumpversion` 不再觸碰數字欄位。已驗證：傳入 `1.1.2.0` 字串時 goversioninfo 輸出 `FixedFileInfo.FileVersion (1.1.2.0)`，安裝後內容頁與 `FileVersionInfo` 均正確顯示。

### 🛠 內部

- 版本提升至 **1.1.2**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh` + `project.nsi`）、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。
- 修正 NSIS 安裝/解除安裝描述（`project.nsi`），安裝包與解除安裝程序之檔案版本資訊與執行程序一致。

## [1.1.1] - 2026-08-30

本版本修復 Windows 系統匣通知氣泡「開啟主視窗」按鈕在系統高負載下偶發導致 GUI 卡死的問題。

### 🐛 錯誤修正

- **修復通知氣泡「開啟主視窗」偶發卡死** — 當系統 CPU 爭用激烈（例如 Windows 維護程序佔滿核心）或 WebView2 回應延遲時，點擊系統匣通知氣泡的「Open Window」按鈕會同步阻塞等待 UI 執行緒，整個 GUI 看似凍結（VPN 隧道不受影響）。`showDock`（`internal/gui/dock_other.go`）改為經 `application.InvokeAsync` 在 Wails UI 執行緒非同步執行：呼叫端立即返回，視窗顯示/聚焦皆在 UI 執行緒內聯完成，不再跨執行緒等待；同時加 recover 防護，意外 panic 不會打斷主執行緒回呼鏈。

### 🛠 內部

- 版本提升至 **1.1.1**：`internal/update/checker.go` 主版本、`build/config.yml`、`windows/info.json`（`1.1.1.0`）、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh`）、MSIX、Linux nfpm、`tools/genverinfo` 全部同步。

## [1.1.0] - 2026-08-28

本版本專注於可辨識度、代理（Proxy）的穩健性與啟動時的自動化規則評估：系統匣狀態改用高對比文字符號、代理提供三種明確模式並附連線測試、無效的代理 URL 不再導致更新檢查失敗、啟動時會在連線前先評估自動化規則。

### ✨ 功能

- **系統匣狀態符號** — Windows 系統匣選單的連線狀態改用純文字符號：`●` 實心 = 已連線、`○` 空心 = 已斷線（Windows 系統匣彈出選單由 GDI 繪製，無法顯示彩色 emoji — `🟢` 會退化為灰色輪廓，新舊狀態難以分辨）；macOS 選單列（原生 AppKit 繪製）維持彩色 emoji。啟動/過渡狀態有各自的標記。
- **代理模式與連線測試** — 設定 → 代理現在提供三種明確模式：**直接連線**（完全忽略系統/環境代理）、**GitHub 鏡像**（`mirror`，例如 `https://ghfast.top` 加速前綴）、**手動代理**（`manual`，完整的 http/https/socks5 URL）。新增**「測試連線」按鈕**：儲存前先對 GitHub Releases API 發出往返請求，並回報成功與延遲。
- **代理立即生效** — 儲存代理設定後，下一次排程更新檢查（以及手動「立即檢查」）無需重新啟動即生效；GUI 啟動時也會直接套用已儲存的代理，避免「啟動當下用壞掉的設定檢查一次」。

### 🐛 錯誤修正

- **無效的代理 URL 不再導致更新檢查失敗** — `config.json` 中損壞的手動代理（例如 `proxy_url = "https://"`）過去會直接被 `http.ProxyURL` 取用，導致每次更新檢查都以 `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify must be specified in the tls.Config` 失敗。現在 URL 會在啟動時與每次使用時驗證（`internal/update/proxy.go`）；無效值會記錄 `WARN update: ignoring invalid manual proxy URL` 並退回直接連線 — 檢查不再失敗。
- **修正「先連線、後被規則切斷」的啟動感受** — 啟動規則評估移到 helper 啟動之後立即執行（記錄 `startup rule re-evaluation`），讓每條隧道的目標狀態先由規則決定；另外加入 `scheduleRuleCheck` 備援：啟動後 60 秒內，任何由 RPC 驅動的手動連線（例如還原上次工作階段）會在 3 秒後依規則重新評估並修正，不必等待 30 秒輪詢；觸發來源也會記錄下來以便除錯。
- **無效的鏡像前綴不再默默破壞檢查** — `mirror` 模式的加速前綴同樣會驗證 scheme/host；非法值退回官方 API 端點。

### 🛠 內部

- 版本提升至 **1.1.0**：`internal/update/checker.go` 主版本、`build/config.yml`、`windows/info.json`（`1.1.0.0`）、`windows/wails.exe.manifest`、NSIS、MSIX 與 Linux nfpm 全部同步。
- **Windows 版本資源標準化** — `wails3 generate syso` 產生的資源語言為 `0x0000`，且 `VS_FIXEDFILEINFO.ProductVersion` 為零，導致 Windows 檔案總管 / `FileVersionInfo` 無法讀取（內容頁版本欄空白）。改用 `goversioninfo`（設定：`build/windows/versioninfo.json`）產生標準的 `0409/04B0` 資源；`generate:syso` 任務已同步更新；exe 與安裝程式的內容現在能正確顯示 `1.1.0`。
- **新增 Windows x86（32 位元）建置** — `task windows:build ARCH=386` 產生 32 位元二進位檔與 `wireguide-x86-installer.exe` 安裝程式（NSIS 腳本支援 x86、安裝至 Program Files、內含 x86 版 `wintun.dll`）。
- **平台邊界釐清** — 移除 iOS 建置任務與設定註解；Android / iOS 不受支援（無法同時多隧道、無法依 Wi-Fi SSID 自動連線）；README 已說明；macOS / Linux 增強版開發中。
- **系統整合強化** — 新增**「最小化啟動」**設定（直接進入系統匣、不顯示主視窗；設定 → 啟動）；新增**系統匣連線通知**：啟動 10 秒後顯示目前連線狀態，網路變更（Wi-Fi 切換、拔除網路線、網路中斷等）導致隧道連線狀態改變後，也會延遲 10 秒顯示穩定後的最新狀態。氣泡通知含動作選單（開啟主視窗 / 斷線），可手動關閉，或在設定的停留時間（預設 10 秒，可在設定 → 啟動 → 通知停留時間調整；`internal/gui/notify_windows.go`）後自動關閉。
- **雙架構發布** — 每次建置同時產生 32 位元（x86）與 64 位元（amd64）的二進位檔與安裝程式（`task windows:build:all`，自動更新 wintun.dll 架構）；應用程式/安裝程式說明統一為「多隧道 + 自動化」，移除跨平台用語。
- **安裝體驗** — 安裝程式預設安裝至 Program Files（32 位元安裝程式自動選擇 Program Files (x86)），安裝過程中可變更目錄；預設建立「開始」功能表捷徑（含「解除安裝 WireGuide Plus」項目，圖示與應用程式一致），可在「捷徑選項」頁面取消勾選；一律建立桌面捷徑（`build/windows/nsis/project.nsi`）。
- **開發與發布文件** — 建置/封裝文件從 README 移至獨立的 `docs/DEVELOPMENT.md`；GitHub Release 工作流程現已包含 32 位元 Windows 產物與 CI 工具鏈（`goversioninfo`）；推送本機 `v*` 標籤即自動建置（Windows x86+amd64、macOS arm64、Linux amd64+arm64）、簽署並發布（`docs/release.md`）。
- 調整 Windows 介面卡名稱比對（`internal/wifi/known_windows.go`、`detect_windows.go`），物理介面卡偵測更精確。
- 視窗標題統一為 **WireGuide Plus**。
- 更新檢查在排程器內去重，避免一輪內多次觸發（失敗只記錄一次，含重試間隔）。

## [1.0.0] - 2026-08-28

里程碑版本：A11y 無障礙語意重構、Windows 網路出口路由邏輯變更、Wails3 建置/圖示/權限清理，加上簡體中文介面與系統匣開關。

### ✨ 功能

- **簡體中文介面（Chinese UI）** — 全介面完整的簡體中文翻譯，涵蓋全部 199 個字串：隧道清單、歷史、工具（DNS 洩漏測試 / 路由表）、日誌、設定、更新、自動化編輯器。首次啟動自動跟隨系統語言（偵測 `zh-*` 地區），或在設定 → 一般 → 語言中手動切換（持久化）。
- **系統匣開關** — 系統匣中的每條隧道現在都是獨立的可點擊開關：勾選連線、取消勾選斷線；連線 emoji（🟢 已連線 / 🟡 連線中 / ○ 已斷線）保留在標籤旁。手動斷線的隧道在重新連線或 WireGuide 重新啟動前，保持不受自動化規則影響（manual-off）。

#### 前端 A11y 無障礙重構

> 範圍：所有平台（Windows/macOS/Linux）的 Svelte 前端，不限於 Windows。

- 所有 modal 覆蓋層的底色（scrim）移除 `role="button"` 與 `tabindex="0"`，還原為純遮罩，螢幕閱讀器不再把全螢幕背景視為可互動按鈕。
- 所有對話框使用 `tabindex="-1"`，並維持標準的 `role="dialog" aria-modal="true"`，符合 WCAG 對話框語意。
- ESC 關閉統一：原本沒有的對話框（匯入結果、歷史、更新通知、自動化編輯器）改在元件最上層掛載 `<svelte:window on:keydown>`（處理器會檢查對話框狀態；Svelte 不允許在 `{#if}` 內掛載）；其餘沿用 App.svelte 的全域捕捉處理器 — 避免多對話框 ESC 衝突，也不破壞 CodeMirror 的按鍵捕捉。
- `Settings.svelte`：`<nav role="tablist">` 改為一般 `<div>`，消除頁籤語意警告；分隔條維持 `role="separator"`，但加上 `tabindex="0"` 與真正的鍵盤操作（方向鍵調整大小、Enter/Space 重設）。
- `frontend/vite.config.js`：svelte 外掛的 `onwarn` 過濾靜態誤報（`a11y_click_events_have_key_events`、`a11y_no_static_element_interactions`、`a11y_no_noninteractive_tabindex`、`a11y_no_noninteractive_element_interactions`）；生產建置警告歸零，無邏輯變更。
- 涉及檔案：`src/App.svelte`、`src/lib/History.svelte`、`src/lib/ConflictWarning.svelte`、`src/lib/TunnelDetail.svelte`、`src/lib/UpdateNotice.svelte`、`src/lib/Settings.svelte`、`src/lib/AutomationEditor.svelte`

#### Windows 背景 helper：網路出口路由邏輯

> 範圍：僅 Windows 的 Go helper 程式碼；其他作業系統不受影響。

- helper 啟動時會捕捉主要上游物理介面卡的 LUID，記錄系統初始的預設出口物理介面；此 LUID 是開機時的快照，不會在執行期間的網路切換時自動更新。
- 修正網路介面篩選邏輯：排除 TUN/隧道/回環虛擬介面卡，僅物理介面卡可作為上游候選；不再把 TUN 虛擬介面卡當作物理介面卡綁定/鎖定。
- WireGuard UDP 出口完全交由 Windows 路由表 + 各介面卡的 InterfaceMetric 躍點數決定；軟體不再強制綁定固定物理介面卡。
- 加入分流（`full_tunnel=false`）邏輯約束：Peer Endpoint IP 必須明確包含在 `AllowedIPs` 中，避免握手 UDP 封包被路由丟棄（`no-handshake`）。
- 日誌：`network primary upstream interface initial luid` 輸出主要物理介面卡 LUID 供除錯；日誌中的 `tunnel connected` 僅代表 TUN 介面卡就緒，不代表遠端 peer 握手成功。
- 除錯提示：在 Windows 上請優先使用 `Find-NetRoute -RemoteIPAddress <peer-ip>` 判斷目標 IP 的實際出口介面卡；PowerShell 的 `Get-NetAdapter.Luid` 是結構體，無法直接與 Go 的 uint64 輸出比較。

### 🛠 建置與專案

大部分是 Windows 建置行為；跨平台部分已標註。

1. **Wails3 Windows 圖示建置行為**（僅 Windows）— `task build` 完整建置會自動執行 `wails3 generate icons`，讀取 `build/appicon.png` 並覆寫 `windows/icon.ico`；手動編輯的 `windows/icon.ico` 會被完整建置覆寫。`windows/icon.ico` 是最終嵌入 exe 的圖示；`build/appicon.png` 只是來源素材；`task windows:build` 偵錯建置會略過圖示產生、保留現有 `windows/icon.ico`。exe / 視窗標題列 / 工作列圖示皆重用 exe 內的 ico 資源；系統匣圖示需要另外的 Go `embed` 資源。
2. **Windows 版本資訊管理**（僅 Windows）— exe 檔案詳細資料來自 `windows/info.json`；`FileVersion` 必須是 4 段數字 `major.minor.patch.build`。介面顯示的版本由 Go 常數（`internal/update/checker.go`）維護，需與 `info.json` 手動保持同步；日後可透過 ldflags 建置時注入達成單一版本來源。
3. **Windows UAC / 系統管理員權限**（僅 Windows）— 目前架構：GUI 啟動 helper 子程序；helper 操作 TUN 介面卡並修改路由需要管理員權限，而提升子程序權限會觸發 UAC 提示 — Windows 的安全性無法完全靜默繞過。短期：`windows/wails.exe.manifest` 加入 `requireAdministrator`，將 UAC 提示移至雙擊 exe 啟動時（仍需使用者確認）；長期：將 helper 重構為 Windows 系統服務（LocalSystem、背景執行），GUI 以一般使用者身分透過 IPC 通訊，完全消除 UAC 提示。

### 🐛 調查

調查筆記，無程式碼變更，供開發者參考。

- 症狀：helper 記錄 `tunnel connected`，但 GUI 顯示 `no handshake`。
  - 根本原因：建立 TUN 裝置 ≠ 與遠端 peer 完成 WireGuard 加密握手；請讀取 wg 核心的 `latest handshake` 狀態來判斷真實連線狀況。
  - 分流的陷阱：Peer IP 不在 `AllowedIPs` 中 → 握手 UDP 封包被路由丟棄。
  - 其他可能：Windows 對外防火牆阻擋 WireGuard UDP、端點網域 DNS 解析失敗。
- 監聽於 `0.0.0.0` 的本機代理：代理程序的流量是獨立的，不會自動流入 WireGuard 隧道；流量方向由 Windows 路由表與隧道的 `AllowedIPs` 共同決定。

### 📝 附註

1. **變更範圍**
   - Svelte 前端 A11y 程式碼：**適用於所有平台（Windows / Linux / macOS）** — ESC 處理與無障礙語意會影響所有桌面平台。
   - helper 網路出口路由：**僅 Windows 的 Go 程式碼變更**；其他作業系統不受影響。
   - 建置、manifest、ico、info.json、UAC：**僅 Windows**。
2. 前端 A11y 變更與 helper 的背景網路邏輯完全解耦，不影響隧道建立、路由或自動化 Wi-Fi 規則。
3. helper 記錄的上游 LUID 只是開機時的快照，在 Wi-Fi / 有線網路切換時不會自動更新。

## [0.5.1] - 2026-08-11

Patch release: the in-app "Update Now" button is now trustworthy on macOS. If you are on 0.5.0 via Homebrew, this is also the first update the button itself should complete cleanly end-to-end.

### Fixed
- **macOS "Update Now" (issue #38)** — the in-app update can no longer report success without actually installing: after `brew upgrade` exits, the installed bundle's version is verified against the release it claimed to install, progress phases ("refreshing" / "installing") are shown in the banner and About panel, and failures surface inline instead of vanishing behind a relaunch. Also survives Homebrew 6's tap-trust gate (`untrusted tap` errors trigger a `brew trust` + one retry) and skips the redundant `brew update` (`HOMEBREW_NO_AUTO_UPDATE=1` — the checker already knows the target version).
- The Homebrew cask itself dropped `auto_updates` (korjwl1/homebrew-tap), so bulk `brew upgrade` no longer skips WireGuide — the root cause of months of silent non-updates.

## [0.5.0] - 2026-08-10

Linux graduates to a supported platform, the CLI learns to start and stop the app, and the Windows helper's IPC surface is locked down to the launching user. Verified on all three OSes before release: a full runtime pass on Windows 11 against a real tunnel (helper IPC, multi-tunnel, kill-switch cycles, CLI lifecycle, tray), the Linux plan in `docs/linux-test-plan.md` on Debian 13 / Raspberry Pi OS ARM64, and the macOS DNS/lifecycle fixes below.

### Added
- **Linux support** — tested and hardened end-to-end on Debian 13 / Raspberry Pi OS ARM64 (Wayland and X11): window decorations restored after tray-restore, gateway/physical-interface detection fingerprints the right network (issue #22), routine RTNETLINK traffic no longer registers as a primary-network change (reconnect decisions compare real default-route snapshots), nftables kill-switch fixes, DEB packaging via nfpm.
- **`wireguide ctl start` / `ctl stop`** — explicit app lifecycle from the CLI. `start` launches the app detached and waits for the helper (long deadline: the macOS admin prompt has no timeout of its own; on macOS it launches its *own* bundle rather than whatever LaunchServices resolves); `stop` quits GUI and helper together and confirms they actually went away. Deliberately the only commands that start anything — `connect`/`status` still refuse rather than boot a VPN stack behind your back.
- **`--json`** on `ctl status` and `ctl list` for scripts and coding agents.
- **CI: 3-OS test matrix** (Linux/macOS/Windows) on every PR; release workflow untouched.

### Security
- **Windows helper pipe scoped to the spawning user (issue #20)** — the named pipe's ACL now grants access to the launching user's SID instead of every interactive user, and each connection's peer SID is verified against it (SYSTEM and a helper spawned without the SID keep working). Verified live on Windows 11 by reading back the pipe's security descriptor.

### Fixed
- **Windows multi-tunnel** — connecting a second tunnel no longer fails on the Wintun adapter name collision; each tunnel gets its own `WireGuide-<id>` adapter, and multi-tunnel status reports per-tunnel interface/duration/traffic instead of zeroed copies.
- **Helper lifetime** — the helper never runs at boot and its lifetime is tied to the GUI: a 60 s startup grace covers a helper whose GUI never attaches (login-autostart with an unanswered UAC prompt no longer leaves an invisible elevated process), and a teardown that leaves no tunnels and no GUI re-arms the shutdown grace window — closing the orphan-helper hole that transient CLI connections opened (a GUI-less `ctl disconnect` of the last tunnel previously left the elevated helper alive until reboot). CLI clients are excluded from connection-lifecycle tracking by design.
- **Kill switch** — rebuilt atomically around every connect/disconnect from actual manager state; a failed connect restores the blockade instead of leaving it half-applied.
- **macOS DNS teardown (issue #34)** — search domains, services added mid-session, and the failed-verify / ForceShutdown paths now all restore DNS.
- **macOS updates (issue #38)** — "Update Now" runs `brew upgrade --greedy` so cask-held updates can't silently no-op.
- **Diagnostics (issue #32)** — ping parsing is locale-agnostic (Korean Windows included), and unreachable hosts report as unreachable instead of a fabricated wall-clock-derived latency.
- **Automation** — rules are validated on save: a malformed CIDR or MAC is rejected with a clear error instead of being written and silently never matching.
- **Idle efficiency** — Wi-Fi polling backs off to 60 s while native change notifications are attached; config-file watching drops from 1 s to 3 s; endpoint-latency logging demoted to debug.

### Removed
- Key generator, CIDR calculator, speed test, mini mode, and the split-tunnel UI stub — dead or abandoned surfaces found in the audit sweep (#35); their bindings and i18n strings went with them.

## [0.4.2] - 2026-07-27

**Urgent fix release for Windows users.** 0.4.1 and earlier shipped with a tray that could permanently lose the main window and an installer that cannot upgrade in place while the app is running. Windows users should update; to get past the installer bug one last time, run `taskkill /F /IM wireguide.exe` from an elevated terminal before launching the 0.4.2 installer. macOS and Linux are unaffected by the tray-window bug (Linux picks up the same Show Window fix), and nothing else changed.

### Fixed
- **Windows tray, issue #30** — left-clicking the tray icon now shows the main window (the platform convention; previously a no-op), and the "Show Window" menu item actually works: it was wired to a macOS-only implementation, so on Windows **a window closed to the tray could never be reopened** — the only recovery was killing the process. The tray menu also showed stale connection state (○ while connected) because menu refills never reached the Win32 popup; the menu now rebuilds through `SetMenu` on every change. macOS behavior is unchanged; Linux gains the same Show Window fix.
- **Windows installer, issue #29** — upgrading by running the installer while WireGuide was running failed with "Error opening file for writing: wireguide.exe" (the GUI and the elevated helper are the same executable, and Windows locks running images; the helper deliberately outlives the GUI, so quitting the tray app wasn't enough). The installer and uninstaller now terminate running instances before touching files. **This fix takes effect when the 0.4.2 installer runs — upgrading *to* 0.4.2 still hits the old installer's bug**, hence the elevated `taskkill` workaround above.

## [0.4.1] - 2026-07-27

### Fixed
- **Automation (GUI), issue #27** — creating or editing rules in the Automation editor was effectively impossible in 0.4.0: the editor's own autosave re-fired the config watcher, and the resulting reload wiped the just-added row before it could be filled in (and could transiently delete a rule being edited). The editor now ignores its own writes (reloading only when the file genuinely changed externally), a blank draft row is no longer autosaved, and a rule that is momentarily incomplete mid-edit keeps its last saved value on disk instead of being deleted. External edits (`wireguide ctl`, another window) still appear live.
- **Automation (GUI)** — per-tunnel rule saves now go through the cross-process-locked settings update instead of a whole-settings overwrite, so a GUI rule edit can no longer clobber a concurrent `wireguide ctl` change to any other setting (and vice versa); condition labels survive the GUI round-trip; a dash- or bare-hex-formatted gateway MAC written by the CLI is no longer treated as a foreign change.
- **Windows (dev):** `go test ./internal/ipc` no longer fails/panics when run unelevated — the tests accept the test binary's own pipe (test builds only; the production SY/BA pipe-owner check is unchanged) (#24).

## [0.4.0] - 2026-07-15

### Added
- **Automation** (issue #12) — per-tunnel `condition → action` rules that connect or disconnect a tunnel based on the network you're on. Conditions: Wi-Fi SSID, subnet (CIDR), or the default-gateway MAC (a precise, medium-agnostic network fingerprint that tells apart networks sharing a subnet); action: connect/disconnect. Rules are ordered by priority (drag-to-reorder, first match wins) and evaluated entirely in the helper via a hybrid trigger (macOS route-monitor subscription; 30 s poll on Windows/Linux). Replaces the legacy per-tunnel Wi-Fi auto-connect / trusted-SSID UI (migrated automatically). Editable in the GUI or via the CLI.
- **Command-line interface** `wireguide ctl` (issue #10) — a third IPC client alongside the GUI (Tailscale-style): `status`, `list`, `connect`, `disconnect`, `import`, `rename`, `delete`, and `automation add/rm/rules` + a read-only decision preview. No per-command sudo, cross-platform, shares the GUI's tunnel store.
- Tunnel-list **sorting** (name / last used / date added, active-on-top) and **compact mode** (issue #16, #17); **drag-resizable** tunnel-list column.

### Fixed
- **update:** the Ed25519 signature is now bound to the hash actually installed (a repo-write attacker could previously pass both checks by swapping SHA256SUMS between check and download); `Install` also enforces `SignatureVerified` in signed-update builds.
- **Windows:** `findInterfaceMTU` buffer overflow + wrong `NlMtu` offset (undefined behaviour on every no-MTU connect; auto-MTU always fell back).
- **Linux:** split-tunnel routes were deleted from the wrong table on the default `Table=auto` path (route leak); DNS search-domain injection; nft kill-switch endpoint-port validation and `oifname` consistency.
- **macOS:** `route -n monitor` subprocess is now supervised (was a silent zombie + stuck monitor on unexpected exit); the tray menu-bar icon uses native click-to-open (fixed the "does nothing on macOS 26" report, issue #18) and follows the menu bar's actual appearance; the connect/Disconnect-race no longer holds `Manager.mu` across slow teardown.
- **storage:** reject case-collisions and Windows reserved names; fsync the parent directory after atomic writes; latency-probe target validation; meta-sidecar lost-update race.
- **Automation (code review, issue #12):** `else`/none_match now matches at its own position so drag-to-reorder priority is uniform (was always held to the end); malformed conditions and unknown actions now fail closed (rule skipped) instead of an unknown action defaulting to connect; a rule-driven connect now runs the same DNS-protection + kill-switch folding as a manual connect (headless automation could previously connect with no protection, or fail entirely under an already-on kill switch), and a rule-driven disconnect strips the tunnel from the kill-switch filter set; macOS no longer overwrites the GUI-reported SSID with an empty root-helper poll (which silently broke SSID rules); Windows gateway-MAC resolves the physical underlay gateway (excluding the WireGuard adapter) so a full tunnel no longer blanks the fingerprint and flaps `mac:` rules; tunnel rename/delete now carry/drop the tunnel's automation rules instead of orphaning them; the rule editor no longer races a debounced save against a tunnel switch. *(Windows gateway change compiles but is unverified on a Windows build.)*
- **config.json:** cross-process read-modify-write is now atomic (file lock) so a `wireguide ctl` edit and a GUI edit can't clobber each other.
- **CLI (issue #10):** `import`/automation edits work on a fresh install (dirs created); `set` exits nonzero when the helper is running but the live apply fails; `delete` refuses to remove a still-connected tunnel whose disconnect failed; `install-skills` writes agent files atomically. The NSIS installer PATH edit no longer interpolates the install path into a PowerShell command (injection), and the macOS cask + Windows installer put `wireguide` on `PATH`.
- **list:** date-added sort now uses a stamped creation time (survives edits) instead of the `.conf` mtime (issue #17).

### Changed
- Latency probe no longer fabricates a `x.x.x.1` gateway target (issue #15); per-tunnel latency target added.

## [0.3.1] - 2026-05-26

### Added
- **Full-tunnel routing-loop protection (Windows + macOS)** — multi-layer defense against the encrypted-UDP-loops-through-tunnel-adapter class of bug (issue #14).
  - Windows: WFP block at `ALE_AUTH_CONNECT_V{4,6}` + `OUTBOUND_TRANSPORT_V{4,6}` layers, iphlpapi-based `/32` bypass host route with `InitializeIpForwardEntry`, `IP_UNICAST_IF` UDP socket binding with `NotifyRouteChange2`-pushed re-pin monitor, runaway-TX watchdog with sustained-asymmetry trip.
  - macOS: `/32` bypass installed before `/1` split routes with fail-fast preflight on missing default gateway, 5 s underlay-detection retry, blackhole fallback on gateway loss inside `reapply` to keep the loop class contained when the upstream gateway briefly disappears, runaway-TX watchdog via `netstat -ibnI`.
- **SignPath Foundation code signing** — CI hooks for SignPath OSS signing of the Windows installer; gated on the foundation's onboarding approval. Releases ship unsigned until then.

### Fixed
- Helper now exits within ~20 s of the GUI dying (was ~70 s) — IPC read deadline trimmed to 10 s now that the GUI's 5 s health-monitor ping cadence is the canonical liveness signal.
- macOS: `RestoreDNS` no longer fires a noisy `netsh`-equivalent against an adapter that's already been detached from the IP stack during disconnect.
- macOS: `getDefaultInterface()` now parses the `netstat -nr` header dynamically; previously the "first lowercase field" heuristic could misidentify `awdl0` (AirDrop) as the default interface on some machines.
- Windows: UAPI listener "may not work" warning downgraded to DEBUG on Windows — the named-pipe bind is expected to fail because the helper runs as an elevated user rather than as `LocalSystem`; status queries route through the in-process `Engine.IpcGet` regardless.

### Changed
- CI release notes generated by `git-cliff` (fuller diffs than the previous auto-generated body).
- CI: explicit NSIS install on Windows runners (the default Windows-latest image no longer carries `makensis` on PATH).
- CI: `Get-FileHash` / `Expand-Archive` in the wintun vendoring step replaced with direct .NET APIs to avoid PowerShell version skew on the runner.
- README: `Install` section moved above `Features`, code-signing dev-process notes trimmed to user-facing status only.

## [0.3.0] - 2026-05-25

### Added
- **Windows kill switch via WFP** — Windows Filtering Platform-based kill switch that survives helper restarts; complements the existing macOS `pf` and Linux `nftables` implementations.
- **Periodic auto-update scheduler** — background check for new releases on a configurable cadence (default 24 h with focus-opportunistic refresh), separate from the existing manual "Check for updates" path.
- **CI release pipeline** — automated darwin (arm64) + Windows (amd64/arm64) builds on tag push, with SHA256SUMS, Ed25519 signature, and `homebrew-tap` cask auto-bump.

### Fixed
- macOS kill switch: `pf` anchor renamed from `com.apple.wireguide` (dot) to `com.apple/wireguide` (slash) so it actually matches the `anchor "com.apple/*"` wildcard in the system `/etc/pf.conf` — previously the rules loaded without ever being evaluated.
- macOS kill switch can now be toggled on without an active tunnel (base block-all set installs cleanly; per-tunnel permits are folded in on subsequent connects).
- Windows disconnect: lingering wintun adapter "defanged" (DNS cleared, metric bumped) before `engine.Close`, so the brief window where Windows still treats the dying adapter as a viable metric-1 path doesn't dump every DNS query onto its dead `8.8.8.8` binding.
- Windows disconnect: dead 12 s DNS-restore call removed; `netsh` output now decoded as the OEM code page so Korean / non-English Windows installs no longer mis-parse error messages.
- Windows: UAPI bypass (status queries served by in-process `Engine.IpcGet` rather than the named pipe that the elevated helper can't bind under the kernel's owner-SID requirement).
- Windows: suicide-reconnect / orphaned `conhost` / dangling route fixes from the WFP kill-switch rework.
- DNS protection regression introduced during the periodic-update-scheduler refactor.
- Numerous race conditions, leak fixes, and audit findings from the cross-platform hardening pass.

### Changed
- Tray and taskbar icons: rounded silhouette via custom genicon (matches the macOS dock icon's visual weight).
- Sidebar dividers, tool pages, and drop affordance polished.
- Settings: maintainer credit added in footer; helper SIGTRAP fix.
- Rebrand: WireGuide red accent + Material-style flat buttons.

## [0.2.0] - 2026-05-05

### Added
- **Wi-Fi auto-connect rules** — per-tunnel SSID-based auto-connect/disconnect; rules fire in the helper so they work even when the GUI is quit
- **Trusted SSID support** — designated SSIDs auto-disconnect all VPN tunnels (home/office network detection)
- **macOS 14+ Location Services integration** — CoreWLAN CGo replaces `networksetup` for SSID detection; app now appears in System Settings → Location Services
- **GUI→Helper SSID forwarding** — on macOS 14+ the helper (root LaunchDaemon) cannot read SSID itself; the GUI polls via CoreWLAN and forwards changes over IPC so auto-connect rules fire correctly
- **Ed25519 signature verification** — auto-update downloads verified against a Ed25519 signature over SHA256SUMS; embedded public key prevents tampered binaries from being installed

### Fixed
- Wi-Fi auto-connect status not updating in GUI/tray after rule fires (`ActiveTunnels` now populated in all status broadcasts)
- `autoConnectedBy` accessed under wrong mutex in `handleRename` (race condition; changed to `wifiMu`)
- Lock ordering violation between `handleRename` and `handleSSIDChange` that could cause deadlock
- Kill switch and DNS protection handlers using `Status().State` instead of `IsConnected()` (broke in multi-tunnel setups where the primary was not the connected tunnel)
- `handleReportSSID` panic on nil `wifiMon` (non-darwin builds and pre-init race)
- `sleep_darwin.go` unsafe.Pointer misuse flagged by `go vet`; replaced with `runtime/cgo.Handle`
- Duplicate SSID appearing in Wi-Fi rules dropdown when current SSID matched a saved rule

### Changed
- Auto-connect logic moved to helper process (was frontend-side) so rules fire independently of GUI lifecycle
- `postConnectRefresh` refactored: `refreshTunnels`+`refreshStatus` kept for manual connect UX; auto-connect path calls only `applyFirewallSettings` (event stream handles status update)
- Dead backward-compat fallback in `subscribeToEvents` removed (active_tunnels now always populated)

## [0.1.9] - 2026-05-05

### Changed
- Removed Wi-Fi rules master toggle; trusted SSIDs are always active when configured

### Fixed
- Various regressions, lifecycle, and performance issues from audit rounds (Round 2, Round 3)
- 30+ fixes from full-codebase review (null guards, lock safety, error propagation)

## [0.1.8] - 2026-04-13

### Changed
- Sidebar navigation: removed Tools tab bar, DNS Leak Test and Route Table are now direct sidebar sub-items
- Settings modal: fixed size regardless of active tab (no more resize when switching to Advanced)
- Settings sidebar active state: tint highlight instead of solid blue (macOS HIG)
- Dropdown controls: custom styled per macOS HIG (28px height, 6px radius, theme-aware chevron)

### Improved
- Route table: sticky column header, legend pinned to bottom, table fills remaining space with scroll
- DNS Leak Test and Route Table now call real backend (previously stub implementations)
- macOS HIG design tokens: added `--border-strong` for input control borders

### Removed
- Network Diagnostics (Ping) tool — not meaningfully useful as a standalone feature
- Unused i18n keys for removed Diagnostics feature

## [0.1.7] - 2026-04-09

### Added
- Multiple simultaneous tunnel support
- Per-tunnel NetworkManager (independent routes, DNS, route monitor per tunnel)
- Per-tunnel health check and reconnection
- Full-tunnel conflict detection (reject two 0.0.0.0/0 configs)
- DNS union across all active tunnels
- No-handshake warning: orange dot in tunnel list, ◐ in tray menu
- Tray menu shows per-tunnel connection + handshake status
- Architecture & design documentation (docs/DESIGN.md)

### Fixed
- Disconnect one tunnel no longer breaks other active tunnels
- Conflict detection: macOS netstat abbreviated CIDRs now parsed correctly
- GUI not reflecting connection state when tunnel connected via system tray
- Bypass route race conditions (lock safety, error propagation)
- Tray icon padding: trimmed transparent pixels for tighter menu bar fit
- Tunnel list unnecessary re-renders on every status tick
- README streamlined: removed defensive tone, screenshots moved to top

### Changed
- Pin Interface toggle added (Settings > Advanced) for dual-network stability
- Bypass routes pinned to upstream interface with -ifscope when enabled

## [0.1.6] - 2026-04-08

### Added
- Settings redesign: split layout with sidebar (General / Advanced / About)
- About tab: app icon, version, GitHub/Issues/License links, update status
- Update popup: modal with release notes ("What's New") and "Skip This Version"
- Helper auto-upgrade: detects version mismatch and reinstalls on app update
- Helper install retry dialog with Quit/Retry options on cancel
- OpenURL Wails binding (restricted to github.com)
- Tests for IsBrewInstall and OpenURL validation (7 new tests)

### Fixed
- Brew install detection: check Caskroom receipt instead of binary path
- Non-brew update: opens GitHub Releases page instead of broken auto-download
- Brew update: runs `brew update` before `brew upgrade` for third-party taps
- Helper Ping response: separate AppVersion field (fixes IPC protocol validation)
- Update popup double-click guard
- localStorage exception handling for skip version
- Detailed admin prompt explaining why password is needed

### Changed
- README/About description: "native macOS" → "cross-platform"

## [0.1.5] - 2026-04-07

### Added
- Health Check toggle in Settings (default: off, recommended with PersistentKeepalive)

### Changed
- Health Check default changed from on to off (consistent with other WG clients)
- README rewritten: removed aggressive tone, verified claims, acknowledged official app works for many users

## [0.1.4] - 2026-04-07

### Security
- Remove script execution (PreUp/PostUp/PreDown/PostDown) — eliminates local privilege escalation via ApproveScripts RPC
- Fix Windows IPC ACL: allow non-admin GUI to connect to helper pipe
- Harden update integrity: asset size validation + Content-Length check

### Fixed
- Kill switch pf rules: use anchor-only approach instead of modifying main ruleset (fixes Tahoe compatibility)
- Kill switch + DNS protection now toggleable while VPN is connected
- Kill switch reconnect deadlock: suspend/resume firewall rules during reconnect
- Log viewer scroll not working
- Tunnel list scroll overflow

### Added
- Handshake-based health check: detects dead tunnels and triggers reconnect after 180s
- Instant sleep/wake detection via NSWorkspace notification (polling fallback kept)
- Typed tunnel error enums (ErrAlreadyConnected, ErrNetwork, etc.)
- DNS post-write verification
- Crash recovery journal with pre-modification DNS snapshot
- Comprehensive unit tests (102 tests, race-clean)
- CHANGELOG.md
- Info-level logs for kill switch and DNS protection events

## [0.1.3] - 2026-04-07

### Fixed
- "Show Window" not working after closing the window (RegisterHook instead of OnWindowEvent)
- Dock icon hide/show when window is closed/reopened
- App icon showing Wails default (white W) instead of WireGuide red icon
- About/Settings dialog showing wrong version — now fetched dynamically from Go

### Added
- GitHub issue templates (bug report, feature request)
- CONTRIBUTING.md and PR template

## [0.1.2] - 2026-04-07

### Fixed
- Dock icon not hiding when window is closed
- Tunnel list not updating after rename

## [0.1.1] - 2026-04-06

### Fixed
- Daemon socket directory permissions (0700 → 0755)
- LaunchDaemon install flow rewrite (app first-launch, not cask postflight)

### Added
- Version display in Settings

## [0.1.0] - 2026-04-05

### Added
- Initial release
- WireGuard tunnel management (import, create, edit, export .conf files)
- Config editor with CodeMirror 6 syntax highlighting and autocompletion
- System tray with connection status badge
- Kill switch via macOS pf
- DNS protection (force DNS through VPN tunnel only)
- Auto-reconnect with exponential backoff
- Sleep/wake recovery
- Route monitor for gateway changes
- Conflict detection (Tailscale, other WG interfaces)
- Network diagnostics (ping, DNS leak test, route table)
- Auto-update (GitHub Releases + Homebrew)
- Real-time RX/TX speed graph
- i18n (English, Korean, Japanese)
- Dark / Light / System theme
