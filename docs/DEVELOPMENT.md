# 开发说明（DEVELOPMENT.md）

本文档面向**开发者**，覆盖本地环境、构建、打包与发布流程。终端用户安装与使用请见 [README](../README.md)。

## 1. 项目结构

```
wireguide-plus/
├── frontend/                 # Wails v3 + Svelte 前端（Vite 构建）
├── internal/                 # Go 后端
│   ├── app/                  # Wails bindings：前端 ↔ helper 客户端 ↔ 本地存储
│   ├── autostart/            # 开机自启注册与移除（InstallAutostart / RemoveAutostart）
│   ├── cli/                  # `wireguideplus ctl …` 命令行（直接驱动运行中的 helper）
│   ├── config/               # WireGuard .conf 解析与校验
│   ├── diag/                 # 诊断：地址冲突、DNS 泄漏、路由与接口归属、ping
│   ├── domain/               # 核心值对象（无外部依赖）
│   ├── elevate/              # 跨平台提权启动子进程
│   ├── firewall/             # 各系统 DNS(53) 放行与防火墙处理
│   ├── gui/                  # 主窗口、系统托盘、状态气泡（macOS 原生 / Win32 / Svelte 窗口）
│   ├── helper/               # 提权 helper 进程（隧道增删、策略下发、RPC handlers）
│   ├── ipc/                  # GUI ↔ helper 的 JSON-RPC 2.0 协议与传输（unix socket / named pipe）
│   ├── logging/              # 按天轮转的 slog 文件日志与保留期清理
│   ├── network/              # 各系统 IP、路由与 DNS 配置
│   ├── policy/               # 纯函数策略层（隧道级准入判定）
│   ├── reconnect/            # 自动重连与死链检测
│   ├── storage/              # 配置、历史、设置（settings.go 含 GUI 设置项）
│   ├── sysexec/              # 子进程启动调整：Detach（脱离启动终端）与 HideWindow
│   ├── tunnel/               # WireGuard / AmneziaWG 隧道生命周期
│   ├── update/               # 版本检查与自更新（Ed25519 校验）
│   ├── version/              # 注入构建版本（源自仓库根 VERSION）
│   └── wifi/                 # Wi-Fi SSID 识别与自动连接规则
├── third_party/wintun/       # 按架构动态加载 wintun 驱动 DLL
├── build/
│   ├── config.yml            # Wails 构建配置（描述、输出名、版本号）
│   ├── darwin/               # macOS：两份 Info.plist、Taskfile、图标
│   ├── linux/                # Linux：Taskfile、nfpm 元数据与脚本、.desktop、appimage
│   ├── windows/              # Windows：Taskfile、nsis/、msix/、版本资源与 manifest
│   └── tools/genicon/        # 由 appicon.png 生成多分辨率 .ico（Taskfile 的 dir: build，故引用为 ./tools/genicon）
├── tools/                    # bumpversion / checkrelease / genverinfo / updatesign
├── releases/                 # 历史安装包：git 跟踪的回滚备份，勿删
├── scripts/                  # 集成/回归测试脚本、git hooks（scripts/setup-hooks.sh）、no-AI 扫描
├── specs/                    # 早期 spec / 计划文档
├── Taskfile.yml              # 顶层任务（go-task）
└── .github/workflows/        # CI（ci.yml）、发布（release.yml）、notes 刷新（fix-release-notes.yml）、扫描（no-ai-scan.yml）
```

## 2. 环境依赖

| 依赖 | 用途 | 安装 |
| --- | --- | --- |
| Go 1.25.12 | 后端编译 | 官方安装包 |
| Node.js 20.19.2（含 npm） | 前端构建 | 官方安装包 |
| wails3（v3.0.0-alpha.74） | 构建骨架 / webview2 引导 | `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.74` |
| task（go-task v3.45.4） | 任务编排 | `go install github.com/go-task/task/v3/cmd/task@v3.45.4` |
| goversioninfo | 生成 exe 版本资源（syso） | `go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0` |
| NSIS（makensis） | 打包 Windows 安装程序 | 见下方「NSIS」节 |

