# WireGuide Plus 2.0 — Windows 系统服务化开发指导

> 目标版本：2.0
> 范围：把 1.x 的「GUI 驱动 + 提权 helper」架构改造为「Windows 系统服务常驻」架构
> 关联代码：`main.go`、`internal/helper`、`internal/ipc`、`internal/gui`、`internal/elevate`、`internal/cli`、`internal/autostart`、`build/windows/nsis`

---

## 1. 目标与收益

2.0 的核心目标：**核心引擎（隧道管理、防火墙、自动化）作为 Windows 系统服务常驻运行**，开机自启、无人值守，不依赖 GUI 是否存在。

| 收益 | 说明 |
|---|---|
| 开机即就绪 | 服务由 SCM 在登录前启动，无需用户登录/UAC 弹窗 |
| 无人值守 | WiFi 规则自动连接、断线重连、崩溃恢复在无 GUI 时照常工作 |
| 稳定性 | SCM 恢复策略（服务崩溃自动重启）比现 UAC 拉起更可靠 |
| 多会话 | 服务位于 session 0，锁屏/注销后隧道不中断 |
| 解耦 | GUI 只是客户端，可单独更新/降级/关闭 |

## 2. 现状盘点

### 2.1 1.x 架构

```
┌─ 用户会话 ──────────────────────────────┐
│  GUI (Wails3 + React)                   │
│    │  ensureHelper(): UAC 拉起 / 直连    │
│    ▼                                    │
│  elevated helper 进程 (管理员)           │
│    隧道管理 / WFP 防火墙 / 重连 / WiFi 自动化│
└─────────────────────────────────────────┘
   IPC: 命名管道 `\\.\pipe\wireguide…`
   安全: SDDL = SY+BA 全权, 拉起用户 GRGW; 客户端校验管道所有者 ∈ {SY, BA}
```

- **入口**：`main.go` flag 分发（`--helper` / GUI / `ctl`）
- **helper 内核**：`internal/helper/helper.go` 的 `Run(addr, ownerUID, ownerSID, dataDir)`，已实现：
  - `tunnel.Manager`：wireguard-go + wintun，连接/断开/状态
  - `firewall`：WFP kill switch / DNS 保护 / 端点保护（`internal/firewall/wfp_windows.go`）
  - `reconnect.Monitor`：断线重连 + 防火墙挂起/恢复
  - `wifi.Monitor` + 规则评估：SSID/子网触发自动连接（`internal/helper/wifi_rules.go`）
  - 崩溃恢复：`tunnel.RecoverFromCrash` + `fw.RecoverFromCrash`
  - 事件广播：`ipc.Server.Broadcast`（状态 diff、WiFi 变化、重连状态）
  - 启动时恢复持久化设置（健康检查、PinInterface、日志级别）
- **生命周期（1.x 是 GUI 驱动的关键点）**：
  - `shutdownGrace = 10s`：GUI 断开且无活动隧道 → 自退（`armShutdownTimer`）
  - `startupGrace = 60s`：启动后无 GUI 连接 → 自退
  - 活动隧道存在时永不退出（wg-quick 语义）
- **IPC 安全模型**（`internal/ipc/transport_windows.go`）：
  - 管道 SDDL：`D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<ownerSID>)` —— 服务化后**直接复用**
  - 客户端 `verifyPipeOwner`：要求管道所有者 ∈ {SY, BA}，防冒名
  - 服务端 `verifyPeer`：要求连接进程属主用户 = ownerSID
  - 连接数上限 32（防同 UID 进程耗尽资源）
- **数据目录**：`systemDataDir()` = `%PROGRAMDATA%\wireguideplus`（helper 状态）
- **用户配置**：`storage.TunnelStore` 在用户 home。⚠️ Windows 上 `deriveUserAppSupport`（`internal/helper/wifi_rules_windows.go`）靠 UAC 透传的 `APPDATA` 环境变量定位用户目录——**SCM 启动的服务不带用户环境**，此路径在服务模式下会失效，是阶段 2 必须处理的点
- **CLI**：`wireguideplus ctl …` 已是独立 IPC 客户端（Tailscale 模式）
- **安装**：NSIS（`build/windows/nsis/project.nsi`）+ `internal/autostart`（Windows 用 HKCU Run key）

### 2.2 与 2.0 的差距

| # | 差距 | 影响 |
|---|---|---|
| 1 | 生命周期由 GUI 驱动（断连即退） | 服务必须常驻，退出逻辑需模式化 |
| 2 | 由 GUI 以 UAC 拉起（`elevate.SpawnHelper`） | 需改为 SCM 注册 + 开机自启 |
| 3 | 单用户绑定（ownerSID） | 服务运行于 session 0，需处理多用户 |
| 4 | 用户配置在用户 home | 服务以系统身份读取有权限/路径问题 |
| 5 | 无服务控制协议 | 需 Start/Stop/Pause 状态映射 + 安装/卸载子命令 |
| 6 | 日志仅文件/广播 | 服务建议补充 Windows Event Log |
| 7 | 更新流程假定 helper 由 GUI 管理 | 服务更新需 SCM 停机/重启 |

