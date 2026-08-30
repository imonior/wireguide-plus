# Changes relative to upstream korjwl1/wireguide v0.5.1

This repository is **WireGuide Plus**, a hard fork of the open-source WireGuard client
[`korjwl1/wireguide`](https://github.com/korjwl1/wireguide) (MIT license), based on
upstream **v0.5.1** (2026-08-11).

This file records everything that changed on top of the upstream 0.5.1 codebase. It is
kept here as a standalone reference because this repository starts with a single squashed
initial commit (no upstream history is carried over). The canonical per-version release
notes live in [CHANGELOG.md](CHANGELOG.md).

---

## 1.1.1 (2026-08-30) — WireGuide Plus

修复 Windows 托盘通知气泡「打开主界面」按钮在系统高负载下偶发导致 GUI 卡死的问题。

### 🐛 修复（Bug Fixes）

- **通知气泡「打开主界面」偶发卡死** — 高 CPU 争用（如 Windows 维护进程占满核心）或
  WebView2 延迟时，点击托盘通知气泡的「Open Window」会同步阻塞等待 UI 线程，整个 GUI
  看似冻结（VPN 隧道不受影响）。`showDock`（`internal/gui/dock_other.go`）改为经
  `application.InvokeAsync` 在 Wails UI 线程异步执行，并加 recover 防护。

### 🛠 内部（Internal）

- 版本号 1.1.1（`internal/update/checker.go` + 全部构建配置同步）。

---

## 1.1.0 (2026-08-28) — WireGuide Plus

迭代版本：托盘状态图标可辨识化、代理三模式语义明确并新增连通性测试、无效代理 URL 校验
（不再拖垮更新检查）、启动自动化规则优先（避免先连后断）。

### ✨ 新功能（Features）

- **托盘状态图标可辨识化** — Windows 托盘菜单连接状态改用 `●` 实心 / `○` 空心字形区分
  （Windows 托盘弹窗由 GDI 绘制，无法渲染彩色 emoji）；macOS 继续用彩色 emoji。
- **代理三模式语义明确 + 连通性测试** — 直连（忽略系统/环境代理）/ GitHub 镜像
  （`mirror` 加速前缀，如 `https://ghfast.top`）/ 手动代理（`manual`，http/https/socks5 URL）
  三者清晰区分；设置页新增"测试连接"按钮，保存前验证代理可达性。
- **代理设置即时生效** — 保存后下一次更新检查无需重启即生效；GUI 启动直接套用已保存代理。

### 🐛 修复（Bug Fixes）

- 无效手动代理 URL（如 `proxy_url = "https://"`）此前导致更新检查持续报
  `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify`；现于启动和每次使用时
  校验 URL，无效值记录 WARN 并回退直连。
- 启动"先连接、后按规则断开"：启动规则评估提前到 helper 启动后立即执行，并新增
  `scheduleRuleCheck`——启动 60s 窗口内任何手动连接 3 秒后按规则纠正，日志记录触发来源。
- 无效 mirror 前缀同样校验 scheme/host，非法值回退官方 API 端点。

### 🛠 内部（Internal）

- 版本号 1.1.0（`internal/update/checker.go` + 全部构建配置同步）。
- Windows 版本资源标准化：`wails3 generate syso` 生成的版本资源语言 `0x0000` 且 ProductVersion 为零导致属性页空白，
  改用 `goversioninfo`（`build/windows/versioninfo.json`）生成标准 `0409/04B0` 资源。
- 新增 Windows x86（32 位）构建：`task windows:build ARCH=386`，NSIS 脚本支持 x86 架构与 `Program Files` 安装目录。
- 移除 iOS 构建任务与配置，明确不支持 Android/iOS（无法多通道并发、无法按 SSID 自动连接）。
- 新增「最小化启动」设置与连接状况托盘通知气泡：启动后 / 连接状态变化时延迟 10 秒显示，停留时长可设置（Windows，`internal/gui/notify_windows.go`）。
- Windows 物理网卡适配器名匹配逻辑调整。
- 窗口标题统一为 **WireGuide Plus**。
- 更新检查调度器内去重，同一轮只记录一次失败。

---

## 1.0.0 (2026-08-28) — WireGuide Plus

里程碑版本：品牌重命名、A11y 无障碍语义重构、Windows 网络出口选路逻辑调整、Wails3
构建/图标/权限梳理，并新增简体中文界面与托盘开关。

### ✨ 新功能（Features）

- **简体中文界面（Chinese UI）** — 全界面新增简体中文翻译，覆盖隧道列表、历史、工具
  （DNS 泄漏测试/路由表）、日志、设置、更新、自动化编辑器等全部 199 条文案。首次启动
  自动跟随系统语言（`zh-*` 区域自动识别），也可在 设置 → 常规 → 语言 中手动切换并持久化。
- **托盘菜单开关（Tray toggles）** — 系统托盘内每条隧道变为独立可点击的开关：勾选连接、
  取消勾选断开；连接状态 emoji（🟢 已连接/🟡 连接中/○ 断开）保留在标签旁。手动关闭的
  隧道保持豁免自动规则（manual-off），直到重新连接或重启 WireGuide。

#### 前端 A11y 无障碍重构

> 影响：全平台（Windows/macOS/Linux）Svelte 前端，不限于 Windows。

- 全部模态弹窗移除蒙层 `role="button"` 与 `tabindex="0"`，蒙层回归纯粹遮罩语义。
- 所有 dialog 统一 `tabindex="-1"` 并保留标准 `role="dialog" aria-modal="true"`。
- ESC 关闭统一处理：缺失的弹窗（导入结果、历史、更新提示、自动化编辑器）在组件顶层挂
  `<svelte:window on:keydown>`，其余复用 App.svelte 全局 capture 处理器。
- `Settings.svelte`：`<nav role="tablist">` 改为普通 `<div>`；分割条 `pane-resizer`
  补 `tabindex="0"` 与真实键盘操作（方向键调整宽度、Enter/Space 复位）。
- `frontend/vite.config.js` 的 svelte 插件 `onwarn` 过滤静态误报，生产构建警告归零。
- 涉及文件：`src/App.svelte`、`src/lib/History.svelte`、`src/lib/ConflictWarning.svelte`、
  `src/lib/TunnelDetail.svelte`、`src/lib/UpdateNotice.svelte`、`src/lib/Settings.svelte`、
  `src/lib/AutomationEditor.svelte`

#### Windows 后台 helper：网络出口选路逻辑调整

> 影响：仅 Windows 平台 Go helper 代码，其他平台不受改动。

- helper 启动阶段采集主上游物理网卡 LUID，用于记录系统初始默认出站物理接口（启动快照）。
- 修正网络接口筛选逻辑：过滤 TUN/隧道/回环虚拟网卡，仅选取物理网卡作为上游候选。
- WireGuard UDP 报文出站完全交由 Windows 路由表 + 网卡 InterfaceMetric 跃点数选路。
- 分流模式（`full_tunnel=false`）：Peer Endpoint IP 需显式加入 `AllowedIPs`，防止握手
  UDP 报文路由丢弃导致 `no-handshake`。
- 日志增强：`network primary upstream interface initial luid`、明确 `tunnel connected`
  仅代表 TUN 适配器就绪。
- 排查工具提示：Windows 下优先使用 `Find-NetRoute -RemoteIPAddress <peer-ip>`；
  PowerShell `Get-NetAdapter.Luid` 为结构体，不可直接与 Go uint64 等值比对。

### 🏷 品牌重命名（Rebranding）

- 产品名：**WireGuide Plus**；公司/publisher：**imonior**；Go 模块路径
  `github.com/korjwl1/wireguide` → `github.com/imonior/wireguide-plus`；bundle ID
  `com.korjwl1.wireguide` → `com.imonior.wireguide-plus`。
- 全局替换 76+ 文件：Go 源码、frontend bindings、全部 build 资产（config.yml /
  info.json / msix / nsis / plist / manifest / nfpm）、文档、CI。
- `LICENSE` 保留上游 MIT 版权声明并新增 `Copyright (c) 2026 imonior`。
- `README.md` / `README.ko.md` 顶部新增 fork 致谢段落（链接到原项目并致谢）。

### 🛠 构建与工程（Build & Project）

主要为 Windows 构建行为，跨平台部分已标注。

1. **Wails3 Windows 图标构建行为**（仅 Windows）——`task build` 完整构建会自动执行
   `wails3 generate icons`，读取 `build/appicon.png` 并覆盖输出 `windows/icon.ico`；
   `task windows:build` 调试构建跳过图标生成。exe / 窗口标题栏 / 任务栏图标复用 exe 内
   ico 资源；系统托盘图标需要 Go `embed` 独立资源。
2. **Windows 版本信息管理**（仅 Windows）——exe 文件详细信息由 `windows/info.json`
   控制，`FileVersion` 必须 4 段数字格式。UI 展示版本由 Go 常量维护
   （`internal/update/checker.go`），需与 `info.json` 手动保持同步。
3. **Windows UAC / 管理员权限梳理**（仅 Windows）——`windows/wails.exe.manifest` 添加
   `requireAdministrator`，将 UAC 弹窗转移到 exe 双击启动；长期建议 helper 重构为
   Windows System Service 以彻底消除 UAC 弹窗。

### 🐛 问题排查记录（Investigation）

排查记录，无代码变更，供开发参考。

- helper 日志输出 `tunnel connected` 但 GUI 显示 `no handshake`：TUN 设备创建完成 ≠
  WireGuard 与远端 Peer 完成加密握手；分流模式高频踩坑为 Peer IP 不在 `AllowedIPs`。
- 本地代理监听 `0.0.0.0`：代理进程流量独立，不会自动流入 WireGuard 隧道。

### 📝 说明（Notes）

1. **改动影响范围区分**
   - Svelte 前端 A11y 代码：**全平台生效**。
   - helper 网络出口选路逻辑：**仅 Windows 平台 Go 代码**。
   - 构建、manifest、ico、info.json、UAC 相关：**仅 Windows 平台**。
2. 前端 A11y 修改与 helper 后台网络逻辑完全解耦。
3. helper 记录的上游 LUID 仅为启动瞬间快照，网络切换时不会自动更新。
