# Changelog

All notable changes to WireGuide Plus will be documented in this file.

> English: [CHANGELOG.en.md](CHANGELOG.en.md) · 繁體中文: [CHANGELOG.zh-TW.md](CHANGELOG.zh-TW.md) · 日本語: [CHANGELOG.ja.md](CHANGELOG.ja.md) · 한국어: [CHANGELOG.ko.md](CHANGELOG.ko.md)

## [1.3.0] - 2026-09-01

本版本将应用更名为 **WireGuide Plus**：窗口标题、托盘、自启动项、helper 日志、Homebrew cask、更新临时文件与 nftables 表名等全链路统一为 plus 命名，并在升级时自动清理旧版残留的启动项、守护进程与防火墙表。同时改进 macOS 托盘图标（改用应用图标红色变体）与路由诊断显示。

### ✨ 新功能

- **macOS 托盘图标改用应用图标** — 菜单栏图标使用应用图标的红色变体，在浅色 / 深色菜单栏下都清晰可辨；未内嵌图标时回退到原单色 W 模板。
- **macOS 路由诊断规范化** — `netstat -rn` 会把 127.0.0.0/8 显示成 "127"、192.168.1.0/24 显示成 "192.168.1"；诊断页现在会展开回规范点分十进制 + 前缀，显示不再像截断。

### 🛠 内部

- **全链路更名 WireGuide Plus**：macOS 自启动 `com.wireguideplus.gui`、LaunchDaemon 与 helper 日志路径、pf anchor `com.apple/wireguideplus`、Linux 桌面图标、Windows 自启动注册表值、wintun 适配器名 `WireGuidePlus-<hash>`、FWPM 会话 / Provider / SubLayer 名、nftables 表 `wireguideplus` / `wireguideplus_dns`、Homebrew cask `wireguideplus` 与 Caskroom 路径、更新临时文件与冲突检测 socket 路径、发布机密钥目录 `~/.wireguideplus`、测试环境变量 `WIREGUIDEPLUS_RESOURCE_*`、macOS 授权弹窗文案全部统一。
- **升级兼容清理**：升级 / 卸载时移除旧版残留的 `com.wireguide.gui` LaunchAgent、`com.wireguide.helper` LaunchDaemon 与 helper、旧 helper 日志、旧 pf anchor `com.apple/wireguide`、`wireguide.desktop` 自启动项、旧 wintun 适配器 `WireGuide-<hash>`、旧 nft 表与旧 FWPM Provider。
- **发布产物改名**：macOS zip / dmg 与 Linux deb 资产名改为 `WireGuidePlus-*`；NSIS PATH 提示与 MSIX 模板可执行文件名同步。
- **测试脚本同步**：systemd unit 与测试 socket 统一为 `wireguideplus-*` 前缀。

## [1.2.5] - 2026-09-01

本版本重构 DNS 泄漏检测：新增「公共 DNS 交叉验证」——测试时除本机配置的解析器外，还会向知名公共 DNS 发送探测以交叉核实；系统解析器按来源网卡分类标记「本机 / VPN / 公共」；公共列表支持从网络刷新与自由编辑。新增「浏览器检测」按钮，一键打开 browserleaks.com 做浏览器级 DNS 与 WebRTC 泄漏检测。同时修复 Windows 连接通知弹窗可能冻结无响应的问题，并更新应用图标。

### ✨ 新功能

- **公共 DNS 交叉验证** — 测试时除本机配置的 DNS 外，还会向知名公共解析器（Google、Cloudflare、OpenDNS、Quad9、阿里、腾讯 DNSPod、114DNS、百度、AdGuard、NextDNS、Comodo 及常用 IPv6 地址）发送探测，交叉核实 DNS 查询是否仍只经过隧道。公共解析器的应答仅表示「可达」，并不是泄漏。
- **从网络获取公共列表** — 点击「从网络获取」从 public-dns.info 拉取当前可靠性最高的解析器（上限 30 个，10 秒超时），并缓存上次成功获取的列表，离线时仍可使用。
- **自定义公共解析器列表** — 可自由添加 / 删除 / 编辑公共解析器条目（IP 或域名），保存在设置中；清空列表会恢复默认交叉验证列表，公共探测始终开启。
- **系统解析器分类标记** — 按来源网卡分类：物理网卡（无线 / 有线）标记「本机」，隧道接口标记「VPN」，其余为「公共」；本机解析器排在最前，并显示来源接口名（Windows 按网卡枚举 DNS，Linux 解析 resolvectl 输出）。
- **浏览器检测** — 新增「浏览器检测」按钮，一键打开 browserleaks.com 执行浏览器级 DNS 与 WebRTC 泄漏检测（将打开默认浏览器，检测数据会发送给第三方网站）。

