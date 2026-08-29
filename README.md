# WireGuide Plus

**A multi-tunnel, automation-first WireGuard client for Windows**

WireGuide Plus is a deeply **fixed and enhanced** fork of the open-source project
[`korjwl1/wireguide`](https://github.com/korjwl1/wireguide). Its two core capabilities:

- **Multi-tunnel concurrency** — multiple WireGuard tunnels establish at the same time
  and run independently without interfering with each other;
- **Conditional auto-connect** — rules based on Wi-Fi SSID, time of day, system startup,
  etc. automatically connect the right tunnel (for example, tunnel A on the office
  Wi-Fi, tunnel B at home).

**English** | [简体中文](README.zh.md) | [繁體中文](README.zh-TW.md) | [한국어](README.ko.md) | [日本語](README.ja.md)

> Currently **fully supported on Windows 10 / 11 (x64 and x86 32-bit)**. The macOS and
> Linux enhanced editions are under development — use the upstream version meanwhile (see
> [Platform support](#platform-support)). **Android / iOS are not supported.**

## Features

- **Multi-tunnel concurrency** — unlike upstream, which allows only one tunnel at a time,
  this edition runs multiple tunnels in parallel — ideal for reaching an intranet and an
  exit network simultaneously.
- **Conditional auto-connect** — triggers on Wi-Fi SSID / time of day / system startup
  connect or disconnect a tunnel automatically; rules support priority and mutual
  exclusion.
- **Auto-reconnect** — tunnels recover automatically after unexpected drops, with the
  connection state visible in real time.
- **Start on login** — a setting that launches WireGuide Plus after login and connects
  according to your rules (combined with "Start minimized" the window starts tucked away).
- **Start minimized** — a setting that starts the app minimized to the **taskbar** on
  Windows (the taskbar button stays visible, so the main window can always be reopened) or
  to the system tray on macOS/Linux.
- **Tray connection notifications** — 10 seconds after startup (once the elevation prompt
  is settled) the current connection state is shown; network changes (Wi-Fi switch,
  cable unplug, network loss, ...) that alter tunnel state also show a 10-second-delayed
  bubble with the stable, latest state. The bubble has an action menu (open main window /
  disconnect), can be dismissed manually, or auto-closes after a configurable dwell time
  (default 10 s, adjustable in Settings).
- **Tunnel management** — import / export `.conf`, connection history, quick toggles.

## Fixes & enhancements over upstream wireguide

### Fixes

1. **Wi-Fi fully supported as the network egress (the most critical fix)** — upstream
   only sends traffic out of the **wired** interface on Windows, so the egress was
   unusable on Wi-Fi. This edition fixes the default-egress-interface selection: on
   Wi-Fi, traffic correctly leaves through the wireless adapter.
2. **GUI theme bug** — fixed broken rendering when switching between dark / light themes.
3. **Standardized Windows version resources** — fixed the blank version info in the exe
   properties page (now generated with `goversioninfo`).
4. **Stability fixes** — deduplicated update-check scheduling, more accurate physical
   adapter detection, and more (see [CHANGELOG](CHANGELOG.en.md)).

### Enhancements

1. **SSID dropdown** — auto-connect rules can pick from **every Wi-Fi SSID the system has
   saved** via a dropdown instead of typing, avoiding typos.
2. **Update checks through a proxy** — configure an HTTP(S) proxy before checking for
   updates, solving update failures caused by GitHub being unreachable / rate-limited.
3. **Multi-language UI** — 简体中文 / English / 日本語 / 한국어 / 繁體中文.
4. **System integration** — start on login, start minimized (taskbar on Windows / tray on
   macOS & Linux), tray connection notifications (shown 10 s after startup / after network
   changes alter connection state, default dwell 10 s, adjustable).
5. **Window title & interaction polish** and more.

## Platform support

| Platform | Status |
| --- | --- |
| Windows 10 / 11 (x64, x86 32-bit) | ✅ Fully supported (multi-tunnel concurrency + SSID auto-connect) |
| macOS | 🚧 Enhanced edition under development — use [wireguide](https://github.com/korjwl1/wireguide) or [WireTunnels](https://github.com/FMDigitech/WireTunnels) |
| Linux | 🚧 Enhanced edition under development — use [wireguide](https://github.com/korjwl1/wireguide) |
| Android / iOS | ❌ **Not supported** (cannot run tunnels concurrently, nor auto-connect different tunnels by Wi-Fi SSID) |

> **macOS alternative: [WireTunnels](https://github.com/FMDigitech/WireTunnels)** — a
> native macOS menu-bar WireGuard client with multi-tunnel support, monitoring and
> control, complementing upstream `wireguide`.

### Why no mobile edition?

The project's core capabilities are **multi-tunnel concurrency** and **rule-based
auto-connect (e.g. by Wi-Fi SSID)**. On Android / iOS the system kernel and permissions
prevent WireGuard implementations from **running multiple tunnels at once** or
**switching tunnels automatically by Wi-Fi SSID** — neither core goal is achievable on
mobile. This project therefore **explicitly does not target mobile devices**; mobile
users should use the official WireGuard app with its On-Demand capability for
single-tunnel needs.

## Roadmap

- **v2.0 (planned)**: run as a **Windows system service** — auto-connect without a user
  login, with a more stable network stack and better privilege control.

## Download & Install

Each release publishes two kinds of Windows builds separately: **installers** and a
**portable build**.

**Installers (recommended)**

- Windows x64 installer: `wireguideplus-amd64-installer.exe`
- Windows x86 (32-bit) installer: `wireguideplus-x86-installer.exe`
- Windows ARM64 installer: `wireguideplus-arm64-installer.exe`

Installer names embed the architecture (`wireguideplus-<arch>-installer.exe`, arch ∈
`x86` / `amd64` / `arm64`), and the executable installed inside carries it too
(`wireguideplus-<arch>.exe` — also visible in the file's Properties → Details). The
64-bit installer installs to `C:\Program Files\WireGuide Plus` by default; the 32-bit
installer to `C:\Program Files (x86)\WireGuide Plus` (`C:\Program Files\WireGuide Plus`
on 32-bit systems). The install directory can be changed during installation. A Start
Menu shortcut (including an "Uninstall WireGuide Plus" entry, default on, optional) and a
desktop shortcut (always created) are registered. Installers bundle everything needed —
no extra files to download.

**Portable build (no installation)**

- `wireguideplus-amd64.exe` **+ `wintun-amd64.dll`** (or **+ `wintun-x86.dll`** for the
  32-bit exe, **+ `wintun-arm64.dll`** for the ARM64 exe) — download **both** files for
  the same architecture and place them in the same folder, then run the exe.

The portable binary is **not standalone**: it needs the matching-architecture driver
DLL next to it (used to create WireGuard tunnels). The exe loads the DLL by its
arch-qualified name (`wintun-amd64.dll` / `wintun-x86.dll` / `wintun-arm64.dll`) — no
rename is ever needed:

| exe | matching driver DLL |
| --- | --- |
| `wireguideplus-amd64.exe` (64-bit) | `wintun-amd64.dll` |
| `wireguideplus-x86.exe` (32-bit) | `wintun-x86.dll` |
| `wireguideplus-arm64.exe` (ARM64) | `wintun-arm64.dll` |

The driver DLLs come from `wintun-0.14.1.zip` (see
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintundll)). Releases also provide
ready-made portable zips (`wireguideplus-amd64-portable.zip` /
`wireguideplus-x86-portable.zip` / `wireguideplus-arm64-portable.zip`), each already
containing the exe **and** the matching driver DLL — download one zip, extract, and run.
A bare `wintun-amd64.dll` / `wintun-x86.dll` / `wintun-arm64.dll` is attached too for
manual pairing (just drop it next to the exe, no renaming). Without the matching driver
DLL, tunnels cannot be created.

## Code Signing

Every published Windows **installer** is Authenticode-signed, which lets you verify
both **integrity** (the binary has not been tampered with in transit or on disk)
and **origin** (it was built and released by this project). Signed binaries also
trigger fewer Windows SmartScreen warnings on first run.

Note: only the installers are signed; the portable zips contain the unsigned build
output. For the full signing policy (scope, approval workflow, account security and
reproducibility) see [SIGNING-POLICY.md](SIGNING-POLICY.md).

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## Build & Development

Build environment requirements, dev / release build commands (including the x86 +
amd64 + arm64 multi-architecture build), NSIS installer notes, version resources and
the release workflow are documented in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md). Publishing is as
simple as pushing a version tag — the GitHub Actions pipeline builds, signs and publishes
the release automatically (see [docs/release.md](docs/release.md)).

## Data & Logs

| Item | Location |
| --- | --- |
| Settings / history | `%APPDATA%\wireguideplus\` (`config.json`, `history.json`) |
| Tunnel configs | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| Logs | `%APPDATA%\wireguideplus\logs\` |

## Uninstall

Uninstall via **Control Panel → Programs and Features → WireGuide Plus**, or run the
uninstaller in the install directory.

## Acknowledgements

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — upstream open-source project
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
