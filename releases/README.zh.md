# 本地历史版本归档

本目录存放**手动备份**的本地构建产物（installer、portable zip、单文件 exe、
wintun 驱动等），供回滚、对比、离线分发使用。它不是构建产物的一部分，也
**不会**随每次发布自动更新。

> 英文版见 [README.md](README.md)，内容以此中文版为准（如不一致）。

## 备份方式（手动）

- **不是**每次发布后自动复制。每次发布的历史资产会永久保留在 GitHub
  Releases 上，不会丢失。
- 这里只备份**本地测试构建**的文件：本地构建 → 测试 OK → 认为有必要留档时，
  手动复制进本目录。
- 手动上传到 GitHub 后无法自动生成哈希，因此复制文件后需在下方清单中
  **手工登记** SHA256。

## 规则

- 新丢进本目录的文件都被 `.gitignore` 的 `releases/*` 忽略，因此不会
  意外堆积二进制。
- 下方清单里那四个 v1.1.0 文件是**有意保留的例外**：它们命中了同一条忽略
  规则却仍被 git 跟踪，说明当初是用 `git add -f` 强制加入的。它们是最后一次
  本地端到端实测构建的回滚备份——不要删除，也不要因为它们占体积而"清理仓库"。
  （同一版本在 GitHub Release 上也有，但资产名还是改名前的 `WireGuide-*`，
  两边并不能 1:1 互换。）
- `README.md` / `README.zh.md` 同样被跟踪，用于固定目录存在并说明用途。
- 关于这些被跟踪的备份，什么会删掉、什么删不掉：
  - `git clean -fd` 与 `git clean -fdx` 都**碰不到**它们——clean 只删未跟踪
    文件。但 `-x` 会连本目录里*新*放入的忽略文件一起删，所以执行前仍要先
    `git clean -ndx releases/` 预览。
  - `git rm`、checkout 到不含它们的树、以及改写历史**会**销毁它们；届时这些
    字节只存在于当初添加它们的提交里，浅克隆和归档导出里则完全没有。
  - `task build` / `wails3` 构建只写 `bin/`，不碰本目录。
  - CI（GitHub Actions）跑在独立 runner 上，不接触本地文件。

## 备份文件清单

当前归档为 v1.1.0 本地测试构建产物（文件名未内嵌版本号，以登记日期区分）：

| 版本 | 平台 | 文件 | SHA256 |
|------|------|------|--------|
| v1.1.0 | Windows amd64 | wireguideplus-amd64-installer.exe | 72957D9839707D5037AC82A8BA62AA41118C229BF10B380541D87BFA628EFFF1 |
| v1.1.0 | Windows amd64 | wireguideplus-amd64-portable.zip | 4947E758419720858889CE0487E99A57DF0214132E58111B28BE498A3AA31C2F |
| v1.1.0 | Windows amd64 | wireguideplus-amd64.exe | A6C34FEF72B170F8F41CF27B469ACCC3546801415E201AD5F6A5A9EAFC99F31C |
| v1.1.0 | Windows amd64 | wintun-amd64.dll | E5DA8447DC2C320EDC0FC52FA01885C103DE8C118481F683643CACC3220DAFCE |

登记日期：2026-08-30

## 新增备份时的操作命令

```powershell
# 复制本地构建产物到本目录（示例）
Copy-Item "D:\build-out\*" .\releases\

# 计算哈希，将结果手工登记到上方清单
Get-FileHash .\releases\*.exe -Algorithm SHA256 | Format-Table
```