### 🐛 修复

- **Windows 通知弹窗冻结** — 弹窗的消息循环之前没有绑定创建它的操作系统线程，goroutine 在线程间迁移后收不到点击 / 关闭 / 定时器消息，弹窗会看起来「卡死」。现已锁定线程，弹窗可正常点击关闭与自动关闭。
- **通知文本绘制加固** — 弹窗文本绘制改用 `UTF16FromString` 并处理错误，避免非法 UTF-16 字符串导致崩溃。

### 🛠 内部

- CLI `dnsleak` 命令同步增强：解析器行显示 `vpn / local / public` 标记与状态，并读取设置中的自定义公共列表。
- 泄漏判定修正：只有物理接口（非 VPN）解析器应答才判定为泄漏；VPN 解析器标记为 VPN 状态；公共解析器应答显示「正常」而非泄漏。
- 新增 `dnsleak` 探测计划与解析测试；重新生成 bindings。
- 更新应用图标（各平台），简化构建任务。

## [1.1.10] - 2026-08-31

本版本修复 1.1.9 反馈的三个界面问题并优化设置交互：DNS 泄漏测试页不再限制显示宽度并标记本机 DNS；日志级别筛选改为精确匹配；设置中的通知时长与代理选择恢复正常保存与回显，自定义镜像 / 本地代理输入框会记住上次使用的地址。

### 🐛 修复

- **DNS 泄漏测试页宽度** — 移除页面内容 640px 的最大宽度限制，与「历史」「路由」页面一样随窗口自适应铺满。
- **本机 DNS 标示** — 测试列表中的每台服务器都来自系统解析器配置（无论手动设置还是 DHCP 获得），现在每行会显示「本机」标签，便于与 VPN 提供的 DNS 区分。
- **日志级别筛选** — 点击「DEBUG / INFO / WARN / ERROR」按钮现在只显示该级别的记录（此前是「该级别及以上」，当某级别没有记录时看起来像筛选失效）。
- **通知时长设置** — 下拉选项改为与「保留日志 / 保留历史 / 语言」一致的动态选项写法，确保修改后能正确保存并在下次打开时回显。
- **代理模式回显** — 修复设置页重新打开后代理下拉框始终显示「直接」的问题（Svelte 无法追踪函数体读取的字段，导致 `<select value={函数()}>` 只在首次求值）。改为响应式计算后，选择保存的镜像 / 手动模式会在重新打开时正确显示。
- **代理地址记忆** — 切换为「自定义镜像」或「本地代理」时，输入框会自动加载上次保存过的地址（例如曾经输入并保存的镜像前缀）；没有历史记录时显示空白与提示。

## [1.1.9] - 2026-08-31

本版本修复应用内更新「下载成功却无法安装」的问题：更新流程在启动安装器之前就删除了临时下载文件，导致 Windows 上启动安装器时提示「找不到文件」并回退到浏览器页面。

### 🐛 修复

- **应用内更新无法安装** — `runUpdateNative` 原来在 `Install` 之前就执行 `os.Remove(path)` 删除临时下载的安装包，而 Windows 安装流程是直接执行该文件（`fork/exec …wireguide-update-*.exe: The system cannot find the file specified`），因此下载 100% 后必然启动失败。现已调整为安装器启动成功后再释放临时文件；Windows 上安装器运行期间文件通常被锁定、删除可能失败，但由系统临时目录自动清理，无影响。
- **需要手动升级一次** — 1.1.7 / 1.1.8 的更新流程存在同样问题；请从本版本起手动下载安装一次（设置 → 更新 → 打开发布页），此后应用内更新即可正常工作。

