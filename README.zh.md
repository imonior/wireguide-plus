# WireGuide Plus

**一款 Windows 上的多隧道、自动化优先的 WireGuard 客户端**

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
- **开机自启** — 登录后自动启动 WireGuide Plus 并按规则连接（配合「启动最小化」
  窗口启动即收拢）。
- **启动最小化** — Windows 上启动后最小化到任务栏（任务栏图标保留，主界面随时可
  重新打开）；macOS/Linux 上最小化到系统托盘。
- **托盘连接状态通知** — 启动后 10 秒（等提权弹窗落定后）提示当前连接状态；网络
  变化（切换 Wi-Fi、拔网线、断网……）导致隧道状态改变时，延时 10 秒弹出稳定后的
  最新状态气泡。气泡带操作菜单（打开主窗口 / 断开），可手动关闭，也可按可调时长
  自动关闭（默认 10 秒，可在设置中调整）。
- **隧道管理** — 导入 / 导出 `.conf`、连接历史、快速开关。
- **AmneziaWG（AWG）隧道** — 支持导入并连接 AmneziaWG（混淆版 WireGuard）配置。AWG 由配置中的 Jc/Jmin/Jmax/S1-S4/H1-H4 混淆参数自动识别，对应隧道会显示「AmneziaWG」徽标；可在「设置 → 高级」中关闭支持。

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
   最小化到托盘）、托盘连接状态通知（启动后 10 秒、网络变化后延时 10 秒提示，
   默认停留 10 秒，可调）。
5. **窗口标题与交互细节** 等更多改进。
6. **AmneziaWG（AWG）协议支持** — 新增基于 amneziawg-go 的 AmneziaWG 协议后端，支持 DPI 抗识别混淆隧道。配置自动识别、界面显示徽标，并可在「设置 → 高级」中一键关闭。

## 自动化规则

自动化以**隧道为单位**独立配置（任意隧道的 `…` 菜单 →「自动化…」）。每个隧道拥有完全独立的规则集合，因此「公司 Wi-Fi 连接办公 VPN、公司 Wi-Fi 断开家庭 VPN、回家自动连接家庭 VPN」这样的组合可以共存互不干扰。

编辑器顶部的实时网络看板只描述**当前隧道**的网络环境，每类信息一行：所有检测到的真实硬件接口（标注「使用中」或「未使用」）、Wi-Fi SSID、网关 MAC、网关 IP 和子网，并显示当前隧道中哪些条件命中了这些值。虚拟网卡会被排除；未连接 Wi-Fi 时 SSID 显示「Wi-Fi 未连接」。看板和规则说明都可以折叠，整个编辑器支持滚动，规则较多时仍便于操作。

「固定接口」目前仅在 macOS 上通过 `-ifscope` 为 VPN 旁路路由绑定接口。Windows 和 Linux 暂未将此设置标记为支持；指定 WireGuard 的物理出口属于操作系统路由策略，不是 WireGuard 协议本身的设置。

### 规则逻辑

- 单条隧道下的规则分为两组、按**自上而下**的顺序评估：**disconnect（断开）组先评估，connect（连接）组后评估**。组内顺序就是编辑器中拖拽排序的优先级。
- **规则内 AND、规则间 OR、首条命中生效**：一条规则下的所有条件必须全部满足，规则才算命中；但在所有规则中**只有第一条命中**的规则会执行动作。因为 disconnect 组先于 connect 组评估，已命中的 disconnect 规则永远比其后命中的 connect 规则优先级高——connect 规则会被标记为「命中但被降权」而不会执行，因此不会出现同一 SSID 下既 disconnect 又 connect 的自相矛盾。
- **否则（none-match）规则**：位于每个动作组末尾的兜底规则卡片，只有该动作组前面的规则**全部未命中**时才触发。
- **编辑时实时匹配指示器**：打开自动化编辑器后，每个条件会即时显示是否命中当前网络，实际生效的首条命中规则会被高亮为「in use（正在使用）」，顶部还有一条裁决条显示该隧道的最终动作。指示器会在每次编辑后立即刷新（≈ 250 ms 防抖，通过 IPC 调用与后台 helper 执行控制**相同**的评估引擎，因此界面显示与真实行为永远一致）。同引擎也可通过命令行调用：`wireguideplus automation`，适合无界面环境排查。

### 条件类型

