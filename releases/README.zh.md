# 本地历史版本归档

本目录存放各版本发布资产的**本地备份**（installer、portable zip、dmg、
deb、tar.gz、SHA256SUMS、签名等），供回滚、对比、离线分发使用。它不是
构建产物的一部分。

> 英文版见 [README.md](README.md)，内容以此中文版为准（如不一致）。

## 规则

- 二进制文件**不进 git 仓库**（体积大），由 `.gitignore` 中的
  `releases/*` 忽略。
- `README.md` / `README.zh.md` 这两个说明文件被 git 跟踪，用于固定目录
  存在并说明用途。
- 常规清理命令不会动本目录：
  - `git clean -fd`：**不会**删除（被忽略的文件默认保留）
  - `task build` / `wails3` 构建：只写 `bin/`，不碰本目录
  - CI（GitHub Actions）：跑在独立 runner 上，不接触本地文件
- ⚠️ 唯一会删除本目录的命令是 `git clean -fdx`（`-x` 连忽略文件一起删），
  **切勿随意运行**。确需删除时先 `git clean -ndx releases/` 预览。

## 备份文件清单

> 每次发布后，将 release 资产复制到本目录并登记。示例：

| 版本 | 平台 | 文件 | SHA256 |
|------|------|------|--------|
| v1.1.0 | Windows amd64 | wireguideplus-1.1.0-amd64-installer.exe | 在此登记 |
| v1.1.0 | Windows amd64 | wireguideplus-1.1.0-amd64-portable.zip | 在此登记 |
| ... | ... | ... | ... |

## 操作命令

```powershell
# 复制历史版本资产到归档（示例）
Copy-Item "D:\releases-backup\*" .\releases\

# 登记哈希（追加到清单）
Get-FileHash .\releases\*.exe -Algorithm SHA256 | Format-Table
```
