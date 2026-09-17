# 把特权 helper 做成系统服务 — 三平台分析

状态：结论性分析（不建议直接实施，建议按 §6 的折中方案逐步推进）。
范围：开机自启的授权提示（Windows UAC / macOS 管理员对话框 / Linux polkit）
与 helper 常驻化之间的取舍。

---

## 1. 先纠正两个前提

### 1.1 需要 root 的不是 GUI，是 helper

当前架构（v2.2.0 源码）已经把特权部分单独隔离成一个无 GUI 的子进程：

| 组件 | 权限 | 职责 |
| --- | --- | --- |
| GUI（Wails 窗口） | 普通用户 | 界面、配置、历史、更新 |
| helper 子进程 | root / Administrator | 创建 TUN、装路由、改 DNS、ICMP 探测、健康检查 |
| 二者之间 | 本机 IPC | 命名管道（Windows）/ Unix socket（macOS、Linux） |

所以"要不要做成系统服务"这个问题的正确形式是 **"helper 要不要常驻"**，
而不是"整个应用要不要做成服务"。把带窗口的 GUI 放进服务里在三个平台上
都不可行（Windows 服务运行在 Session 0，没有桌面，Wails 窗口根本显示不出来；
macOS LaunchDaemon 同理；Linux system service 默认不在用户会话里）。

### 1.2 macOS 和 Linux 并不是"没有这个问题"

它们只是提示形式不同，而且提示频率取决于 helper 是否还活着：

| 平台 | 自启方式 | 提权方式 | 什么时候会提示 |
| --- | --- | --- | --- |
| Windows | `HKCU\...\CurrentVersion\Run` | `powershell Start-Process -Verb RunAs`（`internal/elevate/spawn_windows.go:41`） | 每次找不到存活的 helper 时弹 UAC。helper 在 GUI 断开 10s 后自行退出（`shutdownGrace`），只有在"还留着隧道"时才会存活到下一次启动 |
| macOS | `~/Library/LaunchAgents/*.plist` | `osascript` 管理员对话框安装 `/Library/LaunchDaemons/com.wireguideplus.helper.plist`（`RunAtLoad=false`） | 首次，以及 helper 退出后再次启动。plist 与二进制路径不变时不再重复询问 |
| Linux | `~/.config/autostart/*.desktop` | `pkexec` → polkit 对话框（`internal/elevate/spawn_linux.go:40`） | 同 macOS；**且**纯 CLI / 无 polkit agent 的会话里直接失败（报错，而不是弹框） |

补充两条容易被忽略的现状事实：

- helper 有两个"自杀"窗口：GUI 断开后 `shutdownGrace = 10s` 内无重连即退出
  （`internal/helper/helper.go:106`），启动后 60s 内没有任何 GUI 连上也会退出
  （`startupGrace`，专门防"授权弹框还挂着、GUI 已超时退出"留下的孤儿 root 进程）。
  这两条解释了提示的**重复**规律：同一次登录里关掉 GUI 再打开（10 秒之后、且没有
  隧道存活）会再弹一次；而**开机那次提示的根因不同** —— 开机时本来就不存在任何
  存活的 helper，也没有任何"已授权的入口"，所以只能重新走一次 UAC。
  换言之：10 秒规则决定"同一会话内要不要重复弹"，开机提示则只能靠 D 方案解决。
- macOS 的 plist 已经带 `KeepAlive.SuccessfulExit=false`，即**崩溃自动重启已经存在**，
  只有"正常退出"才不复活。所以"服务化能带来崩溃自愈"这个论点在 macOS 上并不成立。

macOS 的差别在于提示是"安装一次 LaunchDaemon"，之后只要 plist 与二进制路径
不变就不再问；Windows 的 UAC 因为是"每次拉起一个新的提权进程"，所以每次都要问。
Linux 的坑是 polkit agent 在平铺窗口管理器/纯终端会话下可能压根不存在，
那时 `pkexec` 直接报错而不是弹框 —— 这是"授权提示"之外的另一种故障。

结论：Windows 的痛感最强（每次启动必弹），但这三个平台都存在同一类问题，
只是强度和表现形式不同。

---

## 2. 可选方案

### A. 现状：GUI + 按需提权拉起 helper（helper 生命周期绑定 GUI）

- 优点：高权限进程存活时间最短；GUI 关闭即隧道关闭（与 `wg-quick` 语义一致，
  用户心智模型清晰）；没有常驻攻击面；卸载干净。
