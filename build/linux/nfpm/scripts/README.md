# nfpm 打包脚本说明

本目录包含 WireGuide Plus 在 Linux 打包（deb/rpm/Arch）时使用的 nfpm 生命周期脚本。
这些脚本由 `../nfpm.yaml` 的 `scripts:` 段引用，会在安装/卸载的各个阶段执行。

## 文件用途

| 文件 | 阶段 | 作用 | 当前状态 |
|---|---|---|---|
| `preinstall.sh` | 安装前 | 包内容写入前的准备（如检查依赖、创建用户/目录） | 空壳，未启用 |
| `postinstall.sh` | 安装后 | 更新桌面数据库与 MIME 数据库，让应用出现在应用菜单、自定义 URL scheme 生效 | **已启用** |
| `preremove.sh` | 卸载前 | 卸载前的清理准备（如停止服务、保存配置） | 空壳，未启用 |
| `postremove.sh` | 卸载后 | 卸载后的清理（如删除缓存、移除用户数据外的残留） | 空壳，未启用 |

## 如何启用

1. 编辑 `../nfpm.yaml` 的 `scripts:` 段，取消对应行的注释并指向脚本路径：

   ```yaml
   scripts:
     preinstall: "./build/linux/nfpm/scripts/preinstall.sh"
     postinstall: "./build/linux/nfpm/scripts/postinstall.sh"
     preremove: "./build/linux/nfpm/scripts/preremove.sh"
     postremove: "./build/linux/nfpm/scripts/postremove.sh"
   ```

2. 在对应脚本中写入实际逻辑（保持 `#!/bin/sh` 或 `#!/bin/bash` 头不变）。
   nfpm 官方文档：https://nfpm.goreleaser.com/configuration/#scripts

## 注意事项

- 脚本需在安装/卸载环境下可用，避免依赖目标系统未必存在的工具（如有需要请先用 `command -v` 判断）。
- 目前只有 `postinstall.sh` 被 `nfpm.yaml` 引用（第 63 行），其余三个为预留空壳。
- 改动脚本后请重新打包验证：`wails3 task linux:package`（或对应的 `linux:deb`/`linux:rpm`/`linux:aur` 任务）。