> 版本号的**唯一权威**是 [CONTRIBUTING.md](../CONTRIBUTING.md) 的「CI pin」表与 `go.mod`：上表只是本地装环境的速查，两者冲突时以那两个为准（Node 尤其注意：Vite 8 要求 `^20.19.0 || >=22.12.0`，笼统的「Node 20」跑不起来）。

> 注意：`$GOPATH/bin`（`go install` 默认输出目录）需要加入系统 `PATH`，否则 `wails3`、`task`、`goversioninfo` 命令找不到。

**本地工具目录**：非 Go 模块的依赖工具建议统一放在仓库之外的一个目录（下文用 `$env:TOOLS` 指代，路径自选），不要提交进仓库：

| 工具 | 位置 | 说明 |
| --- | --- | --- |
| NSIS 便携版 | `$env:TOOLS\nsis-full\` | 其中 `makensis.exe` 默认不在 PATH，构建前需手动加入，或直接用全路径调用 |
| wintun 源包 | `$env:TOOLS\wintun-0.14.1.zip` | wintun 官方下载失败时的本地缓存（手动方案见 4.2） |
| wintun（amd64） | `$env:TOOLS\wintun-amd64.dll` | 备用驱动副本 |
| 7-Zip 精简版 | `$env:TOOLS\7zr.exe` | 解压 7z 等格式；如需解 NSIS 安装包请安装完整 7-Zip |

> `makensis` 调用示例：`& "$env:TOOLS\nsis-full\makensis.exe" -DARG_WAILS_X86_BINARY=... project.nsi`（或先执行 `$env:PATH = "$env:TOOLS\nsis-full;" + $env:PATH`）。

## 3. 本地开发

```bash
# 开发构建（当前平台）
task build
./bin/wireguideplus

# 前端热更新开发（前后端分离调试）
cd frontend && npm run dev
```

## 4. Windows 构建（x86 32 位 + amd64 64 位 + ARM64）

**发布构建：一次产出 x86、amd64、arm64 三种程序及各自安装包**（在 Windows 上执行）：

```bash
task windows:build:all
```

该任务依次执行 `windows:package ARCH=386`、`ARCH=amd64`、`ARCH=arm64`，并在各架构之间自动刷新 `bin/wintun-x86.dll` / `bin/wintun-amd64.dll` / `bin/wintun-arm64.dll`（wireguard-go 依赖的驱动 DLL，必须与程序架构匹配），确保每个安装包内置正确的 DLL。程序按架构选择文件名（见 `third_party/wintun` 的 `wintunDLLName`），64 位程序只认 `wintun-amd64.dll`，32 位程序只认 `wintun-x86.dll`，ARM64 程序只认 `wintun-arm64.dll`。

按需构建单个架构：

```bash
task windows:build                # amd64
task windows:build ARCH=386       # x86 32 位
task windows:build ARCH=arm64     # arm64（版本资源 syso 走 `-arm` 分支，已实测验证）
```

产物输出到 `bin/`：

| 产物 | 说明 |
| --- | --- |
| `bin/wireguideplus-x86.exe` | 32 位运行程序（文件名内嵌架构） |
| `bin/wireguideplus-amd64.exe` | 64 位运行程序 |
| `bin/wireguideplus-arm64.exe` | ARM64 运行程序 |
| `bin/wireguideplus-<version>-x86-installer.exe` | 32 位安装包（含 32 位程序 + 32 位 wintun-x86.dll） |
| `bin/wireguideplus-<version>-amd64-installer.exe` | 64 位安装包（含 64 位程序 + 64 位 wintun-amd64.dll） |
| `bin/wireguideplus-<version>-arm64-installer.exe` | ARM64 安装包（含 ARM64 程序 + ARM64 wintun-arm64.dll） |
| `bin/wintun-x86.dll` / `bin/wintun-amd64.dll` / `bin/wintun-arm64.dll` | 各架构的 wintun 驱动（文件名即架构，程序据此加载） |

> 运行程序统一命名 `wireguideplus-<arch>.exe`，安装包 `wireguideplus-<version>-<arch>-installer.exe`；
> 文件版本资源（exe 属性 → 详细信息）中的「说明 / 内部名称 / 原始文件名」同样内嵌
> 架构信息（见 §6），由 `tools/genverinfo` 按构建架构动态生成。

### 4.1 前端构建

`task` 的构建链会自动执行前端构建；手动执行：

```bash
cd frontend
$env:PRODUCTION = "true"   # PowerShell；Linux/macOS: export PRODUCTION=true
npm run build
```

**为什么是 `npm install` 而不是 `npm ci`**：npm ≥ 11.4 的实验性批量删除确认（`SAFE_DELETE_BULK_CONFIRM_REQUIRED`）会在 `npm ci` 清理 `node_modules` 时弹出交互确认，且 `safe-delete=never` 无法关掉（npm 的 bug），CI/非交互环境会直接卡死。因此 `install:frontend:deps` 用的是 `npm install`（根 `Taskfile.yml` 为 `npm install`，`build/Taskfile.yml` 为 `npm install --force`）：`package-lock.json` 已提交且不变，增量安装在这里是幂等的，不会引入版本漂移。

### 4.2 wintun driver DLL

`task windows:build*` 会尝试从 `https://www.wintun.net/builds/wintun-0.14.1.zip` 下载 wintun 驱动。该站点在部分网络环境无法访问，构建会失败。

