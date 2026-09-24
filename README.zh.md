# WireGuide Plus

**支持多隧道与网络感知自动化的 WireGuard 客户端**

WireGuide Plus 是对开源项目 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)
进行**深度修复与增强**的版本。两大核心能力：

- **多隧道并发** — 多条 WireGuard 隧道可同时建立、互不干扰地独立运行；
- **条件自动连接** — 按 Wi-Fi SSID、时间段、系统启动等条件自动连接对应隧道
  （例如办公 Wi-Fi 下连隧道 A，家里连隧道 B）。

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [한국어](README.ko.md) | [日本語](README.ja.md)

> **Windows 10 / 11（x64、x86 32 位与 ARM64）以及 macOS（Apple Silicon / arm64）完全支持** —
> macOS 已在 Apple Silicon 真机充分验证。Linux（x64、arm64）提供**实验性预览版** —
> 经 CI 构建、尚未在真机测试（见 [平台支持](#平台支持)）。**不支持 Android / iOS。**

## 主要功能

- **多隧道并发** — 与上游「同一时刻只能连一条隧道」不同，本版支持多条隧道并行
  运行，可同时访问内网与外部网络。
- **条件自动连接** — 按 Wi-Fi SSID / 时间段 / 系统启动等条件自动连接或断开隧道，
  规则支持优先级与互斥。
- **自动重连** — 隧道意外断开后自动恢复，连接状态实时可见。
- **熄屏、锁屏或睡眠时保持连接** — 一项设置（默认开启，可按隧道覆盖），让隧道在熄屏、屏保和锁屏下保持存活，并在机器从睡眠唤醒后自动重连。
- **开机自启** — 登录后自动启动 WireGuide Plus 并按规则连接（配合「启动最小化」
  窗口启动即收拢）。
- **启动最小化** — Windows 上启动后最小化到任务栏（任务栏图标保留，主界面随时可
  重新打开）；macOS/Linux 上最小化到系统托盘。
- **托盘连接状态通知** — 启动后 15 秒（等提权弹窗落定后）弹出全部隧道总览气泡：逐条列出实时状态（绿色对钩 / 红色叉 / 连接中黄点）与自动/手动模式；网络
  变化（切换 Wi-Fi、拔网线、断网……）导致隧道状态改变时，延时 10 秒弹出稳定后的
  最新状态气泡。气泡带操作菜单（打开主窗口 / 断开），可手动关闭，也可按可调时长
  自动关闭（默认 10 秒，可在设置中调整）。macOS 与 Linux 现已弹出同款气泡（此前仅 Windows）。
- **隧道管理** — 导入 / 导出 `.conf`、连接历史、快速开关。
- **AmneziaWG（AWG）隧道** — 支持导入并连接 AmneziaWG（混淆版 WireGuard）配置。AWG 由配置中的 Jc/Jmin/Jmax/S1-S4/H1-H4 混淆参数自动识别，对应隧道会显示「AmneziaWG」徽标；可在「设置 → 高级」中关闭支持。
- **隧道编辑器：字段视图与脚本钩子** — 除 conf 文本外，提供逐字段表单（interface / peer 分组）编辑配置，并支持管理 PreUp / PostUp / PreDown / PostDown 脚本钩子（选择文件、新建空白、编辑代码、清除）。隧道连接中也可编辑：保存时会断开、应用改动并自动重连。
- **每隧道物理出口绑定** — 开启「固定接口」后，可将单个隧道的加密流量固定到指定物理网卡（Windows / Linux / macOS）；绑定网卡失效时弹窗提供等待、自动切换或手动指定。
- **设置导出与导入** — 将 tunnels、scripts 与 `config.json`（不含日志）打包为单个压缩包，便于迁移到其他机器。
- **工具箱标签页** — 内置 **DNS 泄漏测试** 检查流量是否真的从配置的 DNS 服务器出去，以及 **路由可视化** 展示当前路由表，并为每条路由标注 VPN / Direct 徽标，支持局域网直连过滤与蜂窝接口标记。
- **隧道策略** — 每隧道的 DNS 解析路径、强制指定域名走隧道、流量保护、默认 DNS，以及确定性的冲突裁决。
- **延迟探测** — 每隧道延迟探测支持多目标并行；每个候选（公共解析器 8.8.8.8 / 223.5.5.5、隧道端点，以及任意手填地址或域名）同时探测，并展示其解析 IP、类型与往返时延。分流隧道下手填目标会在保存前按隧道 AllowedIPs 覆盖校验。目标保存后会立即重新探测；域名端点会在详情页顶部显示当前解析到的地址。

## 针对原 wireguide 的修复与增强

### 修复

1. **Wi-Fi 作为网络出口（最关键的修复）** — 上游在 Windows 上只走**有线**网卡出口，
  导致 Wi-Fi 下网络出口不可用。本版修复默认出口网卡选择：Wi-Fi 环境下流量正确从
  无线网卡发出。
2. **GUI 主题切换显示异常** — 修复深色 / 浅色主题切换时渲染错乱的问题。
3. **Windows 版本资源规范化** — 修复 exe 属性「详细信息」页版本信息空白的问题
  （改用 `goversioninfo` 生成）。
4. **稳定性修复** — 去重更新检查调度、更准确的物理网卡识别等
  （详见 [更新日志](CHANGELOG.md)）。

### 增强

1. **SSID 下拉选择** — 自动连接规则可从系统**已保存的所有 Wi-Fi SSID** 中下拉选择，
   无需手输，避免打错。
2. **代理更新检查** — 检查更新前可配置 HTTP(S) 代理，解决 GitHub 连不上 / 限流
   导致的更新失败。
3. **多语言界面** — 简体中文 / English / 日本語 / 한국어 / 繁體中文。
4. **系统集成** — 开机自启、启动最小化（Windows 最小化到任务栏 / macOS 与 Linux
   最小化到托盘）、托盘连接状态通知（启动后 15 秒全部隧道总览、网络变化后延时 10 秒提示，
   默认停留 10 秒，可调）。
5. **窗口标题与交互细节** 等更多改进。
6. **AmneziaWG（AWG）协议支持** — 新增基于 amneziawg-go 的 AmneziaWG 协议后端，支持 DPI 抗识别混淆隧道。配置自动识别、界面显示徽标，并可在「设置 → 高级」中一键关闭。

## 自动化规则

自动化以**隧道为单位**独立配置（任意隧道的 `…` 菜单 →「自动化…」）。每个隧道拥有完全独立的规则集合，因此「公司 Wi-Fi 连接办公 VPN、公司 Wi-Fi 断开家庭 VPN、回家自动连接家庭 VPN」这样的组合可以共存互不干扰。

编辑器顶部的实时网络看板只描述**当前隧道**的网络环境，每类信息一行：所有检测到的真实硬件接口（标注「使用中」或「未使用」）、Wi-Fi SSID、网关 MAC、网关 IP 和子网，并显示当前隧道中哪些条件命中了这些值。虚拟网卡会被排除；未连接 Wi-Fi 时 SSID 显示「Wi-Fi 未连接」。看板和规则说明都可以折叠，整个编辑器支持滚动，规则较多时仍便于操作。

「固定接口」现已支持全部桌面平台：Windows 通过 `IP_UNICAST_IF` 将隧道的 UDP 套接字绑定到指定网卡，Linux 使用 `ip route … dev <网卡>` 旁路路由，macOS 以 `-ifscope` 限定旁路路由。指定物理出口属于操作系统路由策略（而非 WireGuard 协议设置）；绑定网卡失效时会弹窗让你选择保持绑定等待、自动切换或手动指定。

### 规则逻辑

- 单条隧道的规则是**一个有序列表**，自上而下依次评估——**规则位置即优先级**，连接与断开规则可以在整个列表中自由混排和拖拽调整。
- **首条命中生效（规则内 AND）**：一条规则下的所有条件必须全部满足，规则才算命中；但整个列表中**只有第一条命中**的规则会执行动作。排在已命中规则之后的规则会被标记为「命中但被降权」而不会执行，因此不会出现同一 SSID 下既 disconnect 又 connect 的自相矛盾。
- **默认状态兜底**：当**所有规则都不命中**当前网络时，隧道收敛到其**默认状态**（连接或断开，在自动化编辑器顶部选择）。既没有规则也没有默认状态的隧道，自动化不会干预；只设置默认状态（零规则）时，隧道会始终收敛到该默认状态。
- **编辑时实时匹配指示器**：打开自动化编辑器后，每个条件会即时显示是否命中当前网络，实际生效的首条命中规则会被高亮为「in use（正在使用）」，顶部还有一条裁决条显示该隧道的最终动作。指示器会在每次编辑后立即刷新（≈ 250 ms 防抖，通过 IPC 调用与后台 helper 执行控制**相同**的评估引擎，因此界面显示与真实行为永远一致）。同引擎也可通过命令行调用：`wireguideplus automation`，适合无界面环境排查。

### 手动覆盖

隧道详情页里的「连接 / 断开」按钮属于手动控制。**手动连接会强制连接、忽略所有自动化规则**；**手动断开会强制断开、忽略所有自动化规则**——直到下一次自动化决策重新评估网络。界面上有一个闩锁指示器，标明当前隧道动作来自手动覆盖还是自动化，让你始终清楚到底是什么在驱动连接。

### 将隧道排除出自动化

手动覆盖只持续到应用重启为止。如果某条隧道**永远**不允许自动化驱动——典型场景是它同时被另一个 WireGuard 客户端控制——请在隧道的自动化编辑器里把「自动化」开关关掉。该隧道保留自己的规则与默认状态，但自动化引擎不会连接或断开它。

同一个开关也是隧道详情页顶部的一键式「停止自动化」开关，让你在查看隧道时就能直接压制自动化。全局「禁止系统休眠」设置（默认关闭）会在有隧道连接时让计算机保持唤醒（Windows、macOS、Linux 三平台均生效），最后一条隧道断开后立即释放，使空闲或休眠的电源策略不会悄悄杀掉隧道。当另一个客户端已占用某条隧道的地址时，WireGuide Plus 会弹出一个常驻对话框，点名冲突的软件、显示该隧道的自动化状态，并给出仅有的两个选择——停止本隧道自动化，或继续尝试——在你决定之前暂停该隧道的自动连接；未处理的冲突会在 GUI 启动及 helper 重启后再次弹出——由你决定，它绝不强退另一个客户端。

### 一条隧道，一个客户端

两个 WireGuard 客户端不能同时运行同一条隧道：隧道的 `Address` 会先到先得地被某个适配器占住，后连接的客户端会在分配地址一步失败（Windows 报 `The object already exists.` / 对象已存在）。如果你同时使用官方 WireGuard 客户端，请先停掉它的同名隧道服务（例如 `Stop-Service 'WireGuardTunnel$<隧道名>'`），或将该隧道排除出自动化（见上）。WireGuide Plus 会检测这种冲突，在常驻冲突对话框与 helper 日志里都点名冲突适配器，并对失败的自动化连接按退避节奏重试（30 秒 → 1 分 → 2 分 → 5 分），而不是每个轮询周期都硬撞适配器。

### 条件类型

| 条件 | 匹配逻辑 | 典型场景 |
| --- | --- | --- |
| **SSID** | 与当前 Wi-Fi 的 SSID 做**字节级全名精确比较**（区分大小写，中间空格与特殊字符全部参与匹配，符合 802.11 定义）。 | 「连接到 `公司 5GHz` 时自动连办公 VPN」。 |
| **子网（Subnet）** | 当前本机物理网口 IP 是否落在指定 CIDR 段（如 `192.168.178.0/24`）。 | 家用/办公路由器 LAN 段可预测、但 SSID 不稳定时。 |
| **网关 MAC** | 当前默认网关（路由器）的 MAC 地址 —— 即使 SSID 或网段很通用，也能锁定某个具体网络。 | 「咖啡店那个路由器**永远不要**自动连接。」 |
| **网关 IP** | 当前物理网络默认网关的 IP 地址。 | SSID 太泛化（全公司叫相同 SSID）、但每个楼层网关 IP 不同时。 |
| **网卡接口（Interface）** | 当前系统作为上行路由的网卡名称。下拉列表列出本机**全部**网络接口——有线和无线网卡都在内，也包括**尚未连接**的设备，方便提前为扩展坞、USB 网卡、雷电网卡、Wi-Fi 网卡等未激活的设备写好规则。 | 「只有插在公司扩展坞上的有线网卡时才连接办公 VPN。」 |
| **时间段** | 一组星期几 + 起止时间（本地时间）。 | 「周一至周五 09:00–18:00，办公 VPN 必须保持在线。」 |

一条规则可以自由组合以上条件：例如「SSID = 公司 AND 时间段 = 周一至周五 09–18」就是一条带两个 AND 条件的规则。每个隧道的 disconnect / connect 两组都支持任意数量的 AND 规则。

## 平台支持

| 平台 | 状态 |
| --- | --- |
| Windows 10 / 11（x64、x86 32 位、ARM64） | ✅ 完全支持（多隧道并发 + SSID 自动连接，含 AmneziaWG） |
| macOS（Apple Silicon / arm64） | ✅ 完全支持 — 已在 Apple Silicon 真机充分验证 |
| Linux（x64、arm64） | 🚧 实验性 — 经 CI 构建，尚未在真机测试 |
| Android / iOS | ❌ **不支持**（无法并发运行隧道，也无法按 Wi-Fi SSID 自动切换隧道） |

### 为什么没有移动版？

本项目的核心能力是**多隧道并发**与**按条件自动连接（如按 Wi-Fi SSID）**。在
Android / iOS 上，系统内核与权限机制使 WireGuard 实现**无法同时运行多条隧道**，
也无法**按 Wi-Fi SSID 自动切换隧道**——两大核心目标在移动端都不可实现。因此本项目
**明确不做移动版**；移动端单隧道需求请使用官方 WireGuard App 的 On-Demand 能力。

## 下载与安装

每个 Release 都会为每个受支持的平台发布**安装包（推荐）**与**免安装版**：下方按操作系统说明。macOS 提供 `.dmg`/`.zip`，Linux 提供 `.deb`/`.tar.gz`，Windows 即下文的安装包/绿色版。所有 Release 还会附带一份 Ed25519 签名的 `SHA256SUMS`（及 `SHA256SUMS.sig`），应用内更新器在应用任何更新前都会校验它。

### Windows

**安装包（推荐）**

- Windows x64 安装包：`wireguideplus-<version>-amd64-installer.exe`
- Windows x86（32 位）安装包：`wireguideplus-<version>-x86-installer.exe`
- Windows ARM64 安装包：`wireguideplus-<version>-arm64-installer.exe`

安装包文件名内嵌版本与架构信息（`wireguideplus-<version>-<arch>-installer.exe`，arch 为
`x86` / `amd64` / `arm64`），安装后的程序文件名同样带架构
（`wireguideplus-<arch>.exe`，文件属性→详细信息中同样可见）。64 位安装包默认安装到
`C:\Program Files\WireGuide Plus`；32 位安装包默认安装到
`C:\Program Files (x86)\WireGuide Plus`（32 位系统为 `C:\Program Files\WireGuide Plus`）。
安装过程中可更改安装目录。会创建开始菜单快捷方式（含「卸载 WireGuide Plus」入口，
默认创建、可选择不创建）与桌面快捷方式（始终创建）。安装包已内置全部所需文件，
无需额外下载。

**绿色版（免安装）**

- `wireguideplus-amd64.exe` **+ `wintun-amd64.dll`**（32 位 exe 配 **`wintun-x86.dll`**，
  ARM64 exe 配 **`wintun-arm64.dll`**）— 需同时下载**同一架构**的两个文件放在同一
  文件夹，再运行 exe。

绿色版**并非独立程序**：运行需要同目录下放置与程序架构匹配的驱动 DLL（用于创建
WireGuard 隧道）。程序按架构自动加载对应文件（`wintun-amd64.dll` / `wintun-x86.dll` /
`wintun-arm64.dll`），**无需改名**，按下表选择即可：

| exe | 匹配的驱动 DLL |
| --- | --- |
| `wireguideplus-amd64.exe`（64 位） | `wintun-amd64.dll` |
| `wireguideplus-x86.exe`（32 位） | `wintun-x86.dll` |
| `wireguideplus-arm64.exe`（ARM64） | `wintun-arm64.dll` |

驱动 DLL 来自 `wintun-0.14.1.zip`（见
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintun-driver-dll)）。Release 提供打包好的便携
zip（`wireguideplus-<version>-amd64-portable.zip` / `wireguideplus-<version>-x86-portable.zip` /
`wireguideplus-<version>-arm64-portable.zip`），内含 exe **和**对应架构的驱动 DLL——下载后解压
即可运行。Release 不再单独附驱动 DLL（请使用便携 zip 或安装包）。缺少匹配的驱动 DLL
时无法创建隧道。