## 3. 目标架构

```
┌─ SCM ────────────────────────────────────────────┐
│  wireguidesvc  (Windows 服务, LocalSystem)        │
│   ── 就是现有 helper 内核, 常驻, 无 GUI 依赖       │
│   隧道管理 / WFP 防火墙 / 重连 / WiFi 自动化        │
└──────┬────────────────────────────────────────────┘
       │ 命名管道 (SDDL 不变, SY/BA/用户)
       ├──────────────┬───────────────────┐
       ▼              ▼                   ▼
   GUI 客户端     ctl CLI (已有)       (可选) 远程/监控
   (Wails,        `wireguideplus ctl`   HTTP 管理面 (复用
    不再 UAC 拉起)                        -tags server 思路)
```

**组件拆分原则**：不新建大模块。服务 = 现有 `helper.Run` + 一层 `svc.Handler` 外壳；GUI = 现有 Wails 前端 + `ensureHelper` 改为「连接服务，未安装则引导安装」；CLI 原样复用。

## 4. 技术选型

| 方案 | 说明 | 结论 |
|---|---|---|
| **A. `golang.org/x/sys/windows/svc`** | Go 官方对 SCM 的支持，零额外依赖；`svc.Run` + `Handler.Execute` 控制 Start/Stop/Pause | ✅ 推荐。Windows 专用、控制精确，与现有 `x/sys/windows` 依赖一致 |
| B. `kardianos/service` | 跨平台封装（win/mac/linux 同一 API） | 备选。若未来要统一 launchd/systemd 管理代码再用 |
| C. 外部工具（NSSM/winsw） | 把任意 exe 包装成服务 | ❌ 增加外部依赖，且拿不到 SCM 生命周期事件 |

- **服务账户**：`LocalSystem`（需要写 `%PROGRAMDATA%`、加载 wintun 驱动、操作 WFP、访问网络）。不要用 `NetworkService`（无驱动加载与 WFP 权限）。
- **管道名**：服务实例使用全局管道，建议 `\\.\pipe\wireguide`（与 `ipc.DefaultSocketPath()` 区分），多用户阶段按需扩展（见 5.3）。

## 5. 实施路线

### 阶段 0 — 服务外壳（不动 helper 内核）

**改动**
1. 新增 `internal/service/service_windows.go`：
   - 实现 `svc.Handler`：`Execute(args, requests, changes)` 循环处理
     `SVC_CONTROL_STOP` → 调用 `h.server.Shutdown()`（复用现有干净退出路径）；
     `SVC_CONTROL_INTERROGATE` → 报告 Running。
   - `Run()` 内构造与 `helper.Run` 相同的依赖（`tunnel.NewManager`、`firewall.NewPlatformFirewall`、`reconnect.NewMonitor`、`wifi.NewMonitor`），通过 **`svc.Run(serviceName, handler)`** 进入 SCM 控制循环。
2. `main.go` 增加 `--service` 分支：先 `svc.Run`，非服务上下文（无 SCM）则提示并退出。
3. 新增 `ctl svc install|uninstall|status|start|stop` 子命令（`internal/cli/cli.go` 的 switch 增加分支）：
   - `install`：`OpenSCManager` + `CreateService`（`SERVICE_AUTO_START`、`SERVICE_WIN32_OWN_PROCESS`、启动参数 `--service --data-dir <ProgramData>`）
   - `uninstall`：停服务 → `DeleteService`
4. **NSIS 集成**（`build/windows/nsis/project.nsi`）：安装时静默注册服务；卸载时先停+删服务。与现有 `internal/autostart` 的 HKCU Run key 二选一（服务安装后移除 Run key）。

**验收**
- `wireguideplus ctl svc install && ctl svc start` 后，服务在 services.msc 可见、可启停。
- 停止服务时隧道断开、防火墙规则清理（走现有 `cleanup()` 路径）。
- GUI 模式行为完全不变（回归）。

### 阶段 1 — 生命周期解耦（关键改造）

**问题**：`internal/helper/helper.go` 的 `startupGrace`/`shutdownGrace` 逻辑会让服务「无 GUI 连接就自退」，与服务常驻目标冲突。