## [1.1.8] - 2026-08-31

本版本对齐自动化规则的判定语义与界面引导，并进一步加固编辑器对旧格式规则的兼容：规则自上而下、首个匹配生效，同一动作的条件之间为「或」关系，「否则」作为兜底应放在最后并执行相反动作；磁盘上缺失条件类型的旧规则不再触发无谓重载。

### ✨ 优化

- **自动化规则语义引导对齐** — 编辑器说明与「否则」条目文案更新：明确「否则」在上方规则均不匹配时生效、建议放在最后作为兜底、动作通常与上方规则相反（五种语言同步）。判定逻辑本身保持不变：按顺序、首个匹配生效、`otherwise` 无条件兜底——与你期望的行为一致。

### 🐛 修复

- **旧格式规则不再触发无谓重载** — 编辑器对比磁盘与本地规则时统一使用与加载相同的类型推断（缺失 `type` 的旧「否则」规则不再回退为 network），避免每次配置变更都误判为外部修改而触发一次多余重载。

### 🛠 内部

- 重新生成 bindings 并验证与 Go API 完全一致（无差异）。
- 版本号更新至 **1.1.8**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm 全部同步。

## [1.1.7] - 2026-08-31

本版本集中修复 1.1.6 反馈的问题：自动化规则不再丢失、DNS 泄漏检测补全状态与加密方式、路由表区分 VPN / 直连、日志过滤修正、通知时长与代理显示问题；并新增连接历史保留时长设置与安装完成后「运行」选项。

### 🐛 修复

- **自动化规则不再丢失（含 otherwise）** — 编辑器加载时不再把缺失条件类型的规则误判为不完整而丢弃；无法被表单表示的磁盘规则也会原样保留，杜绝「打开设置后规则消失」。
- **DNS 泄漏检测补全结果** — 每台 DNS 服务器现在正确显示探测状态（VPN / 泄漏 / 正常 / 无响应）与延迟；新增「使用中」标记指出当前实际出口 DNS。
- **DNS 加密方式探测** — 检测每台解析器支持的传输：明文 UDP/53、DoT（TCP/853 TLS）、DoH（TCP/443 候选），并在检测后给出结果解读与防泄漏建议（使用 VPN DNS、加密 DNS、全隧道模式等）。
- **路由表区分 VPN / 直连** — 后端按活动隧道接口权威标记 `is_vpn`，路由明细正确显示 VPN / Direct 徽章，不再依赖接口名猜测。
- **日志过滤修正** — 日志事件补传 `category` 字段，分类筛选真正生效；级别/分类按钮显示各档计数，直观看出当前日志分布。
- **通知持续时间设置** — 修复下拉框在部分 Svelte 版本下渲染空白、无法显示所选时长的问题。
- **代理显示一致性** — direct 模式下不再残留代理地址；CLI 修改代理模式后设置界面实时同步。

### ✨ 优化

- **连接历史保留时长** — 设置 → 高级新增「历史记录保留时长」（默认 7 天，可关闭），超出自动滚动清理（仍保留 200 条硬上限）。
- **安装完成提示运行** — Windows 安装器完成页新增「运行 WireGuide Plus」选项（默认勾选）。

### 🛠 内部

- 版本号更新至 **1.1.7**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm 全部同步。

## [1.1.6] - 2026-08-30

本版本升级更新机制：Windows / Linux 支持应用内直接下载并安装更新（不再只能跳转 GitHub 页面），更新通知提供「直接升级」与「打开发布页」双按钮并展示实时下载进度；镜像模式下资产下载同样走加速镜像。

### ✨ 新功能