### macOS（Apple Silicon）

每个 Release 提供两个产物：

- `WireGuidePlus-<version>-darwin-arm64.dmg` — 拖拽到「应用程序」的安装器。
- `WireGuidePlus-<version>-darwin-arm64.zip` — 免安装 `.app` 包。

打开 `.dmg`，将 **WireGuide Plus** 拖入「应用程序」，再从聚焦或启动台打开。便携版 `.zip` 解压后即为 `wireguideplus.app`，可直接运行。

注意：

- **仅支持 Apple Silicon。** CI 只构建 `arm64` 单一架构；Intel Mac 需自行[从源码构建](docs/DEVELOPMENT.md)。
- **仅本地临时签名（ad-hoc），未经过 Apple 公证。** 首次打开时 macOS Gatekeeper 会拦截。可右键 →「打开」，或执行一次以下命令清除隔离属性：
  ```sh
  xattr -dr com.apple.quarantine /Applications/wireguideplus.app
  ```
- 同时提供 Homebrew cask（arm64 版）：`brew install --cask wireguideplus` 拉取的也是同一个 `WireGuidePlus-<version>-darwin-arm64.zip`。
- **位置服务授权（用于按 SSID 自动化）**。按 SSID 自动连接会读取当前 Wi-Fi SSID，而在 macOS 上这需要 **位置服务** 权限。请在 **系统设置 → 隐私与安全性 → 位置服务** 中开启（先打开总开关，再允许 WireGuide Plus）；否则 SSID 条件无法评估，基于 SSID 的规则不会触发。

