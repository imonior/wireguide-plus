# WireGuide Plus

**一款 Windows 上的多隧道、自动化优先的 WireGuard 客户端**

WireGuide Plus 是对开源项目 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)
进行**深度修复与增强**的版本。两大核心能力：

- **多隧道并发** — 多条 WireGuard 隧道可同时建立、互不干扰地独立运行；
- **条件自动连接** — 按 Wi-Fi SSID、时间段、系统启动等条件自动连接对应隧道
  （例如办公 Wi-Fi 下连隧道 A，家里连隧道 B）。

[English](README.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [한국어](README.ko.md) | [日本語](README.ja.md)

> **Windows 10 / 11（x64、x86 32 位与 ARM64）完全支持**。macOS（Apple Silicon）与
> Linux（x64、arm64）提供**实验性预览版** — 经 CI 构建，尚未在真机测试
> （见 [平台支持](#平台支持)）。**不支持 Android / iOS。**

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

## 平台支持

| 平台 | 状态 |
| --- | --- |
| Windows 10 / 11（x64、x86 32 位、ARM64） | ✅ 完全支持（多隧道并发 + SSID 自动连接） |
| macOS（Apple Silicon / arm64） | 🚧 实验性 — 经 CI 构建，尚未在真机测试；你同样可以尝试另外一款名叫 [WireTunnels](https://github.com/FMDigitech/WireTunnels) 的 app |
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
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintundll)）。Release 提供打包好的便携
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
