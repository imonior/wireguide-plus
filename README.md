# WireGuide Plus

**A multi-tunnel WireGuard client with network-aware automation.**

WireGuide Plus is a deeply **fixed and enhanced** fork of the open-source project
[`korjwl1/wireguide`](https://github.com/korjwl1/wireguide). Its two core capabilities:

- **Multi-tunnel concurrency** — multiple WireGuard tunnels establish at the same time
  and run independently without interfering with each other;
- **Conditional auto-connect** — rules based on Wi-Fi SSID, time of day, system startup,
  etc. automatically connect the right tunnel (for example, tunnel A on the office
  Wi-Fi, tunnel B at home).

**English** | [简体中文](README.zh.md) | [繁體中文](README.zh-TW.md) | [한국어](README.ko.md) | [日本語](README.ja.md)

> **Fully supported on Windows 10 / 11 (x64, x86 32-bit and ARM64) and macOS
> (Apple Silicon / arm64)** — macOS has been thoroughly verified on real Apple
> Silicon hardware. Linux (x64, arm64) builds are available as **experimental
> previews** — CI-built but not yet tested on real hardware (see
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
- **AmneziaWG (AWG) tunnels** — import and connect AmneziaWG (obfuscated WireGuard) configs. AWG is auto-detected from the Jc/Jmin/Jmax/S1-S4/H1-H4 obfuscation parameters in the config and each such tunnel shows an "AmneziaWG" badge; support can be switched off under Settings → Advanced.
- **Tunnel editor: field view & script hooks** — alongside the raw conf text, a per-field form (interface / peer groups) edits the config, and the PreUp / PostUp / PreDown / PostDown script hooks can be managed (pick a file, create a blank, edit code, clear).
- **Per-tunnel physical egress binding** — with Pin Interface enabled, each tunnel's encrypted traffic can be pinned to a specific physical NIC (Windows / Linux / macOS); if the bound NIC disappears, a dialog offers wait / auto-switch / manual pick.
- **Settings export & import** — bundles tunnels, scripts and `config.json` (logs excluded) into a single archive for migrating to another machine.

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
6. **AmneziaWG (AWG) protocol support** — a new protocol backend (amneziawg-go) for AmneziaWG tunnels, the obfuscated WireGuard fork that resists DPI. Configs are auto-detected, tunnels are badged in the UI, and an opt-out switch lives in Settings → Advanced.

## Automation rules

Automation is configured **per tunnel** (open any tunnel's `…` menu → `Automation…`). Every tunnel has its own independent set of rules, so "connect A on the office Wi-Fi, disconnect B at the office, connect B at home" all coexist without conflict.

The editor's live network panel describes the **currently selected tunnel's** environment,
one row per category: all detected physical hardware interfaces (with `in use` / `not in use`),
Wi-Fi SSID, gateway MAC, gateway IP, and subnet. It also reports which conditions in this
tunnel match those values. Virtual adapters are excluded. When Wi-Fi is not connected, the
SSID row says `Wi-Fi not connected`. The panel and rule guidance can be collapsed, and the
whole editor scrolls so large rule sets remain usable.

`Pin Interface` is now supported on all desktop platforms: Windows binds the tunnel's UDP
sockets to the chosen NIC via `IP_UNICAST_IF`, Linux installs `ip route … dev <iface>`
bypass routes, and macOS scopes bypass routes with `-ifscope`. Choosing a physical egress
is an operating-system routing policy (not a WireGuard protocol setting), and when the
bound NIC disappears a dialog lets you keep the binding and wait, switch automatically, or
pick the NIC manually.

### Rule logic

- **Rules inside one tunnel form ONE ordered list**, evaluated top-to-bottom —
  **rule position is priority**. Connect and disconnect rules can be freely
  interleaved and drag-reordered across the whole list.
- **First match wins (AND inside a rule)**: every condition on a single rule
  must hold for the rule to fire, and only the **first matching rule** in the
  list executes its action. Rules ranked behind a matched rule are
  "deprioritised" and do not execute, so you never both "disconnect on X SSID"
  AND "connect on X SSID" for the same tunnel.
- **Default State fallback**: when **no rule matches** the current network, the
  tunnel converges to its **Default State** (connected or disconnected — chosen
  at the top of the Automation editor). A tunnel with neither rules nor a
  Default State is never touched by automation; setting only a Default State
  (zero rules) makes the tunnel always converge to it.
- Rule editing shows **live match indicators**: while the Automation editor is
  open, every condition shows whether it currently matches the live network, the
  first effective rule is highlighted as "in use", and a top bar shows the
  resulting decision for the tunnel. Indicator changes are refreshed immediately
  on every edit (≈ 250 ms debounced IPC to the evaluation engine that actually
  enforces the rules in the background helper, so UI and real behaviour are
  always identical). The same engine is reachable from the command line via
  `wireguideplus automation` — useful for headless checks.

### Condition types

| Condition | What it matches | Use case |
| --- | --- | --- |
| **SSID** | Case-sensitive, byte-exact full match against the Wi-Fi network's SSID name (spaces and special characters all count — per the 802.11 definition). | "On `Office 5 GHz` connect Work-VPN." |
| **Subnet** | Whether the current local IP falls inside a given CIDR (e.g. `192.168.178.0/24`). | Home routers that use a predictable LAN range, not tied to SSID. |
| **Gateway MAC** | The MAC address of the current default gateway (router) — identifies a specific network even when SSIDs or subnets are generic. | "Never auto-connect on the café router." |
| **Gateway IP** | The default gateway IP address of the current physical network. | Detect a specific home / office router when SSIDs are too generic. |
| **Interface** | The name of the physical network adapter the system is routing through. The dropdown lists every physical adapter on the machine, including currently-disconnected ones, so you can pre-write rules for a laptop dock / USB dongle that isn't plugged in yet. | "Only connect the work VPN when I'm on the docked Ethernet adapter." |
| **On wired network (Ethernet)** | True whenever the system's upstream routing is through a wired (non-wireless) adapter. No SSID needed — pure wired vs wireless decision. | "At the desk (cable) always connect; on Wi-Fi don't." |
| **Time window** | A day-of-week set + a start/end time range (local clock). | "From Monday to Friday 09:00–18:00 the office tunnel stays up." |

A single rule can combine any of the above: e.g. **SSID = Office AND Time = Mo–Fr 09–18** is one rule with two AND conditions. Each tunnel supports any number of AND rules under both the disconnect and connect groups.

## Platform support

| Platform | Status |
| --- | --- |
| Windows 10 / 11 (x64, x86 32-bit, ARM64) | ✅ Fully supported (multi-tunnel concurrency + SSID auto-connect, incl. AmneziaWG) |
| macOS (Apple Silicon / arm64) | ✅ Fully supported — thoroughly verified on real Apple Silicon hardware |
| Linux (x64, arm64) | 🚧 Experimental — CI-built but not yet tested on real hardware |
| Android / iOS | ❌ **Not supported** (cannot run tunnels concurrently, nor auto-connect different tunnels by Wi-Fi SSID) |

### Why no mobile edition?

The project's core capabilities are **multi-tunnel concurrency** and **rule-based
auto-connect (e.g. by Wi-Fi SSID)**. On Android / iOS the system kernel and permissions
prevent WireGuard implementations from **running multiple tunnels at once** or
**switching tunnels automatically by Wi-Fi SSID** — neither core goal is achievable on
mobile. This project therefore **explicitly does not target mobile devices**; mobile
users should use the official WireGuard app with its On-Demand capability for
single-tunnel needs.

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
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintun-driver-dll)). Releases provide ready-made
portable zips (`wireguideplus-amd64-portable.zip` /
`wireguideplus-x86-portable.zip` / `wireguideplus-arm64-portable.zip`), each already
containing the exe **and** the matching driver DLL — download one zip, extract, and run.
Releases no longer attach bare DLLs (use the portable zip or the installer above). Without
the matching driver DLL, tunnels cannot be created.

## Code Signing

Every published Windows **installer** is Authenticode-signed, which lets you verify
**integrity** — the binary has not been modified since it was signed. Signed
binaries also trigger fewer Windows SmartScreen warnings on first run.

A signature proves *who signed the file*, not on its own *how it was built*.
Provenance (which pipeline produced the installer, approval workflow, account
security and reproducibility) is documented separately in
[SIGNING-POLICY.md](SIGNING-POLICY.md), together with the SHA-256 checksums that
ship with every release.

Note: only the installers are signed; the portable zips contain the unsigned build
output.

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
| Tunnel scripts | `%APPDATA%\wireguideplus\scripts\` |
| Logs | `%APPDATA%\wireguideplus\logs\` |

## Uninstall

Uninstall via **Control Panel → Programs and Features → WireGuide Plus**, or run the
uninstaller in the install directory.

## Acknowledgements

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — upstream open-source project
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