- **应用内直接升级（Windows / Linux）** — 更新通知新增「直接升级」按钮：下载完成后自动校验 SHA256（发布版含 Ed25519 签名），通过后启动安装并退出应用；macOS 的 Homebrew 安装仍走 `brew upgrade`。
- **「打开发布页」备选按钮** — 下载失败、校验不通过或想查看发布说明时，一键在浏览器打开对应版本的 GitHub Release 页面。
- **实时下载进度** — 升级过程显示已下载 / 总大小与进度百分比（基于 GitHub API 报告的资产大小，分块传输时同样准确）。
- **镜像模式覆盖资产下载** — 选择 GitHub 加速镜像（mirror）后，资产与校验和文件的下载同样经镜像前缀重写（此前仅 API 检查走镜像，二进制仍直连 GitHub）。

### 🛠 内部

- 下载或安装失败时不再静默：记录日志并回退到打开发布页，保证始终有可用路径。
- 新增下载进度回调、镜像下载重写与 `RunUpdate` 防御分支的单元测试。
- 版本号更新至 **1.1.6**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.5] - 2026-08-30

本版本全面增强日志系统（更新检查、设置审计、分类分级、保留期清理），修复若干设置问题，并重新加入默认关闭的 WireGuard 脚本支持。

### ✨ 新功能

- **更新检查全量日志** — 手动与自动检查均记录实际请求的 endpoint、本地版本、在线版本、`not_modified` 以及错误/重试信息；失败（403、超时等）带 `category=update`，可在 Log 界面查看与筛选。
- **设置变更审计日志** — 每次保存都会记录哪些设置被修改（代理模式、kill switch 等）及关键值；代理凭据会脱敏（`http://***@host`）。
- **日志分类与筛选** — `ipc.LogEntry` 新增 `category` 字段（app / update / settings / tunnel / network / system）；Log 界面新增分类筛选行（All 在最前、默认选中），每条日志显示分类，复制时也携带分类。
- **日志保留期（默认 7 天）** — 按天滚动存储（`wireguideplus-YYYY-MM-DD.log`），超过可配置保留期自动清理。
- **WireGuard 脚本支持（PreUp / PostUp / PreDown / PostDown，默认关闭）** — 与 wg-quick 行为一致（Unix 用 `sh -c`，Windows 用 `cmd.exe /C`），在 helper 内以 30 秒超时执行，输出截断到 1000 字符。默认关闭（设置 → 高级），开启时显示醒目的安全警告，因为命令以完整系统权限运行；PostUp 失败不会中断连接。
- **DNS leak test 增强** — 每台 DNS 服务器显示探测状态（vpn / ok / leak / timeout）与延迟；Windows 收集 DNS 时同时包含 IPv4 与 IPv6。
- **打开文件夹快捷链接** — 设置中新增可点击链接，直接打开隧道配置目录与日志存储目录（跨平台）。

### 🐛 修复

- **通知持续时间设置无法保存** — 离开设置再进入时不再重置。
- **设置中日志分级缺少 All** — 下拉新增 `All`（与 Log 界面默认一致），源头不再过滤任何记录。

### 🛠 内部

- **日志级别 All 全链路生效** — helper 与 GUI 日志处理器均支持 `all`（`slog.Level(-8)`），不会丢弃任何记录。
- 版本号更新至 **1.1.5**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.3] - 2026-08-30

本次版本修复 Windows 客户端自动更新失效的问题：自 v1.1.0 资产改名以来，Windows 发布资产（`wireguideplus-<arch>-installer.exe` / `wireguideplus-<arch>-portable.zip`）命名不含操作系统标识，而更新检查器要求资产名同时携带「OS 标识 + 架构」，导致 Windows 平台永远匹配不到自己的发布资产，已安装用户只会看到「发现新版本但无匹配资产」，无法自动更新。

### 🐛 修复（Bug Fixes）

- **修复 Windows 自动更新资产匹配失效** — `matchAsset`（`internal/update/checker.go`）在 Windows 平台下额外接受「架构锚定 + Windows 专属扩展名」（`.exe` / `.msi` / `.zip`）的资产名，无需 OS 标识；macOS / Linux 资产仍必须携带各自 OS 标识（`darwin` / `linux`），因此不会误匹配 Windows 的无标识资产。新增回归测试覆盖三种架构的正常匹配，以及 Linux / macOS 不得接受无标识 Windows 资产名的反向断言。