程序按架构加载对应文件名的驱动 DLL（见 `third_party/wintun/wintun.go` 的 `wintunDLLName`）：64 位程序加载 `wintun-amd64.dll`，32 位程序加载 `wintun-x86.dll`，ARM64 程序加载 `wintun-arm64.dll`。因此 `bin` 目录下必须是按架构命名的文件，而不是笼统的 `wintun.dll`。

**手动方案**（与任务逻辑一致）：

```powershell
# 先把下载的 wintun-0.14.1.zip 放到自己的路径，再改这一行：
$zip = "$env:USERPROFILE\Downloads\wintun-0.14.1.zip"
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory($zip, "$env:TEMP\wintun-extract")
# 构建 386 时：
Copy-Item "$env:TEMP\wintun-extract\wintun\bin\x86\wintun.dll" bin\wintun-x86.dll
# 构建 amd64 时：
Copy-Item "$env:TEMP\wintun-extract\wintun\bin\amd64\wintun.dll" bin\wintun-amd64.dll
```

`vendor:wintun` 任务检测到对应架构的 `bin/wintun-<arch>.dll` 已存在时会跳过下载，因此手动放置后即可继续。

## 5. NSIS 安装包

安装脚本：`build/windows/nsis/project.nsi`（wails 生成的 `wails_tools.nsh` 为公共宏，**不要手动修改**；其中的 `INFO_PRODUCTVERSION` 由 `task bump:version` 自动维护，见 §6）。

当前安装包行为：

- **默认安装目录**：`C:\Program Files\WireGuide Plus`（amd64 / arm64 安装包，以及 32 位系统上的 x86 安装包）或 `C:\Program Files (x86)\WireGuide Plus`（64 位系统上的 x86 32 位安装包）；安装过程中用户可在目录选择页修改。
- **开始菜单快捷方式**（含「卸载 WireGuide Plus」入口）：默认创建，可在「快捷方式选项」页取消勾选。卸载入口的图标复用运行程序图标，与程序一致。
- **桌面快捷方式**：始终创建，用户不可选择拒绝。
- 卸载程序图标与运行程序使用同一图标源（`build/windows/icon.ico`，通过 `MUI_UNICON` 设置）。

手动打包（绕过 task，用于快速验证 .nsi 语法）：