- 缺点：Windows 每次启动弹 UAC。macOS / Linux 在 helper 退出后再次启动也要重新授权。

### B. 整个应用做成系统服务（GUI 也进服务）

- 三平台都不可行或代价极高：Session 0 无桌面，需要额外的"会话代理"把窗口投到
  用户会话，复杂度、故障面、签名要求全部上一个量级。
- **明确不推荐。**

### C. 只把 helper 服务化：开机自启的高权限 helper + 普通权限 GUI

- Windows：`sc create` 注册为 Windows 服务，或注册一个"登录时以最高权限运行"的
  计划任务（`schtasks /create /rl highest /sc onlogon`）。
- macOS：LaunchDaemon 把 `RunAtLoad` 改成 `true`（现在故意是 `false`，见下），
  可再加 `KeepAlive`。
  > 注意：现有 plist 里已经写明为什么 `RunAtLoad` 必须是 `false` —— 一个开机即起、
  > 没有窗口也没有托盘图标的 root helper 会去求值用户的 Wi-Fi 自动化规则，
  > 在用户没有任何"我想要它活动"的信号时擅自连断隧道（`internal/elevate/spawn_darwin.go:98`）。
  > 也就是说 C 方案不是"换个开关"，而是要推翻一条已经写下来的设计决定；
  > 若确实要做，必须连带设计"开机时哪些自动化规则允许被求值"。
- Linux：`/etc/systemd/system/wireguideplus-helper.service` + `systemctl enable`。
- 优点：**零授权提示**，一次安装之后彻底安静；隧道可在 GUI 未运行时保持；
  服务崩溃能自动重启（`KeepAlive` / `Restart=always`；macOS 的崩溃重启现已具备）；
  GUI 崩溃不再连带隧道断开。
- 缺点：见 §3。

### D. 折中：helper 仍然"按需启动"，但用一条**已授权的固定入口**拉起

- Windows：首次安装（管理员一次）注册一个 `schtasks /rl highest` 计划任务，
  之后 GUI 用 `schtasks /run` 触发 —— **不再弹 UAC**，但 helper 依然是
  "GUI 需要时才存在"。这正是"既去掉提示、又不常驻"的解法。
- macOS：等价物是 LaunchDaemon + `launchctl kickstart`（`RunAtLoad=false` 保持），
  首次安装仍需一次管理员授权（现在就是这样）。
- Linux：可等价地用"已授权的 polkit 规则"（`.policy` + 允许特定 action 免密），
  或 systemd unit + `systemctl start`。
- 优点：拿到 A 的生命周期语义 + 免提示；放弃"常驻"换来了攻击面的收敛。
- 缺点：安装/升级需要一次提权；受 §3.1 的安全约束（条目指向的路径必须不可被普通用户写）。

### E. 让 GUI 自己以最高权限开机启动（不推荐）

Windows 上还有一个常见捷径：用 `schtasks /rl highest` 让**GUI**在登录时提权启动，
GUI 再以普通子进程方式拉起 helper —— 子进程继承父进程的提权令牌，于是全程无 UAC。

不建议：把 WebView 这类渲染不受信任内容的界面进程跑在 Administrator 下，是把
浏览器引擎的漏洞直接变成管理员权限漏洞。省一次点击不值这个代价。

---

## 3. 服务化（C/D）的代价与风险

### 3.1 安全：高权限条目必须指向不可被普通用户写的路径

这是整件事里最容易踩错、后果最严重的一条。

Windows 计划任务 / Linux systemd unit / macOS LaunchDaemon 只要引用了一个
普通用户可写的可执行文件路径（例如 `%LOCALAPPDATA%\...` 或应用自更新目录），
那么"任何能写那个文件的进程"就等价于获得了一次持久化提权 —— 这是标准的
本地提权（LPE）手法，EDR 会直接告警。

因此一旦选择 C/D，就必须同时满足：

1. helper 二进制安装到受保护路径（Windows `Program Files`、
   Linux `/usr/local/lib/`、macOS `/Library/PrivilegedHelperTools/` —— 现在
   macOS 已经是这样）；
2. 该路径只能由管理员/root 写；
3. **应用自更新流程要相应改造**：以前可以用户态覆盖自身二进制，现在更新 helper
   必须重新提权安装一次，或者由 helper 自己在校验签名后自更新。

这一条直接决定了"服务化"不是免费的：它把"零提示"的成本转嫁到了"更新每次要提权"
或者"自更新链路要重做成带签名校验的特权更新"。