### 🛠 内部（Internal）

- 版本号更新至 **1.1.3**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。

## [1.1.2] - 2026-08-30

本次版本修复 Windows 安装包文件版本错位问题：此前发布的 1.1.1 安装包中，运行程序（`wireguideplus-<arch>.exe`）在资源管理器属性页显示的「文件版本」为 **1.1.0.1**（应为 **1.1.1.0**）。

### 🐛 修复（Bug Fixes）

- **修复 Windows 运行程序文件版本错位** — 根因：`goversioninfo v1.7` 将 `FixedFileInfo` 结构体声明为 `Major/Minor/Patch/Build` 顺序（与 Windows 标准布局的 Build/Patch 相反），向 JSON 显式写入数字版本会得到被交换的二进制版本（`1.1.1.0` 变成 `1.1.0.1`）。现在 `build/windows/versioninfo.json` 的 `FixedFileInfo` 数字固定为 0，仅以 `StringFileInfo` 四段版本字符串为唯一输入，由 goversioninfo 推导二进制版本（布局无关、始终匹配）；`tools/genverinfo` 只渲染字符串版本，`tools/bumpversion` 不再触碰数字字段。已验证：传入 `1.1.2.0` 字符串时 goversioninfo 输出 `FixedFileInfo.FileVersion (1.1.2.0)`，安装后属性页与 `FileVersionInfo` 均正确显示。

### 🛠 内部（Internal）

- 版本号更新至 **1.1.2**：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh` + `project.nsi`）、MSIX、Linux nfpm、macOS `Info.plist` 全部同步。
- 修正 NSIS 安装/卸载描述（`project.nsi`），安装包与卸载程序的文件版本信息与运行程序保持一致。

## [1.1.1] - 2026-08-30

本次版本修复 Windows 托盘通知气泡「打开主界面」按钮在系统高负载下偶发导致 GUI 卡死的问题。

### 🐛 修复（Bug Fixes）

- **修复通知气泡「打开主界面」偶发卡死** — 当系统 CPU 争用激烈（例如 Windows 维护进程占满核心）或 WebView2 响应延迟时，点击托盘通知气泡的「Open Window」按钮会同步阻塞等待 UI 线程，整个 GUI 看似冻结（VPN 隧道不受影响）。`showDock`（`internal/gui/dock_other.go`）改为经 `application.InvokeAsync` 在 Wails UI 线程异步执行：调用方立即返回，窗口显示/聚焦均在 UI 线程内联完成，不再跨线程等待；同时加 recover 防护，意外 panic 不会打断主线程回调链。

### 🛠 内部（Internal）

- 版本号更新至 **1.1.1**：`internal/update/checker.go` 主版本、`build/config.yml`、`windows/info.json`（`1.1.1.0`）、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh`）、MSIX、Linux nfpm、`tools/genverinfo` 全部同步。

## [1.1.0] - 2026-08-28

本次版本聚焦可辨识性、代理健壮性与启动自动化规则：托盘状态改用高辨识图标、代理三模式语义明确并新增连通性测试、无效代理 URL 不再破坏更新检查、启动时先按自动化规则判断再连接。

### ✨ 新功能（Features）

- **托盘状态图标可辨识化（Tray state glyphs）** — Windows 托盘菜单中的连接状态改用纯文本字形区分：`●` 实心=已连接、`○` 空心=未连接（Windows 托盘弹窗由 GDI 绘制，无法渲染彩色 emoji，`🟢` 会退化成一圈灰色轮廓，新旧状态难分辨）；macOS 菜单栏（AppKit 原生渲染）继续使用彩色 emoji。启动中/过渡态另有专属标记。
- **代理三模式语义明确 + 连通性测试（Proxy modes & test）** — 设置 → 代理 的选项统一为三种且语义不再混淆：**直连**（完全忽略系统/环境代理）、**GitHub 镜像**（`mirror`，如 `https://ghfast.top` 加速前缀）、**手动代理**（`manual`，http/https/socks5 完整 URL）。新增 **"测试连接"** 按钮：保存前先向 GitHub Releases API 发起往返请求，报告成功与延迟。
- **代理设置即时生效（Proxy applies immediately）** — 保存代理配置后，下一次计划更新检查（及手动"立即检查"）无需重启即生效；GUI 启动时也直接套用已保存的代理，避免"启动即触发一次错误配置的检查"。

