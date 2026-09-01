# 开发说明（DEVELOPMENT.md）

本文档面向**开发者**，覆盖本地环境、构建、打包与发布流程。终端用户安装与使用请见 [README](../README.md)。

## 1. 项目结构

```
wireguide-plus/
├── frontend/                 # Wails v3 + Svelte 前端（Vite 构建）
├── internal/                 # Go 后端
│   ├── gui/                  # 窗口、系统托盘、连接状况通知（Windows）
│   ├── storage/              # 配置、历史、设置（settings.go 含 GUI 设置项）
│   └── wifi/                 # Wi-Fi SSID 识别与自动连接规则
├── build/
│   ├── config.yml            # Wails 构建配置（描述、输出名等）
│   ├── windows/              # Windows 专属构建产物配置
│   │   ├── Taskfile.yml      # Windows 构建/打包任务（含双架构 build:all）
│   │   ├── nsis/             # NSIS 安装脚本（project.nsi）
│   │   ├── info.json / versioninfo.json / icon.ico
│   │   └── msix/             # MSIX 打包模板
│   └── linux/nfpm/           # Linux 打包（nfpm）
├── Taskfile.yml              # 顶层任务（go-task）
└── .github/workflows/        # CI（ci.yml）、发布（release.yml）、notes 刷新（fix-release-notes.yml）
```

## 2. 环境依赖

| 依赖 | 用途 | 安装 |
| --- | --- | --- |
| Go ≥ 1.24 | 后端编译 | 官方安装包 |
| Node.js ≥ 20（含 npm） | 前端构建 | 官方安装包 |
| wails3（v3.0.0-alpha.74） | 构建骨架 / webview2 引导 | `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.74` |
| task（go-task） | 任务编排 | `go install github.com/go-task/task/v3/cmd/task@latest` |
| goversioninfo | 生成 exe 版本资源（syso） | `go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0` |
| NSIS（makensis） | 打包 Windows 安装程序 | 见下方「NSIS」节 |

> 注意：`$GOPATH/bin`（`go install` 默认输出目录）需要加入系统 `PATH`，否则 `wails3`、`task`、`goversioninfo` 命令找不到。

**本地工具目录**：非 Go 模块的依赖工具统一放在 `e:\Projects\tools\`（工作区根，不属于代码仓库）：

| 工具 | 位置 | 说明 |
| --- | --- | --- |
| NSIS 便携版 | `e:\Projects\tools\nsis-full\` | 其中 `makensis.exe` 不在 PATH，构建前需手动加入，或直接用全路径调用 |
| wintun 源包 | `e:\Projects\tools\wintun-0.14.1.zip` | wintun 官方下载失败时的本地缓存（手动方案见 4.2） |
| wintun（amd64） | `e:\Projects\tools\wintun-amd64.dll` | 备用驱动副本 |
| 7-Zip 精简版 | `e:\Projects\tools\7zr.exe` | 解压 7z 等格式；如需解 NSIS 安装包请安装完整 7-Zip |

> `makensis` 调用示例：`& "e:\Projects\tools\nsis-full\makensis.exe" -DARG_WAILS_X86_BINARY=... project.nsi`（或先执行 `$env:PATH = "e:\Projects\tools\nsis-full;" + $env:PATH`）。

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
| `bin/wireguideplus-x86-installer.exe` | 32 位安装包（含 32 位程序 + 32 位 wintun-x86.dll） |
| `bin/wireguideplus-amd64-installer.exe` | 64 位安装包（含 64 位程序 + 64 位 wintun-amd64.dll） |
| `bin/wireguideplus-arm64-installer.exe` | ARM64 安装包（含 ARM64 程序 + ARM64 wintun-arm64.dll） |
| `bin/wintun-x86.dll` / `bin/wintun-amd64.dll` / `bin/wintun-arm64.dll` | 各架构的 wintun 驱动（文件名即架构，程序据此加载） |

> 运行程序统一命名 `wireguideplus-<arch>.exe`，安装包 `wireguideplus-<arch>-installer.exe`；
> 文件版本资源（exe 属性 → 详细信息）中的「说明 / 内部名称 / 原始文件名」同样内嵌
> 架构信息（见 §6），由 `tools/genverinfo` 按构建架构动态生成。

### 4.1 前端构建

`task` 的构建链会自动执行前端构建；手动执行：

```bash
cd frontend
$env:PRODUCTION = "true"   # PowerShell；Linux/macOS: export PRODUCTION=true
npm run build
```

**已知问题**：如果 `node_modules` 不完整，`task` 会触发 `npm ci --force`，而 npm 较新版本对批量删除会弹出交互确认（`[safe-delete]`），在 CI/非交互环境下会卡住。解决：手动 `npm install`（非 `ci`）后重新运行构建；或直接手动执行上面四步。

### 4.2 wintun driver DLL

`task windows:build*` 会尝试从 `https://www.wintun.net/builds/wintun-0.14.1.zip` 下载 wintun 驱动。该站点在部分网络环境无法访问，构建会失败。

程序按架构加载对应文件名的驱动 DLL（见 `third_party/wintun/wintun.go` 的 `wintunDLLName`）：64 位程序加载 `wintun-amd64.dll`，32 位程序加载 `wintun-x86.dll`，ARM64 程序加载 `wintun-arm64.dll`。因此 `bin` 目录下必须是按架构命名的文件，而不是笼统的 `wintun.dll`。

**手动方案**（与任务逻辑一致）：

```powershell
# 下载 wintun-0.14.1.zip 后（本地缓存：e:\Projects\tools\wintun-0.14.1.zip）：
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory("e:\Projects\tools\wintun-0.14.1.zip", "$env:TEMP\wintun-extract")
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
task bump:version 1.1.2    # 可选传参；不传则按 VERSION 文件同步
```

`task bump:version`（`tools/bumpversion`）会一键重写 `build/config.yml`、`build/windows/info.json`、`build/windows/versioninfo.json`、`build/windows/wails.exe.manifest`、`build/windows/nsis/wails_tools.nsh`（`INFO_PRODUCTVERSION`）、`build/windows/msix/*.xml`、`build/linux/nfpm/nfpm.yaml` 及 macOS 两份 Info.plist；Go 程序与版本资源则在构建时自动从 `VERSION` 注入，无需手动修改任何代码或模板。

## 7. 发布流程（推送 tag 即发布）

GitHub Actions 工作流 `.github/workflows/release.yml` 会在推送 `v*` 标签时自动构建 **Windows（x86 + amd64 + arm64）、macOS（arm64）、Linux（amd64 + arm64）** 产物、验证/签名、生成 Release Notes（git-cliff）并创建 GitHub Release，同时更新 Homebrew Cask。

完整步骤（打 tag、签名密钥配置、可选 SignPath 校验）见 [docs/release.md](release.md)。

## 8. 测试

```bash
task test              # Go 单元测试
```
