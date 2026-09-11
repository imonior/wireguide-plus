# WireGuide Plus 策略原则基线（Policy Principles）

> 状态：**权威基线**。本文档是后续所有策略类功能（路由 / DNS / 保护 / 自动化）开发的原则依据；
> 任何新功能的设计与实现都应对照这 33 条逐条检查。修订需在 PR 中显式说明违反或精化了哪一条。

## 实现落点（代码地图）

原则本身是规范，下面是每条原则在当前代码中的落点——没有落点的标注"待实现"。

| 原则 | 落点 |
| --- | --- |
| 1/2 标准配置可移植、私有策略不进 .conf | `internal/storage.TunnelMeta`（`traffic_protect` / `domains` / `use_as_default_dns` / `dns_resolve_path` / `system_dns` 存于 `.meta.json` 旁车）；序列化路径 `internal/config` 不写入这些字段 |
| 3-6 编辑器结构、Scripts 布局、Import/Export、Bind Interface | `frontend/src/App.svelte` + `ConfigEditor` / `FieldsEditor` / `ScriptEditor` / `EgressBinding`；Scripts 保持 `Pre-Up \| Post-Up` / `Pre-Down \| Post-Down` 四格布局 |
| 8/9 Traffic Protect（隧道级） | `TunnelMeta.TrafficProtect` + `frontend/src/lib/TunnelPolicies.svelte`（文案统一为 Traffic Protect）；**全局 Kill Switch 开关已从 Settings/IPC/CLI/helper 全部移除**（`Firewall.SetKillSwitch` 等一并删除） |
| 10/11/12 DNS 路由策略与 IP 路由分离 | `TunnelMeta.Domains`（Domains Through Tunnel）、`UseAsDefaultDNS`、`SystemDNS`；分析见下条 |
| 13-16 前缀集合比较、五态判定、IPv4/IPv6 独立 | `internal/policy/prefix.go`（`ParsePrefixes` / `Classify` / `ConflictIdentical|Contained|Partial|FullTunnel|None`） |
| 17-21 LPM、禁止连接顺序当优先级、IDENTICAL 才需要仲裁 | `internal/policy/route.go`（`routeSeverity`：contained/full_tunnel=info，identical/partial=blocking）；Route Priority 未做 UI |
| 22-25 Conflict Policy（保存 / 手动连接 / 自动化） | 保存：`app.UpdateConfig` → `logPolicyWarnings`（允许 + 记录）；手动连接：`App.svelte doConnect` + `ConflictWarning.svelte`（阻止 + Connect Anyway + Edit Tunnel）；自动化：`helper.policyBlockFor` → `event.policy_blocked`（阻止 + 通知 + 日志，不弹窗） |
| 26 永不自动改写 AllowedIPs | 分析器只读；冲突一律交给用户（Edit Tunnel），代码路径无任何写回 |
| 27/28/29 Protection / DNS 独立分析 | `internal/policy/protection.go`、`internal/policy/dns.go`（Default DNS 多选检测） |
| 30 Analyzer 纯函数 | `internal/policy` 全部入参为 `TunnelView` 快照，无 IO、无副作用；测试 `policy_test.go` 含顺序无关性验证 |
| 31/32 自动化 Desired State 不变，策略校验位于动作与操作之间 | `internal/helper/automation_rules.go`（Desired State → Reconcile 不动）；`automationConnect` 开头调用 `policyBlockFor` |

## 定位

WireGuide Plus 不是单纯的 WireGuard 配置编辑器，而是在标准 WireGuard/AWG 之上叠加策略层：

```text
WireGuard / AWG
       │
       ▼
标准 Tunnel Config
       │
       ├── Routing Policy
       ├── DNS Policy
       ├── Traffic Protection
       └── Automation
```

总体架构：

```text
                    WireGuide Plus
                           │
             ┌─────────────┴─────────────┐
             │                           │
        Tunnel Policy               Automation
             │                           │
    ┌────────┼────────┐                  │
    │        │        │                  ▼
 Routing    DNS   Protection        Desired State
    │        │        │                  │
    ▼        ▼        ▼                  ▼
Route      DNS      Protection       Policy Validation
Analyzer   Analyzer Analyzer              │
    │        │        │                   ▼
    └────────┼────────┴────────────── Allow / Block
             │
             ▼
       Tunnel Operation
```

## 一、配置与策略分离

**1. 标准 WireGuard/AWG 配置保持可移植。**
`.conf` 仍然是标准配置，可用 wg-quick / 官方客户端直接加载。

**2. WireGuide Plus 私有策略永不写入 `.conf`。**
Traffic Protect、Domains Through Tunnel、Use as Default DNS、未来的 Route Priority
都存放在自己的元数据中，不污染标准配置。

## 二、Tunnel Editor 结构

**3. Conf / Fields 保持兄弟视图（toggle 切换）。**

**4. Import / Export 保持原样。**

**5. Bind Interface 保留。**