### 🐛 修复（Bug Fixes）

- **修复无效代理 URL 拖垮更新检查** — `config.json` 中残缺的手动代理（如 `proxy_url = "https://"`）此前会被 `http.ProxyURL` 直接采用，导致每次更新检查报 `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify must be specified in the tls.Config`。现在启动时与每次使用时均校验 URL（`internal/update/proxy.go`），无效值记录 `WARN update: ignoring invalid manual proxy URL` 并回退直连，检查不再失败。
- **修复"先连接、后按规则断开"的启动观感** — 启动规则评估提前到 helper 启动后立即执行（日志 `startup rule re-evaluation`），确保每个隧道的目标状态由规则先行决定；同时新增 `scheduleRuleCheck` 兜底：启动 60 秒窗口内任何 RPC 手动连接（如恢复上次会话）都会在 3 秒后按规则重新评估并纠正，不等 30 秒轮询，日志记录触发来源便于排查。
- **无效镜像前缀不再静默破坏检查** — `mirror` 模式下的加速前缀同样做 scheme/host 校验，非法值回退官方 API 端点。

### 🛠 内部（Internal）

- 版本号更新至 **1.1.0**：`internal/update/checker.go` 主版本、`build/config.yml`、`windows/info.json`（`1.1.0.0`）、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm 全部同步。
- **Windows 版本资源标准化** — `wails3 generate syso` 生成的版本资源语言为 `0x0000` 且 `VS_FIXEDFILEINFO.ProductVersion` 为零，Windows 资源管理器 / `FileVersionInfo` 无法读出（属性页版本字段空白）。改用 `goversioninfo`（配置：`build/windows/versioninfo.json`）生成标准 `0409/04B0` 资源，`generate:syso` 任务同步更新；exe 与安装包属性页现正确显示 `1.1.0`。
- **新增 Windows x86（32 位）构建** — `task windows:build ARCH=386` 产出 32 位运行程序与 `wireguide-x86-installer.exe` 安装包（NSIS 脚本支持 x86 架构、安装到 `Program Files`、打包 x86 版 `wintun.dll`）。
- **明确平台边界** — 移除 iOS 构建任务与配置注释；本项目不支持 Android / iOS（无法多通道并发、无法按 SSID 自动连接），README 已同步说明，macOS / Linux 增强版待开发。
- **系统集成增强** — 新增「最小化启动」设置（启动时直接最小化到系统托盘，不显示主窗口，设置 → 启动）；新增「连接状况托盘通知」：启动后延迟 10 秒显示当前连接状况，网络变动（Wi-Fi 切换、网线插拔、网络断开等）导致隧道连接状态变化时也延迟 10 秒显示稳定后的最新状况；通知气泡带操作菜单（打开主界面 / 断开连接），可手动关闭或按设置自动关闭（默认停留 10 秒，可在 设置 → 启动 → 通知停留时长 调整，`internal/gui/notify_windows.go`）。
- **双架构发布** — 每次构建同时产出 32 位（x86）与 64 位（amd64）程序及对应安装包（`task windows:build:all`，含 wintun.dll 架构自动刷新）；软件/安装包描述统一为「多隧道 + 自动化」重点，移除跨平台（cross-platform）表述。
- **安装体验** — 安装包默认安装到 Program Files（32 位安装包自动选择 Program Files (x86)），安装过程中可自定义目录；开始菜单快捷方式（含「卸载 WireGuide Plus」入口，卸载入口图标与运行程序一致）默认创建，可在「快捷方式选项」页取消勾选；桌面快捷方式始终创建（`build/windows/nsis/project.nsi`）。
- **开发与发布文档** — 构建/打包说明从 README 移至独立开发文档 `docs/DEVELOPMENT.md`；GitHub Release 工作流补齐 32 位 Windows 产物与 CI 工具链（goversioninfo），本地推送 `v*` 标签即可自动构建（Windows x86+amd64、macOS arm64、Linux amd64+arm64）、签名并发布（`docs/release.md`）。
- Windows 网卡适配器名匹配逻辑调整（`internal/wifi/known_windows.go`、`detect_windows.go`），物理网卡识别更准确。
- 窗口标题统一为 **WireGuide Plus**。
- 更新检查在调度器内去重，避免同一轮多次触发（仅记录一次失败并给出重试间隔）。