### 3.2 语义：隧道从"会话内"变成"系统级"

当前设计（macOS 那段注释写得很明确）是"不做看不见的常驻 root 进程"，
隧道生命周期 = GUI 生命周期。服务化后必须重新回答：

- GUI 关闭了，隧道要不要继续？断还是不断？
- 多用户登录时，谁连的隧道算谁的？
- 隧道在登录前就起来（macOS/Linux 开机自启）时，DNS/路由在系统网络尚未就绪的
  窗口期怎么处理？（这正是 v2.2.0 那次 `TS453Dmini` 三次连不上的根因：
  helper 起来时 DNS 还没就绪，域名解析失败。常驻 + 开机自启会把这个问题
  从"偶发"变成"每次开机的必经路径"。）

需要新增一个明确的设置项（"退出 GUI 后保持隧道"），否则用户会看到
"我明明退出了，VPN 还连着"，这比多按一次 UAC 更难解释。

### 3.3 攻击面与权限收敛

- 常驻 root 进程把 IPC 变成唯一的信任边界。现有防护是够用的基础
  （Windows 按 SID 收敛管道 ACL + 逐连接对端校验、Unix 侧 peercred），
  但"常驻"意味着这个边界要 7×24 暴露，而按需启动时它只在会话期存在。
- Linux 上可以显著降低风险：systemd unit 加
  `CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW`、`AmbientCapabilities`、
  `ProtectSystem=strict`、`NoNewPrivileges=yes`，把"root"缩到"仅网络能力"。
  这是 C/D 在 Linux 上最值得做的一件事，收益比"去掉一次 polkit 弹框"大得多。
- Windows 服务无法像 Linux 那样做能力降级（TUN 与路由都需要管理员），
  只能靠"最小接口 + 严格对端校验 + 受保护路径"。
- macOS 的"正统"做法是 `SMJobBless`（需要签名 + 公证 + 特定 entitlement），
  现在用 `osascript` 写 LaunchDaemon 在 macOS 13+ 仍可用但不是推荐路径；
  真要走 C/D，最好一并升级到 SMJobBless。

### 3.4 可维护性与可调试性

- 服务日志与用户日志分离（Windows 事件日志 / launchd 统一日志 / journald），
  现有的"用户日志目录"约定要改。helper 的 daily log 目前写在用户目录，
  服务化后（尤其是 Linux/Windows 的系统会话）写入权限会失败 ——
  这是一个必然要处理的具体问题。
- 卸载残留：托盘退出 ≠ 卸载。三个平台都需要显式的"卸载 helper/服务"逻辑，
  否则用户卸载应用后仍留一条高权限自启项，这既是体验问题也是合规问题。

### 3.5 企业环境

- 常驻 root/Administrator 服务在企业里常被 EDR、合规基线直接拦截；
- "最高权限计划任务"是最典型的持久化手法之一，安全软件会标记；
- 反过来，按需提权（现状）在企业里通常更容易通过审核。

---

## 4. 收益 / 代价一览

| 维度 | A 现状 | C 常驻服务 | D 已授权入口按需启动 |
| --- | --- | --- | --- |
| Windows 启动提示 | 每次（helper 10s 后自杀） | 一次 | 一次 |
| macOS / Linux 提示 | 首次 + helper 退出后 | 一次 | 一次 |
| 隧道可在 GUI 关闭后保持 | 否 | 是 | 否 |
| 高权限进程常驻 | 否 | 是 | 否 |
| 崩溃自动恢复 | macOS 已有；Win/Linux 无 | 是 | 否（GUI 可重拉） |
| 自更新是否需要重新提权 | 否 | 是 | 是 |
| 卸载残留风险 | 低 | 高 | 中 |
| 本地提权（LPE）风险 | 低 | 中（取决于路径与加固） | 中 |
| 需要改动的代码量 | — | 大 | 中 |
| 与现有"wg-quick 语义"一致性 | 一致 | 需重新定义 | 一致 |

---

## 5. 关于"风险与收益不匹配"的判断

这个直觉基本正确，但要分清两条不同的轴：

- **收益轴**：真实收益只剩下一个 —— 去掉启动时的授权提示。
  之前被算作服务化附加收益的两项，现在都不成立了：
  "隧道在 GUI 关闭后保持"已由 `disconnect_on_quit=off` 直接提供（§6.2.1），
  "崩溃自动重启"在 macOS 上本来就有（`KeepAlive.SuccessfulExit=false`）。