### Linux（实验性）

每个架构（`amd64`、`arm64`）提供两个产物：

- `WireGuidePlus-<version>-linux-<arch>.deb` — Debian / Ubuntu 安装包。
- `WireGuidePlus-<version>-linux-<arch>-portable.tar.gz` — 免安装二进制。

用包管理器安装 `.deb`，例如 `sudo apt install ./WireGuidePlus-<version>-linux-amd64.deb`；或解压便携包后运行 `./wireguideplus`。便携版需要 GTK3 / WebKitGTK 运行库，`.deb` 会自动安装；裸系统请先安装：

```sh
sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0 libayatana-appindicator3-1
```

> Linux 构建为**实验性**——仅经 CI 构建、尚未在真机验证，且需要桌面会话（不支持无头服务器）。

## 代码签名

代码签名因平台而异。在 **Windows** 上，每个发布的**安装包**都经过 Authenticode 签名（启用 SignPath 签名时），可验证**完整性**——二进制自签名后未被修改，且首次运行触发的 Windows SmartScreen 警告更少。在 **macOS** 上，`.app` 为**本地临时签名（ad-hoc）**（无 Apple 开发者签名），故首次启动会被 Gatekeeper 拦截（见上方 macOS 安装说明）。在 **Linux** 上，`.deb` 与免安装版均为**未签名**。