## [1.0.0] - 2026-08-28

里程碑版本：A11y 无障碍语义重构、Windows 网络出口选路逻辑调整、Wails3 构建/图标/权限梳理，并新增简体中文界面与托盘开关。

### ✨ 新功能（Features）

- **简体中文界面（Chinese UI）** — 全界面新增简体中文翻译，覆盖隧道列表、历史、工具（DNS 泄漏测试/路由表）、日志、设置、更新、自动化编辑器等全部 199 条文案。首次启动自动跟随系统语言（`zh-*` 区域自动识别），也可在 设置 → 常规 → 语言 中手动切换并持久化。
- **托盘菜单开关（Tray toggles）** — 系统托盘内每条隧道变为独立可点击的开关：勾选连接、取消勾选断开；连接状态 emoji（🟢 已连接/🟡 连接中/○ 断开）保留在标签旁。手动关闭的隧道保持豁免自动规则（manual-off），直到重新连接或重启 WireGuide。

#### 前端 A11y 无障碍重构

> 影响：全平台（Windows/macOS/Linux）Svelte 前端，不限于 Windows。

- 全部模态弹窗移除蒙层 `role="button"` 与 `tabindex="0"`，蒙层回归纯粹遮罩语义，避免读屏器将全屏背景识别为可交互按钮。
- 所有 dialog 统一 `tabindex="-1"` 并保留标准 `role="dialog" aria-modal="true"`，遵循 WCAG 弹窗语义规范。
- ESC 关闭统一处理：缺失的弹窗（导入结果、历史、更新提示、自动化编辑器）在**组件顶层**挂 `<svelte:window on:keydown>`（handler 内以条件判断弹窗状态；Svelte 不允许在 `{#if}` 内挂载），其余弹窗复用 App.svelte 全局 capture 处理器——规避多弹窗 ESC 冲突，同时不破坏 CodeMirror 的按键捕获。
- `Settings.svelte`：`<nav role="tablist">` 改为普通 `<div>`，消除标签语义不匹配警告；分割条 `pane-resizer` 保留 `role="separator"`，补 `tabindex="0"` 与真实键盘操作（方向键调整宽度、Enter/Space 复位）。
- `frontend/vite.config.js` 的 svelte 插件 `onwarn` 过滤静态误报（`a11y_click_events_have_key_events`、`a11y_no_static_element_interactions`、`a11y_no_noninteractive_tabindex`、`a11y_no_noninteractive_element_interactions`），生产构建警告归零，业务逻辑无改动。
- 涉及文件：`src/App.svelte`、`src/lib/History.svelte`、`src/lib/ConflictWarning.svelte`、`src/lib/TunnelDetail.svelte`、`src/lib/UpdateNotice.svelte`、`src/lib/Settings.svelte`、`src/lib/AutomationEditor.svelte`

#### Windows 后台 helper：网络出口选路逻辑调整

> 影响：仅 Windows 平台 Go helper 代码，其他平台不受改动。