| 条件 | 匹配逻辑 | 典型场景 |
| --- | --- | --- |
| **SSID** | 与当前 Wi-Fi 的 SSID 做**字节级全名精确比较**（区分大小写，中间空格与特殊字符全部参与匹配，符合 802.11 定义）。 | 「连接到 `公司 5GHz` 时自动连办公 VPN」。 |
| **子网（Subnet）** | 当前本机物理网口 IP 是否落在指定 CIDR 段（如 `192.168.178.0/24`）。 | 家用/办公路由器 LAN 段可预测、但 SSID 不稳定时。 |
| **网络 / BSSID** | 当前默认网关的 MAC 地址（BSSID）。锁定某个具体的物理 AP，而不只是它的 SSID。 | 「咖啡店那个共享路由器**永远不要**自动连接。」 |
| **网关 IP** | 当前物理网络默认网关的 IP 地址。 | SSID 太泛化（全公司叫相同 SSID）、但每个楼层网关 IP 不同时。 |
| **网卡接口（Interface）** | 当前系统作为上行路由的物理网卡名称。下拉列表列出所有已存在的物理网卡，包括**尚未连接**的设备，方便提前为扩展坞、USB 网卡、雷电网卡等未插入的设备写好规则。 | 「只有插在公司扩展坞上的有线网卡时才连接办公 VPN。」 |
| **在有线网络（Ethernet）** | 上行流量通过非 Wi-Fi 的有线适配器路由时即命中，不依赖 SSID 信息。 | 「插网线办公时必连 VPN；切换 Wi-Fi 不需要。」 |
| **时间段** | 一组星期几 + 起止时间（本地时间）。 | 「周一至周五 09:00–18:00，办公 VPN 必须保持在线。」 |

一条规则可以自由组合以上条件：例如「SSID = 公司 AND 时间段 = 周一至周五 09–18」就是一条带两个 AND 条件的规则。每个隧道的 disconnect / connect 两组都支持任意数量的 AND 规则。

## 平台支持

| 平台 | 状态 |
| --- | --- |
| Windows 10 / 11（x64、x86 32 位、ARM64） | ✅ 完全支持（多隧道并发 + SSID 自动连接，含 AmneziaWG） |
| macOS（Apple Silicon / arm64） | ✅ 完全支持 — 已在 Apple Silicon 真机充分验证；你同样可以尝试另外一款名叫 [WireTunnels](https://github.com/FMDigitech/WireTunnels) 的 app |
| Linux（x64、arm64） | 🚧 实验性 — 经 CI 构建，尚未在真机测试 |
| Android / iOS | ❌ **不支持**（无法并发运行隧道，也无法按 Wi-Fi SSID 自动切换隧道） |

> **macOS 替代方案：[WireTunnels](https://github.com/FMDigitech/WireTunnels)** — 原生
> macOS 菜单栏 WireGuard 客户端，支持多隧道、监控与控制，可作为上游 `wireguide`
> 的补充。

### 为什么没有移动版？

本项目的核心能力是**多隧道并发**与**按条件自动连接（如按 Wi-Fi SSID）**。在
Android / iOS 上，系统内核与权限机制使 WireGuard 实现**无法同时运行多条隧道**，
也无法**按 Wi-Fi SSID 自动切换隧道**——两大核心目标在移动端都不可实现。因此本项目
**明确不做移动版**；移动端单隧道需求请使用官方 WireGuard App 的 On-Demand 能力。

## 路线图

- **v2.0（规划中）**：以 **Windows 系统服务**方式运行 — 无需用户登录即可自动连接，
  网络栈更稳定、权限控制更完善。

## 下载与安装

每个 Release 将 Windows 构建分为两类分别发布：**安装包**与**绿色版（便携版）**。

**安装包（推荐）**

- Windows x64 安装包：`wireguideplus-amd64-installer.exe`
- Windows x86（32 位）安装包：`wireguideplus-x86-installer.exe`
- Windows ARM64 安装包：`wireguideplus-arm64-installer.exe`

安装包文件名内嵌架构信息（`wireguideplus-<arch>-installer.exe`，arch 为
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
zip（`wireguideplus-amd64-portable.zip` / `wireguideplus-x86-portable.zip` /
`wireguideplus-arm64-portable.zip`），内含 exe **和**对应架构的驱动 DLL——下载后解压
即可运行。Release 不再单独附驱动 DLL（请使用便携 zip 或安装包）。缺少匹配的驱动 DLL
时无法创建隧道。

## 代码签名

所有发布的 Windows **安装包**均经过 Authenticode 签名，可同时验证**完整性**（二进制在
传输或磁盘上未被篡改）与**来源**（由本项目构建并发布）。已签名的二进制在首次运行时
也会触发更少的 Windows SmartScreen 警告。

注意：**仅安装包**经过签名；便携版 zip 内为未签名的构建产物。完整签名政策（范围、审批
流程、账户安全与可复现性）见 [SIGNING-POLICY.md](SIGNING-POLICY.md)。

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 构建与开发

构建环境依赖、开发 / 发布构建命令（含 x86 + amd64 + arm64 多架构构建）、NSIS 安装包说明、
版本资源与发布流程见独立开发文档 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。发布只需
本地推送版本标签，GitHub Actions 流水线会自动构建、签名并发布 Release
（见 [docs/release.md](docs/release.md)）。

## 数据与日志

| 项目 | 位置 |
| --- | --- |
| 设置 / 历史 | `%APPDATA%\wireguideplus\`（`config.json`、`history.json`） |
| 隧道配置 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| 日志 | `%APPDATA%\wireguideplus\logs\` |

## 卸载

通过 **控制面板 → 程序和功能 → WireGuide Plus** 卸载，或运行安装目录下的卸载程序。

## 致谢

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 上游开源项目
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