**改动**
1. 给 `Helper` 增加运行模式字段（如 `mode`：`modeGUI` / `modeService`），由 `--service` 传入。
2. `armShutdownTimer` 中：`modeService` 下**跳过所有自退定时**（`OnConnect`/`OnDisconnect` 仍保留，用于控制事件推送与 `HasControlConn` 判断，但不再触发 `shutdown()`）。
3. `main.go` 的 `--service` 路径直接调用 `helper.Run(..., helper.WithMode(service))` 或新增 `helper.RunService(...)`。
4. 保留「活动隧道保护」逻辑，作为服务停止时的最后防线（SCM Stop 请求仍走 `Shutdown`）。

**验收**
- 服务启动后 GUI 全程不开，隧道/WiFi 自动化照常工作。
- GUI 打开 → 连接 → 关闭，服务保持运行。
- `sc stop wireguidesvc` 能干净停止。

### 阶段 2 — 多用户与会话 0

**问题**：服务运行在 session 0 的 LocalSystem 下，用户配置（`storage.TunnelStore`）位于各自 home。当前 Windows 实现（`deriveUserAppSupport`）靠 UAC 拉起时透传的 `APPDATA` 环境变量定位用户目录——**SCM 启动的服务不带任何用户环境变量**，必须改为显式路径。