- helper 启动阶段采集主上游物理网卡 LUID，用于记录系统初始默认出站物理接口；该 LUID 为启动时刻快照，运行时网络切换不会自动刷新缓存。
- 修正网络接口筛选逻辑：过滤 TUN/隧道/回环虚拟网卡，仅选取物理网卡作为上游候选；TUN 虚拟网卡本身不做物理网卡绑定锁定。
- WireGuard UDP 报文出站完全交由 Windows 路由表 + 网卡 InterfaceMetric 跃点数完成选路；软件不再强制绑定固定物理网卡。
- 分流模式（`full_tunnel=false`）逻辑约束补充：Peer Endpoint IP 需要显式加入 `AllowedIPs`，防止握手 UDP 报文路由丢弃导致 `no-handshake`。
- 日志增强：`network primary upstream interface initial luid` 输出主物理网卡 LUID 用于问题排查；明确日志中 `tunnel connected` 仅代表 TUN 适配器就绪，不等同于远端 Peer 握手成功。
- 排查工具提示：Windows 下优先使用 `Find-NetRoute -RemoteIPAddress <peer-ip>` 判断目标 IP 实际出站网卡；PowerShell `Get-NetAdapter.Luid` 为结构体，不可直接与 Go 输出 uint64 数值做等值比对。

### 🛠 构建与工程（Build & Project）

主要为 Windows 构建行为，跨平台部分已标注。

1. **Wails3 Windows 图标构建行为**（仅 Windows）——`task build` 完整构建会自动执行 `wails3 generate icons`，读取 `build/appicon.png` 并覆盖输出 `windows/icon.ico`；手动修改的 `windows/icon.ico` 会被完整构建覆盖。`windows/icon.ico` 是最终嵌入 exe 的图标，`build/appicon.png` 仅作源素材；`task windows:build` 调试构建跳过图标生成，保留现有 `windows/icon.ico`。exe / 窗口标题栏 / 任务栏图标复用 exe 内 ico 资源；系统托盘图标需要 Go `embed` 独立资源。
2. **Windows 版本信息管理**（仅 Windows）——exe 文件详细信息由 `windows/info.json` 控制，`FileVersion` 必须 4 段数字格式 `major.minor.patch.build`。UI 展示版本由 Go 常量维护（`internal/update/checker.go`），需与 `info.json` 手动保持同步；后续可通过 ldflags 编译注入实现单处版本源。
3. **Windows UAC / 管理员权限梳理**（仅 Windows）——当前架构为 GUI 进程拉起 helper 子进程；helper 操作 TUN 网卡、修改路由需要管理员权限，子进程提权会触发 UAC 弹窗，Windows 安全机制无法完全静默绕过。短期方案：`windows/wails.exe.manifest` 添加 `requireAdministrator`，将 UAC 弹窗转移到 exe 双击启动（仍需用户确认）；长期建议：helper 重构为 Windows System Service（LocalSystem 权限后台运行），GUI 以普通用户权限通过 IPC 通信，彻底消除 UAC 弹窗。

### 🐛 问题排查记录（Investigation）

排查记录，无代码变更，供开发参考。

- 现象：helper 日志输出 `tunnel connected`，但 GUI 显示 `no handshake`。
  - 根因区分：TUN 设备创建完成 ≠ WireGuard 与远端 Peer 完成加密握手；需读取 wg 内核 `latest handshake` 状态判断真实连通性。
  - 分流模式高频踩坑：Peer IP 不在 `AllowedIPs`，握手 UDP 报文路由丢弃。
  - 其他可能：Windows 出站防火墙拦截 WireGuard UDP、endpoint 域名 DNS 解析异常。
- 本地代理监听 `0.0.0.0`：代理进程流量独立，不会自动流入 WireGuard 隧道；流量走向由 Windows 路由表与隧道 `AllowedIPs` 共同决定。

### 📝 说明（Notes）

1. **改动影响范围区分**
   - Svelte 前端 A11y 代码：**全平台生效（Windows / Linux / macOS）**；弹窗 ESC、无障碍语义变更所有桌面平台都会生效。
   - helper 网络出口选路逻辑：**仅 Windows 平台 Go 代码修改**，其他 OS 不受影响。
   - 构建、manifest、ico、info.json、UAC 相关：**仅 Windows 平台**。
2. 前端 A11y 修改与 helper 后台网络逻辑完全解耦，不影响隧道创建、路由、自动化 Wi-Fi 规则运行。
3. helper 记录的上游 LUID 仅为启动瞬间快照；Wi-Fi/有线网络切换时不会自动更新该值。

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
