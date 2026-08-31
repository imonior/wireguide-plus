# Linux 集成测试脚本

本目录包含 WireGuide Plus 在 Linux 上运行的手动/按需集成测试脚本。
它们**不属于** CI 或常规构建流程，只在开发验证时手动运行（需要一台可以破坏性测试的 Linux 机器）。

## 公共用法

大多数测试脚本用法一致（`network_test_recover.sh` 除外，见下）：

```
sudo bash scripts/<测试脚本>.sh <wireguide 二进制路径> <VPN 配置文件> <隔离测试目录>
```

- 参数 1：`wireguide` 可执行文件（先 `wails3 task linux:build` 或使用构建产物）
- 参数 2：一个可用的 WireGuard 配置文件（用于建立测试隧道）
- 参数 3：隔离的测试工作目录（测试会在其中放 runtime/config/data/helper 等）
- 参数 4（可选）：`uid_num`，默认取当前 `id -u`

测试脚本都会在退出时调用 `network_test_recover.sh` 做网络恢复，避免破坏宿主机网络。

## 各脚本用途

| 脚本 | 用途 | 特殊要求 |
|---|---|---|
| `automation_integration_test.sh` | 自动化审计链路集成测试：验证自动重连/自动化配置下的行为，通过 `generate_204` 探测连通性 | 需 root + 可访问外网 |
| `cli_feature_matrix_test.sh` | CLI 功能矩阵 + 恢复测试：遍历 `ctl` 子命令（killswitch、dns-protection 等），验证各开关与故障恢复 | 需 root |
| `crash_recovery_integration_test.sh` | 崩溃恢复集成测试：构造崩溃场景（systemd 单元被杀），验证自动重连与 throw 路由清理 | 需 root + systemd |
| `firewall_integration_test.sh` | 防火墙集成测试：验证 killswitch / DNS 保护的 nftables 规则在 split-tunnel 下是否正确拦截 | 需 root + nftables |
| `full_tunnel_integration_test.sh` | **破坏性**全隧道测试：`AllowedIPs=0.0.0.0/0,::/0` 接管全部流量，验证公网 IP/DNS 均走隧道；失败时走紧急恢复 | 需 root，**会中断宿主机网络**，建议远程会话防锁死 |
| `healthcheck_integration_test.sh` | 健康检查集成测试：验证对端不可达时健康检查触发重连，以及 nftables 恢复规则 | 需 root + nftables |
| `resource_stability_test.sh` | 资源稳定性测试：循环连接/断开（默认 30 轮）并高频调用状态查询（默认 100 次），监控内存泄漏 | 可通过 `WIREGUIDE_RESOURCE_CYCLES`、`WIREGUIDE_RESOURCE_STATUS_CALLS`、`WIREGUIDE_RESOURCE_GOGC` 等环境变量调整 |
| `network_test_recover.sh` | **紧急恢复脚本**（不是独立测试）：被其他测试脚本及独立 systemd 瞬时单元调用，负责杀掉测试 helper、删除残留 wg-* 接口/套接字、清理解析规则、恢复 `/etc/resolv.conf` | 用法：`sudo bash network_test_recover.sh <resolv.conf 备份> <helper pidfile> <恢复日志路径>` |

## 注意事项

- 这些脚本都是破坏性/侵入式的：会修改系统路由、nftables 规则、`/etc/resolv.conf`，请只在测试机或虚拟机中运行。
- `full_tunnel_integration_test.sh` 会临时接管整机流量，若测试失败由 `network_test_recover.sh` 兜底恢复，但仍建议在可远程恢复的环境中执行。
- 环境变量：`XDG_RUNTIME_DIR`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME` 会被脚本重定向到隔离目录，避免污染真实用户配置。