```bash
cd build/windows/nsis
# 32 位安装包（PRODUCT_EXECUTABLE 决定安装后的程序文件名）
makensis -DPRODUCT_EXECUTABLE=wireguideplus-x86.exe -DARG_WAILS_X86_BINARY=..\..\..\bin\wireguideplus-x86.exe project.nsi
# 64 位安装包
makensis -DPRODUCT_EXECUTABLE=wireguideplus-amd64.exe -DARG_WAILS_AMD64_BINARY=..\..\..\bin\wireguideplus-amd64.exe project.nsi
# ARM64 安装包
makensis -DPRODUCT_EXECUTABLE=wireguideplus-arm64.exe -DARG_WAILS_ARM64_BINARY=..\..\..\bin\wireguideplus-arm64.exe project.nsi
```

## 6. 版本资源（exe 属性）

exe 的「详细信息」标签页版本信息由 `build/windows/versioninfo.json.tmpl` 渲染而来（模板变量：`FileDescription`、`InternalName`、`OriginalFilename`，均内嵌架构后缀，如 `WireGuide Plus (amd64) - ...`、`wireguideplus-amd64.exe`）。`generate:syso` 任务先用 `tools/genverinfo` 渲染出临时 `versioninfo.gen.json`（`-version` 取自根 `VERSION` 文件，见下方单一源），再由 `goversioninfo` 生成 `wails_windows_<arch>.syso` 并链接进可执行文件。

**版本号单一源**：所有版本号（Go 程序 `-ldflags`、Windows 版本资源、NSIS / MSIX、Linux nfpm、macOS Info.plist）统一来自仓库根 `VERSION` 文件。发布前只需：

```bash
task bump:version NEW_VERSION=1.1.2   # 传参须用变量式；不传则按 VERSION 文件同步
```

`NEW_VERSION=1.1.2` 是**变量**写法。位置参数（`task bump:version 1.1.2`）会被 go-task 当成第二个任务名，直接报 `task: Task "1.1.2" does not exist`（已实测，退出码 200）。

`task bump:version`（`tools/bumpversion`）会一键重写 `build/config.yml`、`build/windows/info.json`、`build/windows/versioninfo.json`、`build/windows/wails.exe.manifest`、`build/windows/nsis/wails_tools.nsh`（`INFO_PRODUCTVERSION`）、`build/windows/msix/*.xml`、`build/linux/nfpm/nfpm.yaml` 及 macOS 两份 Info.plist；Go 程序与版本资源则在构建时自动从 `VERSION` 注入，无需手动修改任何代码或模板。

## 7. 发布流程（推送 tag 即发布）

GitHub Actions 工作流 `.github/workflows/release.yml` 会在推送 `v*` 标签时自动构建 **Windows（x86 + amd64 + arm64）、macOS（arm64）、Linux（amd64 + arm64）** 产物、验证/签名、生成 Release Notes（取自英文默认的 `CHANGELOG.md`）并创建 GitHub Release，同时更新 Homebrew Cask。

完整步骤（打 tag、签名密钥配置、可选 SignPath 校验）见 [docs/release.md](release.md)。

## 8. 测试

```bash
task check             # 本地质量门禁：go vet ./internal/... + go test -race ./internal/...
```

`task check` 刻意只覆盖 `./internal/...`：根包 `embed` 了 `frontend/dist`，前端未构建时 `go vet ./...` / `go test ./...` 会直接失败。CI 在三个 job 里各自先构建前端再跑全量 `./...`（Linux 与 macOS 带 `-race`，Windows 不带），因此全量测试与 `-race` 结论以 CI 为准。

发布元数据一致性另有一条独立检查（五语言 i18n 键集、五份 CHANGELOG 段落、`VERSION` 与各打包元数据文件）：

```bash
go run ./tools/checkrelease
```

`task bump:version` 在改写元数据前会自动执行它，`ci.yml` 亦单独调用一次。