- **成本轴**：服务化的成本集中在三处，而且都是"必须配套"的：
  受保护安装路径 + 特权自更新链路 + 完整卸载逻辑。
  只做"注册成服务"而不做这三件，等于给用户装了一个提权后门。

换句话说：**收益是体验层的一次性改善，成本是安全模型和发布流程的结构性改动。**
这个交换在"只为了少点一次 UAC"的前提下不成立；
但如果同时需要"GUI 不在也要保持隧道"（例如把软件当常驻 VPN 用），
它就开始划算了 —— 这取决于产品定位，而不是技术偏好。

---

## 6. 推荐方案

**默认保持 A；把"免提示"做成一个可选的、加固过的 D 模式；C 暂不做。**

理由：

1. A 的语义和现有实现（含 macOS 那段刻意的 `RunAtLoad=false` 设计）是自洽的，
   不值得为了提示弹框整体推翻。
2. D 能拿到"零提示"这一唯一刚需收益，同时不引入常驻 root 进程，
   生命周期语义与现在完全一致。
3. C 的唯一额外收益（GUI 关闭后保持隧道）应当单独作为产品决策讨论，
   而不是被打包进"为了去掉 UAC"这个动机里 —— 混在一起谈容易出现
   "为了省一次点击，顺手增加了常驻权限面"的错误取舍。

### 6.1 若要实施 D，实施顺序（每一步都独立可回退）

1. **先决条件：受保护安装路径。** helper 安装到 `Program Files` /
   `/usr/local/lib` 等管理员属主路径，并校验哈希/签名后才注册自启条目。
   这一步不做，后面都不要做。
2. **Windows**：注册 `schtasks /rl highest /sc onlogon` 或"触发时运行"的任务，
   GUI 改用 `schtasks /run` 拉起（不再走 `Start-Process -Verb RunAs`）。
   保留失败回退到 UAC 路径的逻辑。
3. **Linux**：写 `/etc/polkit-1/rules.d/` 规则允许本用户免密启动 helper，
   或 systemd unit + `systemctl start`；同时加 capability 收敛。
4. **macOS**：保持 LaunchDaemon `RunAtLoad=false`，把安装动作从"每次 no socket"
   改成"安装状态持久化 + `launchctl kickstart`"；理想情况换成 SMJobBless。
5. **设置项**：`免授权启动（安装一次授权条目）` 开关 + 完整卸载按钮。
6. **日志**：helper 的日志目录按会话解析（服务/系统会话下写到
   `ProgramData` / `/var/log` / 用户目录三者按运行身份选择）。
7. **升级**：更新流程里检测 helper 版本不一致 → 重新提权安装一次（现有
   `ForceReinstall` 路径已经具备雏形，见 `internal/elevate/spawn.go` 的
   `Args.ForceReinstall`）。

### 6.2 无论是否实施，都建议先做的加固 —— 处置结果

| 项 | 状态 | 落点 |
| --- | --- | --- |
| "退出 GUI 是否断开隧道"做成显式设置项 | **已实施** | `settings.disconnect_on_quit` |
| 开机自启时等 DNS 就绪再连 | **已实施** | `internal/tunnel/resolve.go` |
| Linux 侧给 systemd unit 加 capability 收敛 | **未实施：前提不成立**（见下） | — |

#### 6.2.1 已实施：`disconnect_on_quit`

原来的行为是隐式的："Quit = 断开所有隧道 + 让 helper 退出"（`internal/gui/gui.go`
的 `doShutdown`）。现在它是一个设置项，默认 **开**，即保持既有行为不变
（`nil` 也按开处理，所以升级不会让任何人在背后多出一条常驻隧道）：

- **开**：`doShutdown` 依次发 `Disconnect` 与 `Shutdown` RPC，隧道随 GUI 一起结束。
- **关**：只断开 GUI 自己的 IPC 连接，**不发** `Shutdown` —— 发它会让 helper 走
  `cleanup()`，而 `cleanup()` 正是逐条断开隧道的那个函数，与"保留会话"的意图正好相反。
  helper 随后按它本来就有的规则存活：*有活动隧道时永不自动退出*（
  `internal/helper/helper.go` 的 `armShutdownTimer`，即 wg-quick 的 monitor 语义），
  下次启动 GUI 会重新连上同一个 helper —— 连带省掉一次授权提示。

也就是说 §4 表格里"隧道可在 GUI 关闭后保持"这一格，**从服务化独有的收益变成了
A 方案内的一个开关**。这削弱了 C 方案剩下的独立理由（见 §5 的修订）。