**6. Scripts 布局固定：**

```text
[ Pre-Up ]    [ Post-Up ]

[ Pre-Down ]  [ Post-Down ]
```

不重新设计编辑器。

## 三、Traffic Protect

**7. UI 术语只用 Traffic Protect，永远不引入 Kill Switch 产品概念。**

**8. Traffic Protect 是 tunnel-scoped（隧道级）流量保护**，保护范围是该隧道的
AllowedIPs：

```text
Tunnel A  AllowedIPs = 10.10.0.0/16, Traffic Protect = ON
    → 10.10.0.0/16 必须经过 A
    → A 不可用时，这些流量不能从其他出口泄漏（只断这部分，不整台断网）
```

**9. Full Tunnel 只是 Traffic Protect 的最大范围**（AllowedIPs = 0.0.0.0/0、::/0
时保护范围恰好覆盖默认流量），不单独造概念。

## 四、DNS 策略

**10. Domains Through Tunnel 是 DNS 路由策略**（哪个域名用哪个 resolver/策略）。

**11. Use as Default DNS 是独立的 DNS 策略**，与 Domains Through Tunnel 互不等价。

**12. DNS 路由与 IP 路由是两个关注点，不能混同：**

```text
DNS Server = 10.10.0.53  ≠  DNS query 走 Tunnel A

真实链路：
Domain → DNS Policy → 选择 Resolver → 系统 Route Lookup → 选择出口
```

`Domains Through Tunnel` 管前者（域名→resolver），`AllowedIPs` 管后者（IP→隧道）。

**33. DNS Resolve Path（DNS 解析通道）是按隧道的 DNS 路径策略**，与 11 的
"Use as Default DNS"（换 resolver 地址）不等价——详见第九节。

**13～14（见下）DNS 域名冲突与 Default DNS 冲突必须独立检测：**

- **Domain overlap**：两个隧道声明同一个域名（`corp.example.com` ×2）。
- **Domain containment**：`example.com` 与 `internal.example.com`——涉及 domain
  specificity（更具体的声明应优先，需向用户呈现这一语义）。
- **Multiple Default DNS**：两个隧道同时勾选 Use as Default DNS 是
  **Default DNS Conflict**，不是普通配置项。

因此 DNS 冲突归 **DNS Policy Analyzer**，不混进 AllowedIPs 检查。

## 五、AllowedIPs 分析

**13. AllowedIPs 必须解析成独立前缀**，不能拿字符串比较：

```text
AllowedIPs → Parser → []netip.Prefix → Canonical Prefix Set
```

**14. 每个前缀都必须两两比较**（隧道 A prefixes × 隧道 B prefixes）。

**15. IPv4 与 IPv6 独立分析。**

**16. Route Conflict 至少五种状态：**

| 状态 | 例 |
|---|---|
| NONE（无重叠） | `10.0.0.0/8` vs `192.168.0.0/16` |
| CONTAINED（包含） | `10.0.0.0/8` vs `10.10.0.0/16` |
| IDENTICAL（相同） | `10.10.0.0/16` vs `10.10.0.0/16` |
| PARTIAL（部分重叠） | `10.10.0.0/15` vs `10.11.0.0/16`（非包含非相同） |
| FULL_TUNNEL | `0.0.0.0/0` vs `10.0.0.0/8` → 提示 "Full Tunnel covers this route" |

注意：CONTAINED 在 LPM 下无歧义（见 17），告警语义与 PARTIAL/IDENTICAL/FULL_TUNNEL
区分对待——运行时冲突检查对"新前缀严格位于既有更宽路由内部"的情况只提示不阻塞。

## 六、路由优先级

**17. 正常路由遵循最长前缀匹配（LPM）**：`10.10.0.0/16` 永远胜过 `10.0.0.0/8`。

**18. 永远不用连接顺序充当路由优先级。**

**19. 永远不用隧道列表顺序充当路由优先级。**

**20. 相同前缀（IDENTICAL）是真正的平局/冲突**——LPM 没有赢家，必须显式解决。

**21. Route Priority 是未来的显式策略，不是当前批次的 UI 功能**；跨 macOS /
Windows / Linux 一致映射到底层 route/interface 行为的方案需单独研究，
不要假设"设置一个 metric 就解决"。

## 七、Conflict Policy（冲突处置策略）

**22. 保存时有冲突 → 允许 + 警告**（配置存在冲突 ≠ 配置非法）。

**23. 手动连接时有冲突 → 默认阻止**，给出 Cancel / Connect Anyway / Edit Tunnel。

**24. 手动覆盖 → Connect Anyway**（用户显式决定）。

**25. 自动化时有冲突 → 默认阻止自动连接**，通过 Tray Notification + Log 告知，
绝不自动弹窗等待。

## 八、硬规则

**26. 永远不自动改写用户的 AllowedIPs。**
分析器只负责"告诉用户问题 → 用户决定 → Edit Tunnel"，不猜用户意图。