签名本身证明的是**由谁签署**，并不能单独证明**安装包是如何构建出来的**。构建来源、审批
流程、账户安全与可复现性等信息，以及随每个 Release 提供的 SHA-256 校验和，均记录在
[SIGNING-POLICY.md](SIGNING-POLICY.md)。

注意：所有平台中，只有 Windows 安装包带有代码签名；macOS 的 `.app` 为本地临时签名、Linux 产物均未签名，因此每个 Release 附带的 Ed25519 签名 `SHA256SUMS` 才是通用的完整性校验方式。

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 构建与开发

构建环境依赖、开发 / 发布构建命令（含 x86 + amd64 + arm64 多架构构建）、NSIS 安装包说明、
版本资源与发布流程见独立开发文档 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。发布只需
本地推送版本标签，GitHub Actions 流水线会自动构建、签名并发布 Release
（见 [docs/release.md](docs/release.md)）。

## 数据与日志

| 平台 | 项目 | 位置 |
| --- | --- | --- |
| Windows | 设置 / 历史 | `%APPDATA%\wireguideplus\`（`config.json`、`history.json`） |
| Windows | 隧道配置 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| Windows | 隧道脚本 | `%APPDATA%\wireguideplus\scripts\` |
| Windows | 日志 | `%APPDATA%\wireguideplus\logs\` |
| macOS | 设置 / 历史 | `~/Library/Application Support/wireguideplus/` |
| macOS | 隧道配置 | `~/Library/Application Support/wireguideplus/tunnels/*.conf` |
| macOS | 隧道脚本 | `~/Library/Application Support/wireguideplus/scripts/` |
| macOS | 日志 | `~/Library/Logs/wireguideplus/` |
| Linux | 设置 / 历史 | `~/.config/wireguideplus/`（`$XDG_CONFIG_HOME/wireguideplus/`） |
| Linux | 隧道配置 | `~/.config/wireguideplus/tunnels/*.conf` |
| Linux | 隧道脚本 | `~/.config/wireguideplus/scripts/` |
| Linux | 日志 | `~/.local/share/wireguideplus/`（`$XDG_DATA_HOME/wireguideplus/`） |

## 卸载

- **Windows** — 通过 **控制面板 → 程序和功能 → WireGuide Plus** 卸载，或运行安装目录下的卸载程序。
- **macOS** — 将 **WireGuide Plus** 从「应用程序」拖入废纸篓；如需可一并删除 `~/Library/Application Support/wireguideplus` 与 `~/Library/Preferences/com.imonior.wireguide-plus.plist`。
- **Linux** — `sudo apt remove wireguideplus`（`.deb`），或删除免安装二进制与 `~/.config/wireguideplus`。

## 致谢

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 上游开源项目
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