#### 6.2.2 已实施：endpoint 解析跨过开机 DNS 未就绪

v2.2.0 开机三次连接失败的机制是明确的：`internal/tunnel/engine.go` 在 Connect
开头对每个 peer endpoint 做**一次** `LookupHost`，失败即 `return error`（与 wg-quick
一致：对端不可达就不建隧道）。这条规则在稳态下是对的，但开机后最初几秒 DNS 服务
还没起来，同一个问题隔一会儿问会有完全不同的答案 —— 而自动化引擎在启动窗口里
反复评估，每次都撞上同一个一次性失败。

`internal/tunnel/resolve.go`（新增）把这一步改成**有界重试**：

- 总预算 15s（`endpointResolveBudget`），单次尝试 5s，退避 0.8s 起翻倍；
- **只对瞬时失败重试**：`*net.DNSError`（含 `IsNotFound`，因为"域名真不存在"与
  "解析器还连不上"在这里无法区分，而把后者当致命错误的代价是每次开机都连不上）、
  超时、`net.Error.Timeout()`；配置类错误不重试，用户不必为一个写错的地址干等；
- 预算用尽时返回**最后一次的解析错误**，调用方的报错信息仍然指向真实原因；
- 第 2 次及以后成功会打一条 Info（"endpoint resolved after retry"），
  这样"连接慢"和"连接失败"在日志里能区分开。

#### 6.2.3 未实施：Linux capability 收敛（前提不成立）

这条原建议默认 Linux 侧存在一个 systemd unit。**实际上没有**：Linux 的 helper 由
`pkexec` 直接 spawn 成一个普通进程（`internal/elevate/spawn_linux.go`），
系统里不存在任何 unit 文件，因此没有 `CapabilityBoundingSet=` / `AmbientCapabilities=`
可以挂靠。要在 Linux 上把"完整 root"真正收敛成"仅 `CAP_NET_ADMIN` + `CAP_NET_RAW`"，
必须**先引入按需的 systemd unit**，也就是把 D 方案在 Linux 那一半先做出来 ——
这是架构级改动，而当前开发机是 Windows，改完无法验证特权路径（TUN、路由、
iptables 全部跑不到）。**在没有 Linux 验证环境之前不动它**：无法验证的特权代码
比"权限面偏大"危险得多。

若将来实施 D，Linux 侧 unit 的关键收敛项（按需启动、不常驻、能力收敛）应当是：

```ini
[Unit]
Description=WireGuide Plus privileged helper (on-demand only)

[Service]
Type=exec
# 占位符由安装器按实际路径替换；参数与 pkexec 路径保持同一套
# （--socket / --uid / --data-dir / --logs-dir）
ExecStart=/usr/local/lib/wireguide-plus/wireguide-plus --helper \
    --socket=/run/wireguide-plus/helper.sock --uid=<user> --data-dir=<dir>
# 把 root 缩到"只够配置网卡、路由、nftables 与 ICMP"
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=yes
# 生命周期交给 helper 自己的 startupGrace / shutdownGrace，systemd 不重拉
Restart=no

# 故意没有 [Install] / WantedBy：只由 GUI 用 systemctl start 触发，
# 不 enable —— 一旦 enable 就变成"开机即起的常驻 root"，正是 §3.2 要避免的
```

前提仍是 §3.1：`ExecStart` 指向的路径必须管理员属主且普通用户不可写。

---

## 7. 结论

- 三平台的实际状况不同，但**都不是"没有这个问题"**：Windows 每次弹，
  macOS/Linux 首次弹并且 helper 每次退出后重弹。
- 纯收益（零提示）可以用 D 方案拿到，成本是一次提权安装 + 受保护路径 +
  特权更新链路。
- 常驻服务（C）的额外收益（GUI 关闭后保持隧道）属于产品定位问题，
  应当单独决策，且必须连同卸载残留、日志、多用户语义一起设计。
- **不推荐**把 GUI 一起做成服务（三平台都需要会话代理，复杂度不可接受）。
- 现状（A）在没有明确"常驻 VPN"需求前应保持默认。
- "退出 GUI 是否断开隧道"与"开机等 DNS 就绪"两项加固**已落地**
  （§6.2.1 / §6.2.2）；Linux 的 capability 收敛因"当前并不存在 systemd unit"
  而搁置，并已写明将来实施 D 时的 unit 收敛模板（§6.2.3）。