**27. Route Conflict 与 Protection Conflict 是两个独立的冲突类别**：

```text
A: AllowedIPs = 10.0.0.0/8,  Traffic Protect = ON
B: AllowedIPs = 10.10.0.0/16, Traffic Protect = ON

路由层：B 的 /16 经 LPM 有明确结果。
保护层：A 声称保护 10/8，B 声称保护 10.10/16 → Protection Policy Conflict。
```

**28. DNS 域名冲突独立于路由冲突。**

**29. 多个 Default DNS 选择需要冲突检测。**

## 九、分析器与自动化的边界

**30. 策略分析器必须是 Pure / Deterministic / 无副作用：**

```text
AnalyzeRoutes(tunnels) → ConflictReport
```

输入相同则输出相同。分析器内部不连接/断开隧道、不改配置、不动系统路由、
不弹 UI、不发通知——这些全部由上层处理。

**31. 自动化的 Desired State → Reconciliation 架构保持不变。**

**32. 新的策略校验位于"期望动作"与"实际隧道操作"之间：**

```text
Automation → 我要 Connect A
    → Policy Validation
        ├── Route Analyzer
        ├── DNS Analyzer
        └── Protection Analyzer
            → Allow / Block
```

不把 AllowedIPs、DNS、Protection 的判断塞进 Automation Evaluator 本身。

## 十、DNS Resolve Path（按隧道的 DNS 路径策略）

**33. 系统 DNS 的出口路径只能由"按隧道"的开关决定，不能由全局开关决定；
且必须按"路径"实现，而不是换 resolver 地址。**

多通道同时连接时，"强制 DNS 只走 VPN"这类全局开关无法回答"走哪一个通道"，
因此全局 DNS Protection 开关已删除，改为"设置总开关 + 每个隧道一个
`DNSResolvePath` 开关（DNS 解析通道）"：

```text
总开关（Settings，默认关）：关 = 功能整体不存在（不显示、不强制、不报冲突）
总开关开 + 隧道勾选后（该隧道连接期间）：
  本机任何进程发起的 DNS 查询 → 必须经此隧道接口发出
  其他接口的 53 端口被丢弃（无论系统/软件配置的 DNS 服务器是哪一个）
  隧道未连接时：DNS 按系统配置的方式解析

它不是：
  "把该隧道的 DNS 设为系统唯一 DNS"（那是 Use as Default DNS，换的是地址）
  而是：
  "约束解析的出口路径"（换的是路径）
```

检测与处置（冲突按"配置合法、运行互斥"分层）：

- **保存时**：`app.SetTunnelPolicies` → analyzer 报告（info 级）→
  `TunnelPolicies.svelte` 内联提醒，**允许保存**（用户可能分时使用）。
- **连接时**（运行时真正互斥的时刻）：
  - 手动连接：`helper.awaitDNSResolvePathClear` 检测到已有连接中的隧道
    持有该角色时**挂起连接**并广播 `event.dns_path_conflict`，弹窗让用户选：
    "关闭该功能并连接"（调 `Tunnel.ResolveDNSPathConflict(disable)`）或
    "停止连接"（`cancel`）。用户把占用方断开后也会自动放行（轮询检测）。
    超时（120s）或无 GUI（CLI）→ 直接失败，绝不静默。
  - 自动化：没有人能应答弹窗，因此不等待——`automationConnect` 跳过并
    `event.policy_blocked` 通知。
- 冲突类型（`internal/policy/dns.go`，配置级；严重级别随"是否都连接"浮动）：
  - `dns_resolve_path_multiple`：两个隧道都声明 DNS 解析通道。
    双方都连接 → blocking；至多一方连接 → info（仅提醒）。
  - `dns_resolve_path_vs_default_dns`：一个声明路径、另一个声明 resolver
    地址 → 任一方连接即 blocking（两个角色对"DNS 去哪"说法不一致）。
  - 同一隧道同时持有 DNS Resolve Path 与 Use as Default DNS 是自洽的，
    不算冲突。
- 强制手段：该隧道连接成功后 `helper.applyPostConnectFirewall` 按隧道调用
  `firewall.EnableDNSProtection(iface, cfg.Interface.DNS)`；隧道断开时
  `clearDNSPathIfOwner` 才撤销（拥有者归属由 `helper.dnsPathOwner` 记录，
  避免后连接的隧道被动撤掉前者的规则）。
- **总开关 OFF 必须即时生效，包括运行时状态**：设置保存检测到
  `dns_resolve_path` ON→OFF 时，GUI 调 `Tunnel.ClearDNSPathEnforcement`，
  helper 立即 `DisableDNSProtection` 并清掉 `dnsPathOwner`——防火墙规则是
  运行时状态，不会随配置隐藏而消失；不做这一步，"关 = 功能整体不存在"
  就只剩 UI 语义。正在停靠的连接随总开关关闭自动放行（轮询读到 master 关
  → blockers 为空），且因 claim 被 master 门控而不会安装任何规则。