**决策点（建议 v2.0 取舍）**
- **方案 A（推荐，范围小）**：配置根统一到系统级 `%PROGRAMDATA%\wireguideplus\`（含 `tunnels/`、`config.json`），服务直接读写；`storage.TunnelStore` 增加可配置根目录构造（现为 `App Support/tunnels`，加一个参数即可）。单用户即可，后续再演进。
- **方案 B（多用户完整）**：每用户一套配置目录，管道名带用户 SID（`\\.\pipe\wireguide-<SID>`），服务按活动会话切换上下文。复杂度高，建议 2.1。
- **GUI 行为**：GUI 检测配置位置并兼容迁移（1.x 已有配置 → 复制到新位置）。

**改动**
1. `systemDataDir()`（`main.go`）在 `--service` 模式下固定为 `%PROGRAMDATA%\wireguideplus`。
2. `storage.TunnelStore` 构造增加 root 参数；迁移现有 `tunnels/`。
3. 管道 SDDL：服务模式下 `ownerSID` 校验的语义调整——服务由 LocalSystem 拥有，`verifyPeer` 应校验「连接者是本机任一已登录用户」（枚举活动会话）而非单一 SID；或先保持宽松（BA 组成员）并记录安全评审。

**验收**
- 安装服务后，任一管理员用户登录都能通过 GUI/ctl 控制隧道。
- 锁屏/注销期间隧道持续；重新登录后 GUI 显示真实状态（事件重放或状态拉取）。

### 阶段 3 — GUI 对接改造

**改动**
1. `internal/gui/helper_lifecycle.go` 的 `ensureHelper`：
   - 优先尝试连接服务管道（新增 `ipc.Dial` 指向服务地址）。
   - 连接失败 → 提示「服务未运行」，提供一键安装/启动（调 `ctl svc install`，或新 RPC `Svc.Install`）。
   - **删除** UAC 拉起路径（`elevate.SpawnHelper`）及版本匹配重启逻辑（服务升级由 SCM 管理，见阶段 5）。
2. 版本校验：服务响应增加 `AppVersion`（已有 `PingResponse`），GUI 检测服务过旧时提示用户运行更新（不再自己强杀）。
3. `internal/autostart`：Windows 路径改为「确保服务存在」，删除 HKCU Run key 逻辑（macOS/Linux 保留原样）。

**验收**
- 全新机器：装完 → 首次打开 GUI → 引导安装服务 → 正常使用，全程无 UAC。
- GUI 崩溃/被杀：服务与隧道不受影响；重开 GUI 秒连。

### 阶段 4 — 服务控制面与日志

**改动**
1. RPC 增加：`Svc.Status`（SCM 状态 + 隧道状态合并）、`Svc.SetAutostart`。
2. 日志：
   - 保留现有 `broadcastHandler`（GUI 订阅）+ `helper-stderr.log`。
   - 新增 Windows Event Log 输出（`golang.org/x/sys/windows/svc/eventlog`）：服务启停、隧道连接/断开、防火墙告警、崩溃恢复。
3. 崩溃恢复的 SCM 侧配合：`CreateService` 时设置 `SERVICE_CONFIG_FAILURE_ACTIONS`（失败 → 延迟 5s 重启，最多 3 次），配合现有 `RecoverFromCrash`。

**验收**
- `eventvwr` 中能看到服务/隧道关键事件。
- 人为杀掉服务进程 → SCM 自动重启 → 隧道恢复（日志可见）。

### 阶段 5 — 更新与回滚

**改动**
1. `internal/update` 的服务适配：GUI 执行更新时，新增「经 SCM 停服务 → 替换 exe → 启动」的原子流程（`ctl svc stop` + 文件替换 + `ctl svc start`），避免「GUI 强杀服务」。
2. 服务侧自更新可滞后实现（2.1）：先保持「GUI 触发、SCM 托管重启」模式。
3. 签名校验逻辑不变（Ed25519 + SHA256SUMS）。

**验收**
- 升级/降级后服务与 GUI 版本一致（`PingResponse.AppVersion` 校验），隧道在升级窗口内完成重连。

### 阶段 6 — 安全强化

1. 服务账户最小化：确认 LocalSystem 权限面；必要时拆「服务（LocalSystem）+ 工作进程（低权限）」（如 Tailscale 的 tailscaled 模型）。
2. 管道 SDDL 复查：`SY/BA` 全权、普通用户仅 `GRGW`（现有定义已满足）；会话 0 下确认无交互式桌面注入面。
3. 输入校验：服务不信任任何客户端（`maxConcurrentConns`、`verifyPeer`、`ReadDeadline` 现有防护保留并测试）。
4. 卸载安全：停服务可能导致 kill switch 失效——卸载流程须先断网保护告警，再停服务，最后删防火墙规则。

## 6. 关键设计决策摘要

| 决策 | 结论 | 理由 |
|---|---|---|
| 服务账户 | LocalSystem | 需要 wintun 驱动 + WFP + 写 ProgramData |
| 服务技术栈 | `x/sys/windows/svc` | 零额外依赖、生命周期事件完整 |
| 服务 = 现有 helper | 是（外壳化） | 内核能力已齐备，避免重写 |
| 多用户 | 方案 A（系统级配置根） | 控制 v2.0 范围 |
| GUI 拉起方式 | 删除 UAC，改引导安装服务 | 消除每次登录弹 UAC |
| 日志 | 文件 + Event Log 双写 | 无人值守场景可观测性 |
| 更新 | GUI 触发 + SCM 托管重启 | 2.1 再考虑服务自更新 |

## 7. 测试计划

### 单元
- `armShutdownTimer` 在两种模式下的行为（GUI 模式回归 / 服务模式常驻）
- `storage.TunnelStore` 新 root 参数
- `ctl svc` 子命令参数解析与错误处理

### 集成（Windows 真机）
1. 全新安装 → `ctl svc install` → 服务注册成功、`AUTO_START`
2. 开机（不登录）→ 服务已运行 → 登录后 GUI 直连
3. 无 GUI 期间：WiFi 规则自动连接、断线自动重连（断网 30s 重连）
4. 杀掉服务进程 → SCM 自动重启 → 崩溃恢复拉起隧道
5. `sc stop` / 卸载：隧道断开、防火墙清理、kill switch 不残留
6. 升级：GUI 触发 → 停服务 → 替换 → 启动 → 版本一致
7. 多用户登录切换：隧道不中断，第二个用户 GUI 状态正确
8. 回归：macOS/Linux 的 LaunchAgent/LaunchDaemon/systemd 路径不受影响

### 手动清单
- 用 `docs/manual-test-checklist.md` 覆盖 GUI 模式全功能回归。

## 8. 里程碑与交付

| 里程碑 | 内容 | 预计改动面 |
|---|---|---|
| M1 | 阶段 0：服务外壳 + 安装/卸载 + NSIS 集成 | `internal/service`(新)、`main.go`、`cli.go`、`project.nsi` |
| M2 | 阶段 1：生命周期解耦 | `helper.go`（模式字段 + 定时器分支） |
| M3 | 阶段 2：系统级配置根 + 会话 0 适配 | `storage`、`systemDataDir`、`transport_windows.go` |
| M4 | 阶段 3：GUI 对接 | `helper_lifecycle.go`、`autostart` |
| M5 | 阶段 4-6：控制面/日志/更新/安全 | `ipc`、`update`、NSIS |
| M6 | 全量测试 + 发布 | 测试计划 + release 流程 |

**发布提示**：2.0 是破坏性升级，release notes 需说明「服务化带来的行为变化」（如：卸载流程、配置目录迁移、UAC 消失）。

## 9. 风险与注意事项

1. **kill switch 与服务生命周期**：服务被强制停止（用户/异常）会瞬时失去 kill switch 保护——SCM 恢复策略 + 现有崩溃恢复是主要防线，务必测试。
2. **配置迁移**：1.x 用户已有配置在 `%APPDATA%`，迁移失败会导致「看不见隧道」。迁移脚本要幂等、可回滚。
3. **多用户首版简化**：若采用方案 A，多个 Windows 账户共享一套配置——需在 UI 明示「单配置」或做账户隔离提示。
4. **勿删 `--helper` 模式**：macOS/Linux 及 Windows 开发调试仍依赖现有 UAC helper 路径，服务化是新增路径而非替换（发布后再评估下线）。
