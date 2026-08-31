# Changelog

All notable changes to WireGuide Plus will be documented in this file.

> 简体中文: [CHANGELOG.md](CHANGELOG.md) · 繁體中文: [CHANGELOG.zh-TW.md](CHANGELOG.zh-TW.md) · 日本語: [CHANGELOG.ja.md](CHANGELOG.ja.md) · 한국어: [CHANGELOG.ko.md](CHANGELOG.ko.md)

## [1.2.5] - 2026-09-01

This release rebuilds the DNS leak test around public-resolver cross-checking: alongside the resolvers from the host's own network configuration, the test now also probes well-known public DNS servers for cross-verification, tags every resolver by its source (Local / VPN / Public), and lets you refresh or customize the public list. A new "Browser test" button opens browserleaks.com for browser-level DNS and WebRTC leak checks. It also fixes Windows connection popups that could freeze, and ships a new app icon.

### ✨ New features

- **Public DNS cross-check** — the test now also probes well-known public resolvers (Google, Cloudflare, OpenDNS, Quad9, Alibaba, Tencent DNSPod, 114DNS, Baidu, AdGuard, NextDNS, Comodo, plus common IPv6 addresses) so answers can be cross-verified against traffic leaving the tunnel. A public resolver answering only means it is reachable — not a leak.
- **Refresh from the network** — "Fetch from network" pulls the currently most reliable resolvers from public-dns.info (up to 30, 10-second timeout) and caches the last successful fetch so the list stays usable offline.
- **Custom public resolver list** — add, edit or remove entries (IP or hostname) freely; the list is persisted in settings. Clearing it restores the built-in defaults — public probing always stays on.
- **Resolver source tags** — system resolvers are now tagged by the interface they come from: physical adapters (WLAN / Ethernet) are "Local", tunnel interfaces are "VPN", the rest are "Public". Local resolvers are listed first, with the source interface name (Windows enumerates per-adapter DNS, Linux parses resolvectl output).
- **Browser test** — a new "Browser test" button opens browserleaks.com in your default browser for browser-level DNS and WebRTC leak detection (probe data is sent to the third-party site).

### 🐛 Fixes

- **Windows connection popup freeze** — the popup's message loop was not pinned to the OS thread that created it; after the goroutine migrated threads, click / close / timer messages stopped arriving and the popup looked frozen. The thread is now locked for the popup's lifetime, so it can be dismissed and auto-closes normally.
- **Popup text drawing hardening** — text drawing now uses `UTF16FromString` with error handling, so an invalid UTF-16 string can no longer crash the popup.

### 🛠 Internal

- The CLI `dnsleak` command is enhanced in sync: each resolver row shows a `vpn / local / public` tag and its status, and the customized public list from settings is used.
- Leak detection corrected: only a physical (non-VPN) resolver answering is a leak; VPN resolvers are tagged as VPN; public resolvers answering show "OK" rather than a leak.
- New probe-plan and parsing tests for the DNS leak module; bindings regenerated.
- New app icon across platforms; build tasks simplified.

## [1.1.10] - 2026-08-31

This release fixes the three UI issues reported on 1.1.9 and improves the settings interaction: the DNS leak test page no longer constrains its width and marks the host's own DNS servers; log level filtering is now exact; notification duration and proxy selection save and display correctly again, and custom mirror / local proxy inputs remember the last saved address.

### 🐛 Fixes

- **DNS leak test width** — removed the 640px max-width so the page fills the window like the History and Routes pages.
- **Host DNS marker** — every probed resolver comes from the host's own DNS configuration (manual or DHCP); each row now shows a "System" chip so they are easy to tell apart from VPN-provided DNS.
- **Log level filtering** — clicking DEBUG / INFO / WARN / ERROR now shows only records of that level (previously "level and above", which looked like a no-op whenever a level had no records).
- **Notification duration** — the dropdown now uses the same dynamic option pattern as log retention / history retention / language, so changes persist and are shown when reopening settings.
- **Proxy mode display** — the proxy select no longer sticks on "Direct" after reopening settings (Svelte cannot track which fields a function body reads, so `value={fn()}` was only evaluated once). It now recomputes reactively and shows the saved mirror / manual mode.
- **Proxy address memory** — switching back to "custom mirror" or a local proxy restores the last saved address in the input (e.g. a previously saved mirror prefix); empty history shows a blank input with a hint.

## [1.1.9] - 2026-08-31

This release fixes in-app updates failing right after a successful download: the updater deleted the temp installer before launching it, so Windows reported the file as missing and fell back to the release page.

### 🐛 Fixes

- **In-app update could not install** — `runUpdateNative` removed the downloaded installer with `os.Remove(path)` *before* calling `Install`, while the Windows install path execs that very file (`fork/exec …wireguide-update-*.exe: The system cannot find the file specified`). A 100% download was therefore always followed by a launch failure. The temp file is now released only after the installer has been launched; on Windows the exe is usually locked while the installer runs, so removal may fail — harmless, since %TEMP% is cleaned up by the OS.
- **One manual upgrade required** — the updater on 1.1.7 / 1.1.8 has the same flaw, so upgrading in-app from those versions still hits it. Please install 1.1.9 manually once (Settings → Updates → open the release page); from then on in-app updates work normally.

## [1.1.8] - 2026-08-31

This release aligns the automation editor's guidance with the rule semantics and hardens old-format rule handling: rules run top to bottom with the first match winning, conditions of the same action are OR-ed, and "otherwise" acts as the fallback placed last, usually with the opposite action. Legacy rules that lack a condition type no longer trigger spurious reloads.

### ✨ Improvements

- **Automation semantics guidance** — the editor hint and the "otherwise" row description now spell out the model: "otherwise" fires when no rule above it matched, keep it last as the fallback, and its action is usually the opposite of the rules above (updated in all five languages). The evaluation logic itself is unchanged: ordered first-match, `none_match` matches unconditionally — exactly the behaviour you described.

### 🐛 Fixes

- **Old-format rules no longer trigger spurious reloads** — the disk-vs-local comparison now uses the same type inference as loading (a legacy rule missing `type`, e.g. an old "otherwise", no longer falls back to `network`), so a config change is no longer misread as an external edit forcing an extra reload.

### 🛠 Internal

- Regenerated bindings and verified they match the Go API exactly (no diff).
- Version bumped to **1.1.8**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, NSIS, MSIX, Linux nfpm all in sync.

## [1.1.7] - 2026-08-31

This release fixes the issues reported on 1.1.6: automation rules no longer go missing, DNS leak detection now reports status and encryption, the route table marks VPN vs. direct routes, log filtering is fixed, and the notification-duration & proxy display issues are resolved. It also adds a connection-history retention setting and a "Run" option after installation.

### 🐛 Fixes

- **Automation rules no longer get lost (including "otherwise")** — the editor no longer misreads rules that lack a condition type as incomplete and drops them; on-disk rules the form can't represent are preserved verbatim, so rules can't vanish just by opening settings.
- **DNS leak detection completes its results** — each DNS server now correctly shows its probe status (VPN / Leak / OK / No reply) and latency, plus an "In use" marker identifying the current egress resolver.
- **DNS encryption fingerprint** — per-resolver transport detection: plaintext UDP/53, DoT (TCP/853 TLS), DoH (TCP/443 candidate); after the test, the UI explains the result and lists leak-prevention steps (use VPN DNS, encrypted DNS, full-tunnel mode, etc.).
- **Route table distinguishes VPN / Direct** — the backend authoritatively flags `is_vpn` by matching active tunnel interfaces, so rows show correct VPN / Direct badges instead of name guessing.
- **Log filtering fixed** — log events now carry the `category` field so category filters actually work; level/category buttons show per-bucket counts so the distribution is visible at a glance.
- **Notification duration setting** — fixed a dropdown that rendered blank under some Svelte versions, hiding the selected duration.
- **Proxy display consistency** — direct mode no longer leaves a stale proxy address behind; proxy mode changes made via CLI now sync into the settings UI live.

### ✨ Improvements

- **Connection-history retention** — Settings → Advanced now offers "Keep history for" (default 7 days, can be disabled); older sessions are pruned automatically (the 200-record hard cap still applies).
- **Run after install** — the Windows installer's finish page offers "Run WireGuide Plus" (checked by default).

### 🛠 Internal

- Version bumped to **1.1.7**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, NSIS, MSIX, Linux nfpm all in sync.

## [1.1.6] - 2026-08-30

This release upgrades the update mechanism: Windows / Linux can now download and install updates in-app (no longer just jumping to the GitHub page), the update notice offers both "Update Now" and "Open Release Page" buttons with a live download progress bar, and mirror mode now also accelerates asset downloads.

### ✨ Features

- **In-app update (Windows / Linux)** — the update notice gained an "Update Now" button: after download completes, the SHA256 checksum (plus Ed25519 signature in release builds) is verified, then the installer runs and the app exits. Homebrew installs on macOS still go through `brew upgrade`.
- **"Open Release Page" fallback button** — one click opens the matching GitHub Release page in the browser when the download fails, verification fails, or you just want to read the release notes.
- **Live download progress** — the update flow shows downloaded / total bytes and a percentage (based on the asset size reported by the GitHub API, so it stays accurate even with chunked transfers).
- **Mirror mode covers asset downloads** — with a GitHub accelerator mirror configured, asset and checksum downloads are rewritten through the mirror prefix too (previously only the API check used the mirror while binaries still hit GitHub directly).

### 🛠 Internal

- Download/install failures are no longer silent: they are logged and fall back to opening the release page, so a working path to the new version always exists.
- Added unit tests for the progress callback, mirror download rewriting and the `RunUpdate` defensive branches.
- Version bumped to **1.1.6**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm and macOS `Info.plist` all in sync.

## [1.1.5] - 2026-08-30

This release enriches the logging system (update checks, settings audits, categories, retention), fixes a few Settings issues, and brings back opt-in WireGuard scripts.

### ✨ Features

- **Full update-check logging** — manual and scheduled checks now log the endpoint actually queried, the local version, the latest version, `not_modified`, and error/retry details; failures (403, timeouts, …) are tagged with `category=update` so they show up and stay filterable in the Log viewer.
- **Settings-change audit log** — every save records which settings changed (proxy mode, kill switch, …) together with key values; proxy credentials are redacted (`http://***@host`).
- **Log categories & filtering** — `ipc.LogEntry` gained a `category` field (app / update / settings / tunnel / network / system); the Log viewer added a category filter row (All first, selected by default) and shows the category per line and when copying.
- **Log retention (default 7 days)** — logs rotate per day (`wireguideplus-YYYY-MM-DD.log`) and are pruned after a configurable retention period.
- **Opt-in WireGuard scripts (PreUp / PostUp / PreDown / PostDown)** — mirroring wg-quick (`sh -c` on Unix, `cmd.exe /C` on Windows), run inside the helper with a 30 s timeout and output capped at 1000 chars. Off by default (Settings → advanced); enabling shows a prominent security warning since the commands run with full system privileges. PostUp errors do not abort the connection.
- **DNS leak test enrichment** — each resolver now reports probe status (`vpn` / `ok` / `leak` / `timeout`) and latency; Windows DNS discovery now collects both IPv4 and IPv6 resolvers.
- **Open-folder shortcuts** — Settings now has clickable links that open the tunnel config folder and the log storage folder (cross-platform).

### 🐛 Bug Fixes

- **Notification duration could not be saved** — the value no longer resets after leaving and reopening Settings.
- **Settings log-level picker missed "All"** — the dropdown now offers `All` (matching the Log viewer's default) so no record is filtered at the sink level.

### 🛠 Internal

- **Log level "All" honored everywhere** — helper and GUI log handlers parse `all` (`slog.Level(-8)`) so no record is dropped.
- Version bumped to **1.1.5**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm and macOS `Info.plist` all in sync.

## [1.1.3] - 2026-08-30

This release fixes broken Windows auto-update: since the v1.1.0 asset rename, Windows release assets (`wireguideplus-<arch>-installer.exe` / `wireguideplus-<arch>-portable.zip`) carry no OS token in their names, but the update checker required the asset name to carry both an OS token and the architecture. Windows could therefore never match its own assets, and installed clients only saw "update available but no matching asset" without being able to auto-update.

### 🐛 Bug Fixes

- **Fixed Windows auto-update asset matching** — `matchAsset` (`internal/update/checker.go`) now also accepts arch-anchored Windows-native extensions (`.exe` / `.msi` / `.zip`) on Windows without an OS token; macOS / Linux assets still require their own OS token (`darwin` / `linux`), so they can never match a tokenless Windows asset name. Regression tests cover all three Windows architectures matching correctly and the reverse assertions that Linux / macOS must reject tokenless Windows asset names.

### 🛠 Internal

- Version bumped to **1.1.3**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm and macOS `Info.plist` all in sync.

## [1.1.2] - 2026-08-30

This release fixes a Windows file-version mismatch: in the published 1.1.1 installer, the running executable (`wireguideplus-<arch>.exe`) reported its "File version" as **1.1.0.1** instead of **1.1.1.0**.

### 🐛 Bug Fixes

- **Fixed the Windows executable file-version mismatch** — root cause: `goversioninfo v1.7` declares its `FixedFileInfo` struct as `Major/Minor/Patch/Build` (Patch and Build swapped vs. the standard Windows layout), so explicitly writing numeric versions into the JSON produced a swapped binary version (`1.1.1.0` became `1.1.0.1`). `build/windows/versioninfo.json` now pins the `FixedFileInfo` numbers to 0 and feeds only the four-part `StringFileInfo` strings as the single input; goversioninfo derives the binary version from them (layout-independent, always matching). `tools/genverinfo` renders string versions only and `tools/bumpversion` no longer touches the numeric fields. Verified: parsing the `1.1.2.0` strings makes goversioninfo emit `FixedFileInfo.FileVersion (1.1.2.0)`, and both Explorer's Properties page and `FileVersionInfo` show the correct value after install.

### 🛠 Internal

- Version bumped to **1.1.2**: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS (`wails_tools.nsh` + `project.nsi`), MSIX, Linux nfpm and macOS `Info.plist` all in sync.
- Corrected the NSIS installer/uninstaller descriptions (`project.nsi`) so the installer and uninstaller version info matches the shipped executable.

## [1.1.1] - 2026-08-30

This release fixes an intermittent GUI freeze when clicking "Open Window" on the Windows tray notification bubble under high system load.

### 🐛 Bug Fixes

- **Notification bubble "Open Window" no longer freezes the GUI intermittently** — under heavy CPU contention (e.g. a Windows maintenance process pegging a core) or WebView2 latency, clicking "Open Window" on the tray notification bubble synchronously waited on the UI thread, making the whole GUI appear frozen (the VPN tunnel kept working). `showDock` (`internal/gui/dock_other.go`) now runs asynchronously via `application.InvokeAsync` on the Wails UI thread: the caller returns immediately, window show/focus execute inline on the UI thread, and no cross-thread wait remains; a recover guard also prevents an unexpected panic from breaking the main-thread callback chain.

### 🛠 Internal

- Version bumped to **1.1.1**: `internal/update/checker.go` main version, `build/config.yml`, `windows/info.json` (`1.1.1.0`), `windows/wails.exe.manifest`, NSIS (`wails_tools.nsh`), MSIX, Linux nfpm and `tools/genverinfo` all in sync.

## [1.1.0] - 2026-08-28

This release focuses on distinguishability, proxy robustness and startup automation rules: tray state uses high-contrast glyphs, the proxy has three clearly defined modes with a connectivity test, invalid proxy URLs no longer break update checks, and startup now evaluates automation rules before connecting.

### ✨ Features

- **Tray state glyphs** — connection state in the Windows tray menu now uses plain-text glyphs: `●` solid = connected, `○` hollow = disconnected (Windows tray popups are drawn by GDI and cannot render colored emoji — `🟢` degrades to a grey outline, making old/new states hard to tell apart); the macOS menu bar (native AppKit rendering) keeps colored emoji. Startup/transition states have their own markers.
- **Proxy modes & connectivity test** — Settings → Proxy now offers three unambiguous modes: **Direct** (ignore system/environment proxy entirely), **GitHub mirror** (`mirror`, e.g. `https://ghfast.top` acceleration prefix), **Manual proxy** (`manual`, full http/https/socks5 URL). New **"Test Connection"** button: a round-trip request to the GitHub Releases API before saving, reporting success and latency.
- **Proxy applies immediately** — after saving proxy settings, the next scheduled update check (and manual "Check Now") applies without restarting; on GUI startup the saved proxy is applied directly too, avoiding "one check with a broken config right at launch".

### 🐛 Bug Fixes

- **Invalid proxy URL no longer breaks update checks** — a broken manual proxy in `config.json` (e.g. `proxy_url = "https://"`) was previously consumed directly by `http.ProxyURL`, making every update check fail with `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify must be specified in the tls.Config`. The URL is now validated at startup and on every use (`internal/update/proxy.go`); invalid values log `WARN update: ignoring invalid manual proxy URL` and fall back to a direct connection — checks no longer fail.
- **Fixed the "connect first, then rule-driven disconnect" startup feel** — startup rule evaluation moved to right after the helper starts (log `startup rule re-evaluation`), so each tunnel's target state is decided by the rules first; a `scheduleRuleCheck` fallback was added: within the first 60 s after startup, any RPC-driven manual connect (e.g. restoring the last session) is re-evaluated against the rules after 3 s and corrected, without waiting for the 30 s poll; the triggering source is logged for troubleshooting.
- **Invalid mirror prefixes no longer silently break checks** — the acceleration prefix in `mirror` mode is scheme/host validated too; illegal values fall back to the official API endpoint.

### 🛠 Internal

- Version bumped to **1.1.0**: `internal/update/checker.go` main version, `build/config.yml`, `windows/info.json` (`1.1.0.0`), `windows/wails.exe.manifest`, NSIS, MSIX and Linux nfpm all in sync.
- **Windows version resources standardized** — resources generated by `wails3 generate syso` had language `0x0000` and a zeroed `VS_FIXEDFILEINFO.ProductVersion`, so Windows Explorer / `FileVersionInfo` could not read them (blank version field in the properties page). Switched to `goversioninfo` (config: `build/windows/versioninfo.json`) producing standard `0409/04B0` resources; the `generate:syso` task was updated accordingly; the exe and installer properties now correctly show `1.1.0`.
- **New Windows x86 (32-bit) build** — `task windows:build ARCH=386` produces a 32-bit binary and the `wireguide-x86-installer.exe` installer (NSIS script supports x86, installs to Program Files, bundles the x86 `wintun.dll`).
- **Platform boundaries clarified** — iOS build task and config comments removed; Android / iOS are not supported (no concurrent multi-tunnel, no SSID-based auto-connect); the README explains this; macOS / Linux enhanced editions are pending.
- **System integration enhancements** — new **"Start minimized"** setting (launches directly into the system tray without the main window; Settings → Startup); new **tray connection notification**: the current connection state is shown 10 s after startup, and also 10 s after a network change (Wi-Fi switch, cable unplug, network loss, ...) alters tunnel connection state — the stable, latest state is shown. The bubble has an action menu (open main window / disconnect), can be dismissed manually, or auto-closes after the configured dwell time (default 10 s, adjustable in Settings → Startup → Notification dwell; `internal/gui/notify_windows.go`).
- **Dual-architecture releases** — every build produces both 32-bit (x86) and 64-bit (amd64) binaries and installers (`task windows:build:all`, with automatic wintun.dll architecture refresh); app/installer descriptions unified on "multi-tunnel + automation", cross-platform wording removed.
- **Install experience** — installers default to Program Files (the 32-bit installer auto-selects Program Files (x86)) and the directory can be changed during installation; a Start Menu shortcut (including the "Uninstall WireGuide Plus" entry, icon matching the running app) is created by default and can be unchecked on a "Shortcut options" page; a desktop shortcut is always created (`build/windows/nsis/project.nsi`).
- **Development & release docs** — build/packaging docs moved from the README to the standalone `docs/DEVELOPMENT.md`; the GitHub Release workflow now includes the 32-bit Windows artifact and the CI toolchain (`goversioninfo`); pushing a local `v*` tag builds (Windows x86+amd64, macOS arm64, Linux amd64+arm64), signs and publishes automatically (`docs/release.md`).
- Windows adapter-name matching adjusted (`internal/wifi/known_windows.go`, `detect_windows.go`) for more accurate physical-adapter detection.
- Window title unified to **WireGuide Plus**.
- Update checks deduplicated inside the scheduler to avoid multiple triggers in one round (only one failure logged, with a retry interval).

## [1.0.0] - 2026-08-28

Milestone release: A11y accessibility semantic refactor, Windows network-egress routing logic changes, Wails3 build/icon/permission cleanup, plus a Simplified Chinese UI and tray toggles.

### ✨ Features

- **Simplified Chinese UI (Chinese UI)** — full Simplified Chinese translations across the whole interface, covering all 199 strings: tunnel list, history, tools (DNS leak test / route table), logs, settings, updates, automation editor. The first launch auto-follows the system language (`zh-*` locales detected), or you can switch manually in Settings → General → Language (persisted).
- **Tray toggles** — every tunnel in the system tray is now an independent clickable toggle: check to connect, uncheck to disconnect; the connection emoji (🟢 connected / 🟡 connecting / ○ disconnected) stays next to the label. Manually disconnected tunnels remain exempt from automation rules (manual-off) until reconnected or WireGuide restarts.

#### Frontend A11y accessibility refactor

> Scope: all platforms (Windows/macOS/Linux) Svelte frontend, not just Windows.

- All modal overlays dropped the `role="button"` and `tabindex="0"` from the scrim, returning it to a pure mask so screen readers no longer treat the fullscreen background as an interactive button.
- All dialogs use `tabindex="-1"` and keep the standard `role="dialog" aria-modal="true"`, following WCAG dialog semantics.
- ESC close unified: dialogs that lacked it (import result, history, update notice, automation editor) now mount `<svelte:window on:keydown>` at the component top level (the handler checks dialog state; Svelte does not allow mounting inside `{#if}`); the rest reuse App.svelte's global capture handler — avoiding multi-dialog ESC conflicts without breaking CodeMirror's key capture.
- `Settings.svelte`: `<nav role="tablist">` replaced with a plain `<div>` to silence tab-semantics warnings; the `pane-resizer` splitter keeps `role="separator"` but gains `tabindex="0"` and real keyboard operation (arrow keys resize, Enter/Space reset).
- `frontend/vite.config.js`: the svelte plugin `onwarn` now filters static false positives (`a11y_click_events_have_key_events`, `a11y_no_static_element_interactions`, `a11y_no_noninteractive_tabindex`, `a11y_no_noninteractive_element_interactions`); production build warnings are zeroed, with no logic changes.
- Files touched: `src/App.svelte`, `src/lib/History.svelte`, `src/lib/ConflictWarning.svelte`, `src/lib/TunnelDetail.svelte`, `src/lib/UpdateNotice.svelte`, `src/lib/Settings.svelte`, `src/lib/AutomationEditor.svelte`

#### Windows background helper: network egress routing logic

> Scope: Windows-only Go helper code; other OSes are unaffected.

- At startup the helper captures the primary upstream physical adapter's LUID to record the system's initial default egress physical interface; this LUID is a boot-time snapshot and does not auto-refresh on runtime network switches.
- Fixed the network-interface filtering: TUN/tunnel/loopback virtual adapters are filtered out, only physical adapters are upstream candidates; TUN virtual adapters are no longer bound/locked as if they were physical.
- WireGuard UDP egress is fully delegated to the Windows routing table + per-adapter InterfaceMetric hop counts; the software no longer force-binds a fixed physical adapter.
- Split-tunnel (`full_tunnel=false`) constraint added: the Peer Endpoint IP must be explicitly included in `AllowedIPs`, preventing handshake UDP packets from being route-dropped (`no-handshake`).
- Logging: `network primary upstream interface initial luid` outputs the primary physical adapter LUID for troubleshooting; the log line `tunnel connected` only means the TUN adapter is ready, not that the remote peer handshake succeeded.
- Troubleshooting hints: on Windows prefer `Find-NetRoute -RemoteIPAddress <peer-ip>` to determine the actual egress adapter for a target IP; PowerShell's `Get-NetAdapter.Luid` is a struct and cannot be compared directly to Go's uint64 output.

### 🛠 Build & Project

Mostly Windows build behavior; cross-platform parts are marked.

1. **Wails3 Windows icon build behavior** (Windows only) — `task build` full builds automatically run `wails3 generate icons`, reading `build/appicon.png` and overwriting `windows/icon.ico`; manual edits to `windows/icon.ico` are overwritten by full builds. `windows/icon.ico` is the icon ultimately embedded in the exe; `build/appicon.png` is only the source asset; `task windows:build` debug builds skip icon generation and keep the existing `windows/icon.ico`. The exe / window title bar / taskbar icons reuse the ico resource inside the exe; the system tray icon needs a separate Go `embed` resource.
2. **Windows version info management** (Windows only) — exe file details come from `windows/info.json`; `FileVersion` must be 4-part numeric `major.minor.patch.build`. The UI-displayed version is maintained as a Go constant (`internal/update/checker.go`) and must be kept in sync with `info.json` manually; later, ldflags build-time injection could give a single version source.
3. **Windows UAC / admin privileges** (Windows only) — current architecture: the GUI launches a helper subprocess; the helper operates TUN adapters and modifies routes, which requires admin rights, and elevating the subprocess triggers the UAC prompt — Windows security cannot be fully bypassed silently. Short term: `windows/wails.exe.manifest` adds `requireAdministrator`, moving the UAC prompt to double-click exe launch (still requires user confirmation); long term: refactor the helper into a Windows System Service (LocalSystem, running in the background) with the GUI communicating over IPC as a normal user, eliminating the UAC prompt entirely.

### 🐛 Investigation

Investigation notes, no code changes, for developer reference.

- Symptom: helper logs `tunnel connected`, but the GUI shows `no handshake`.
  - Root-cause: a TUN device being created ≠ the WireGuard encrypted handshake with the remote peer having completed; read the wg kernel `latest handshake` state to judge real connectivity.
  - Split-tunnel pitfall: Peer IP not in `AllowedIPs` → handshake UDP packets route-dropped.
  - Other possibilities: Windows outbound firewall blocking WireGuard UDP, endpoint domain DNS resolution failure.
- A local proxy listening on `0.0.0.0`: proxy process traffic is independent and does not automatically flow into the WireGuard tunnel; traffic direction is decided jointly by the Windows routing table and the tunnel's `AllowedIPs`.

### 📝 Notes

1. **Scope of the changes**
   - Svelte frontend A11y code: **applies on all platforms (Windows / Linux / macOS)** — ESC handling and accessibility semantics affect every desktop platform.
   - Helper network egress routing: **Windows-only Go code changes**; other OSes are unaffected.
   - Build, manifest, ico, info.json, UAC: **Windows only**.
2. The frontend A11y changes are fully decoupled from the helper's background network logic; they do not affect tunnel creation, routing, or automation Wi-Fi rules.
3. The helper's recorded upstream LUID is only a boot-time snapshot; it does not auto-update when switching between Wi-Fi/wired networks.

## [0.5.1] - 2026-08-11

Patch release: the in-app "Update Now" button is now trustworthy on macOS. If you are on 0.5.0 via Homebrew, this is also the first update the button itself should complete cleanly end-to-end.

### Fixed
- **macOS "Update Now" (issue #38)** — the in-app update can no longer report success without actually installing: after `brew upgrade` exits, the installed bundle's version is verified against the release it claimed to install, progress phases ("refreshing" / "installing") are shown in the banner and About panel, and failures surface inline instead of vanishing behind a relaunch. Also survives Homebrew 6's tap-trust gate (`untrusted tap` errors trigger a `brew trust` + one retry) and skips the redundant `brew update` (`HOMEBREW_NO_AUTO_UPDATE=1` — the checker already knows the target version).
- The Homebrew cask itself dropped `auto_updates` (korjwl1/homebrew-tap), so bulk `brew upgrade` no longer skips WireGuide — the root cause of months of silent non-updates.

## [0.5.0] - 2026-08-10

Linux graduates to a supported platform, the CLI learns to start and stop the app, and the Windows helper's IPC surface is locked down to the launching user. Verified on all three OSes before release: a full runtime pass on Windows 11 against a real tunnel (helper IPC, multi-tunnel, kill-switch cycles, CLI lifecycle, tray), the Linux plan in `docs/linux-test-plan.md` on Debian 13 / Raspberry Pi OS ARM64, and the macOS DNS/lifecycle fixes below.

### Added
- **Linux support** — tested and hardened end-to-end on Debian 13 / Raspberry Pi OS ARM64 (Wayland and X11): window decorations restored after tray-restore, gateway/physical-interface detection fingerprints the right network (issue #22), routine RTNETLINK traffic no longer registers as a primary-network change (reconnect decisions compare real default-route snapshots), nftables kill-switch fixes, DEB packaging via nfpm.
- **`wireguide ctl start` / `ctl stop`** — explicit app lifecycle from the CLI. `start` launches the app detached and waits for the helper (long deadline: the macOS admin prompt has no timeout of its own; on macOS it launches its *own* bundle rather than whatever LaunchServices resolves); `stop` quits GUI and helper together and confirms they actually went away. Deliberately the only commands that start anything — `connect`/`status` still refuse rather than boot a VPN stack behind your back.
- **`--json`** on `ctl status` and `ctl list` for scripts and coding agents.
- **CI: 3-OS test matrix** (Linux/macOS/Windows) on every PR; release workflow untouched.

### Security
- **Windows helper pipe scoped to the spawning user (issue #20)** — the named pipe's ACL now grants access to the launching user's SID instead of every interactive user, and each connection's peer SID is verified against it (SYSTEM and a helper spawned without the SID keep working). Verified live on Windows 11 by reading back the pipe's security descriptor.

### Fixed
- **Windows multi-tunnel** — connecting a second tunnel no longer fails on the Wintun adapter name collision; each tunnel gets its own `WireGuide-<id>` adapter, and multi-tunnel status reports per-tunnel interface/duration/traffic instead of zeroed copies.
- **Helper lifetime** — the helper never runs at boot and its lifetime is tied to the GUI: a 60 s startup grace covers a helper whose GUI never attaches (login-autostart with an unanswered UAC prompt no longer leaves an invisible elevated process), and a teardown that leaves no tunnels and no GUI re-arms the shutdown grace window — closing the orphan-helper hole that transient CLI connections opened (a GUI-less `ctl disconnect` of the last tunnel previously left the elevated helper alive until reboot). CLI clients are excluded from connection-lifecycle tracking by design.
- **Kill switch** — rebuilt atomically around every connect/disconnect from actual manager state; a failed connect restores the blockade instead of leaving it half-applied.
- **macOS DNS teardown (issue #34)** — search domains, services added mid-session, and the failed-verify / ForceShutdown paths now all restore DNS.
- **macOS updates (issue #38)** — "Update Now" runs `brew upgrade --greedy` so cask-held updates can't silently no-op.
- **Diagnostics (issue #32)** — ping parsing is locale-agnostic (Korean Windows included), and unreachable hosts report as unreachable instead of a fabricated wall-clock-derived latency.
- **Automation** — rules are validated on save: a malformed CIDR or MAC is rejected with a clear error instead of being written and silently never matching.
- **Idle efficiency** — Wi-Fi polling backs off to 60 s while native change notifications are attached; config-file watching drops from 1 s to 3 s; endpoint-latency logging demoted to debug.

### Removed
- Key generator, CIDR calculator, speed test, mini mode, and the split-tunnel UI stub — dead or abandoned surfaces found in the audit sweep (#35); their bindings and i18n strings went with them.

## [0.4.2] - 2026-07-27

**Urgent fix release for Windows users.** 0.4.1 and earlier shipped with a tray that could permanently lose the main window and an installer that cannot upgrade in place while the app is running. Windows users should update; to get past the installer bug one last time, run `taskkill /F /IM wireguide.exe` from an elevated terminal before launching the 0.4.2 installer. macOS and Linux are unaffected by the tray-window bug (Linux picks up the same Show Window fix), and nothing else changed.

### Fixed
- **Windows tray, issue #30** — left-clicking the tray icon now shows the main window (the platform convention; previously a no-op), and the "Show Window" menu item actually works: it was wired to a macOS-only implementation, so on Windows **a window closed to the tray could never be reopened** — the only recovery was killing the process. The tray menu also showed stale connection state (○ while connected) because menu refills never reached the Win32 popup; the menu now rebuilds through `SetMenu` on every change. macOS behavior is unchanged; Linux gains the same Show Window fix.
- **Windows installer, issue #29** — upgrading by running the installer while WireGuide was running failed with "Error opening file for writing: wireguide.exe" (the GUI and the elevated helper are the same executable, and Windows locks running images; the helper deliberately outlives the GUI, so quitting the tray app wasn't enough). The installer and uninstaller now terminate running instances before touching files. **This fix takes effect when the 0.4.2 installer runs — upgrading *to* 0.4.2 still hits the old installer's bug**, hence the elevated `taskkill` workaround above.

## [0.4.1] - 2026-07-27

### Fixed
- **Automation (GUI), issue #27** — creating or editing rules in the Automation editor was effectively impossible in 0.4.0: the editor's own autosave re-fired the config watcher, and the resulting reload wiped the just-added row before it could be filled in (and could transiently delete a rule being edited). The editor now ignores its own writes (reloading only when the file genuinely changed externally), a blank draft row is no longer autosaved, and a rule that is momentarily incomplete mid-edit keeps its last saved value on disk instead of being deleted. External edits (`wireguide ctl`, another window) still appear live.
- **Automation (GUI)** — per-tunnel rule saves now go through the cross-process-locked settings update instead of a whole-settings overwrite, so a GUI rule edit can no longer clobber a concurrent `wireguide ctl` change to any other setting (and vice versa); condition labels survive the GUI round-trip; a dash- or bare-hex-formatted gateway MAC written by the CLI is no longer treated as a foreign change.
- **Windows (dev):** `go test ./internal/ipc` no longer fails/panics when run unelevated — the tests accept the test binary's own pipe (test builds only; the production SY/BA pipe-owner check is unchanged) (#24).

## [0.4.0] - 2026-07-15

### Added
- **Automation** (issue #12) — per-tunnel `condition → action` rules that connect or disconnect a tunnel based on the network you're on. Conditions: Wi-Fi SSID, subnet (CIDR), or the default-gateway MAC (a precise, medium-agnostic network fingerprint that tells apart networks sharing a subnet); action: connect/disconnect. Rules are ordered by priority (drag-to-reorder, first match wins) and evaluated entirely in the helper via a hybrid trigger (macOS route-monitor subscription; 30 s poll on Windows/Linux). Replaces the legacy per-tunnel Wi-Fi auto-connect / trusted-SSID UI (migrated automatically). Editable in the GUI or via the CLI.
- **Command-line interface** `wireguide ctl` (issue #10) — a third IPC client alongside the GUI (Tailscale-style): `status`, `list`, `connect`, `disconnect`, `import`, `rename`, `delete`, and `automation add/rm/rules` + a read-only decision preview. No per-command sudo, cross-platform, shares the GUI's tunnel store.
- Tunnel-list **sorting** (name / last used / date added, active-on-top) and **compact mode** (issue #16, #17); **drag-resizable** tunnel-list column.

### Fixed
- **update:** the Ed25519 signature is now bound to the hash actually installed (a repo-write attacker could previously pass both checks by swapping SHA256SUMS between check and download); `Install` also enforces `SignatureVerified` in signed-update builds.
- **Windows:** `findInterfaceMTU` buffer overflow + wrong `NlMtu` offset (undefined behaviour on every no-MTU connect; auto-MTU always fell back).
- **Linux:** split-tunnel routes were deleted from the wrong table on the default `Table=auto` path (route leak); DNS search-domain injection; nft kill-switch endpoint-port validation and `oifname` consistency.
- **macOS:** `route -n monitor` subprocess is now supervised (was a silent zombie + stuck monitor on unexpected exit); the tray menu-bar icon uses native click-to-open (fixed the "does nothing on macOS 26" report, issue #18) and follows the menu bar's actual appearance; the connect/Disconnect-race no longer holds `Manager.mu` across slow teardown.
- **storage:** reject case-collisions and Windows reserved names; fsync the parent directory after atomic writes; latency-probe target validation; meta-sidecar lost-update race.
- **Automation (code review, issue #12):** `else`/none_match now matches at its own position so drag-to-reorder priority is uniform (was always held to the end); malformed conditions and unknown actions now fail closed (rule skipped) instead of an unknown action defaulting to connect; a rule-driven connect now runs the same DNS-protection + kill-switch folding as a manual connect (headless automation could previously connect with no protection, or fail entirely under an already-on kill switch), and a rule-driven disconnect strips the tunnel from the kill-switch filter set; macOS no longer overwrites the GUI-reported SSID with an empty root-helper poll (which silently broke SSID rules); Windows gateway-MAC resolves the physical underlay gateway (excluding the WireGuard adapter) so a full tunnel no longer blanks the fingerprint and flaps `mac:` rules; tunnel rename/delete now carry/drop the tunnel's automation rules instead of orphaning them; the rule editor no longer races a debounced save against a tunnel switch. *(Windows gateway change compiles but is unverified on a Windows build.)*
- **config.json:** cross-process read-modify-write is now atomic (file lock) so a `wireguide ctl` edit and a GUI edit can't clobber each other.
- **CLI (issue #10):** `import`/automation edits work on a fresh install (dirs created); `set` exits nonzero when the helper is running but the live apply fails; `delete` refuses to remove a still-connected tunnel whose disconnect failed; `install-skills` writes agent files atomically. The NSIS installer PATH edit no longer interpolates the install path into a PowerShell command (injection), and the macOS cask + Windows installer put `wireguide` on `PATH`.
- **list:** date-added sort now uses a stamped creation time (survives edits) instead of the `.conf` mtime (issue #17).

### Changed
- Latency probe no longer fabricates a `x.x.x.1` gateway target (issue #15); per-tunnel latency target added.

## [0.3.1] - 2026-05-26

### Added
- **Full-tunnel routing-loop protection (Windows + macOS)** — multi-layer defense against the encrypted-UDP-loops-through-tunnel-adapter class of bug (issue #14).
  - Windows: WFP block at `ALE_AUTH_CONNECT_V{4,6}` + `OUTBOUND_TRANSPORT_V{4,6}` layers, iphlpapi-based `/32` bypass host route with `InitializeIpForwardEntry`, `IP_UNICAST_IF` UDP socket binding with `NotifyRouteChange2`-pushed re-pin monitor, runaway-TX watchdog with sustained-asymmetry trip.
  - macOS: `/32` bypass installed before `/1` split routes with fail-fast preflight on missing default gateway, 5 s underlay-detection retry, blackhole fallback on gateway loss inside `reapply` to keep the loop class contained when the upstream gateway briefly disappears, runaway-TX watchdog via `netstat -ibnI`.
- **SignPath Foundation code signing** — CI hooks for SignPath OSS signing of the Windows installer; gated on the foundation's onboarding approval. Releases ship unsigned until then.

### Fixed
- Helper now exits within ~20 s of the GUI dying (was ~70 s) — IPC read deadline trimmed to 10 s now that the GUI's 5 s health-monitor ping cadence is the canonical liveness signal.
- macOS: `RestoreDNS` no longer fires a noisy `netsh`-equivalent against an adapter that's already been detached from the IP stack during disconnect.
- macOS: `getDefaultInterface()` now parses the `netstat -nr` header dynamically; previously the "first lowercase field" heuristic could misidentify `awdl0` (AirDrop) as the default interface on some machines.
- Windows: UAPI listener "may not work" warning downgraded to DEBUG on Windows — the named-pipe bind is expected to fail because the helper runs as an elevated user rather than as `LocalSystem`; status queries route through the in-process `Engine.IpcGet` regardless.

### Changed
- CI release notes generated by `git-cliff` (fuller diffs than the previous auto-generated body).
- CI: explicit NSIS install on Windows runners (the default Windows-latest image no longer carries `makensis` on PATH).
- CI: `Get-FileHash` / `Expand-Archive` in the wintun vendoring step replaced with direct .NET APIs to avoid PowerShell version skew on the runner.
- README: `Install` section moved above `Features`, code-signing dev-process notes trimmed to user-facing status only.

## [0.3.0] - 2026-05-25

### Added
- **Windows kill switch via WFP** — Windows Filtering Platform-based kill switch that survives helper restarts; complements the existing macOS `pf` and Linux `nftables` implementations.
- **Periodic auto-update scheduler** — background check for new releases on a configurable cadence (default 24 h with focus-opportunistic refresh), separate from the existing manual "Check for updates" path.
- **CI release pipeline** — automated darwin (arm64) + Windows (amd64/arm64) builds on tag push, with SHA256SUMS, Ed25519 signature, and `homebrew-tap` cask auto-bump.

### Fixed
- macOS kill switch: `pf` anchor renamed from `com.apple.wireguide` (dot) to `com.apple/wireguide` (slash) so it actually matches the `anchor "com.apple/*"` wildcard in the system `/etc/pf.conf` — previously the rules loaded without ever being evaluated.
- macOS kill switch can now be toggled on without an active tunnel (base block-all set installs cleanly; per-tunnel permits are folded in on subsequent connects).
- Windows disconnect: lingering wintun adapter "defanged" (DNS cleared, metric bumped) before `engine.Close`, so the brief window where Windows still treats the dying adapter as a viable metric-1 path doesn't dump every DNS query onto its dead `8.8.8.8` binding.
- Windows disconnect: dead 12 s DNS-restore call removed; `netsh` output now decoded as the OEM code page so Korean / non-English Windows installs no longer mis-parse error messages.
- Windows: UAPI bypass (status queries served by in-process `Engine.IpcGet` rather than the named pipe that the elevated helper can't bind under the kernel's owner-SID requirement).
- Windows: suicide-reconnect / orphaned `conhost` / dangling route fixes from the WFP kill-switch rework.
- DNS protection regression introduced during the periodic-update-scheduler refactor.
- Numerous race conditions, leak fixes, and audit findings from the cross-platform hardening pass.

### Changed
- Tray and taskbar icons: rounded silhouette via custom genicon (matches the macOS dock icon's visual weight).
- Sidebar dividers, tool pages, and drop affordance polished.
- Settings: maintainer credit added in footer; helper SIGTRAP fix.
- Rebrand: WireGuide red accent + Material-style flat buttons.

## [0.2.0] - 2026-05-05

### Added
- **Wi-Fi auto-connect rules** — per-tunnel SSID-based auto-connect/disconnect; rules fire in the helper so they work even when the GUI is quit
- **Trusted SSID support** — designated SSIDs auto-disconnect all VPN tunnels (home/office network detection)
- **macOS 14+ Location Services integration** — CoreWLAN CGo replaces `networksetup` for SSID detection; app now appears in System Settings → Location Services
- **GUI→Helper SSID forwarding** — on macOS 14+ the helper (root LaunchDaemon) cannot read SSID itself; the GUI polls via CoreWLAN and forwards changes over IPC so auto-connect rules fire correctly
- **Ed25519 signature verification** — auto-update downloads verified against a Ed25519 signature over SHA256SUMS; embedded public key prevents tampered binaries from being installed

### Fixed
- Wi-Fi auto-connect status not updating in GUI/tray after rule fires (`ActiveTunnels` now populated in all status broadcasts)
- `autoConnectedBy` accessed under wrong mutex in `handleRename` (race condition; changed to `wifiMu`)
- Lock ordering violation between `handleRename` and `handleSSIDChange` that could cause deadlock
- Kill switch and DNS protection handlers using `Status().State` instead of `IsConnected()` (broke in multi-tunnel setups where the primary was not the connected tunnel)
- `handleReportSSID` panic on nil `wifiMon` (non-darwin builds and pre-init race)
- `sleep_darwin.go` unsafe.Pointer misuse flagged by `go vet`; replaced with `runtime/cgo.Handle`
- Duplicate SSID appearing in Wi-Fi rules dropdown when current SSID matched a saved rule

### Changed
- Auto-connect logic moved to helper process (was frontend-side) so rules fire independently of GUI lifecycle
- `postConnectRefresh` refactored: `refreshTunnels`+`refreshStatus` kept for manual connect UX; auto-connect path calls only `applyFirewallSettings` (event stream handles status update)
- Dead backward-compat fallback in `subscribeToEvents` removed (active_tunnels now always populated)

## [0.1.9] - 2026-05-05

### Changed
- Removed Wi-Fi rules master toggle; trusted SSIDs are always active when configured

### Fixed
- Various regressions, lifecycle, and performance issues from audit rounds (Round 2, Round 3)
- 30+ fixes from full-codebase review (null guards, lock safety, error propagation)

## [0.1.8] - 2026-04-13

### Changed
- Sidebar navigation: removed Tools tab bar, DNS Leak Test and Route Table are now direct sidebar sub-items
- Settings modal: fixed size regardless of active tab (no more resize when switching to Advanced)
- Settings sidebar active state: tint highlight instead of solid blue (macOS HIG)
- Dropdown controls: custom styled per macOS HIG (28px height, 6px radius, theme-aware chevron)

### Improved
- Route table: sticky column header, legend pinned to bottom, table fills remaining space with scroll
- DNS Leak Test and Route Table now call real backend (previously stub implementations)
- macOS HIG design tokens: added `--border-strong` for input control borders

### Removed
- Network Diagnostics (Ping) tool — not meaningfully useful as a standalone feature
- Unused i18n keys for removed Diagnostics feature

## [0.1.7] - 2026-04-09

### Added
- Multiple simultaneous tunnel support
- Per-tunnel NetworkManager (independent routes, DNS, route monitor per tunnel)
- Per-tunnel health check and reconnection
- Full-tunnel conflict detection (reject two 0.0.0.0/0 configs)
- DNS union across all active tunnels
- No-handshake warning: orange dot in tunnel list, ◐ in tray menu
- Tray menu shows per-tunnel connection + handshake status
- Architecture & design documentation (docs/DESIGN.md)

### Fixed
- Disconnect one tunnel no longer breaks other active tunnels
- Conflict detection: macOS netstat abbreviated CIDRs now parsed correctly
- GUI not reflecting connection state when tunnel connected via system tray
- Bypass route race conditions (lock safety, error propagation)
- Tray icon padding: trimmed transparent pixels for tighter menu bar fit
- Tunnel list unnecessary re-renders on every status tick
- README streamlined: removed defensive tone, screenshots moved to top

### Changed
- Pin Interface toggle added (Settings > Advanced) for dual-network stability
- Bypass routes pinned to upstream interface with -ifscope when enabled

## [0.1.6] - 2026-04-08

### Added
- Settings redesign: split layout with sidebar (General / Advanced / About)
- About tab: app icon, version, GitHub/Issues/License links, update status
- Update popup: modal with release notes ("What's New") and "Skip This Version"
- Helper auto-upgrade: detects version mismatch and reinstalls on app update
- Helper install retry dialog with Quit/Retry options on cancel
- OpenURL Wails binding (restricted to github.com)
- Tests for IsBrewInstall and OpenURL validation (7 new tests)

### Fixed
- Brew install detection: check Caskroom receipt instead of binary path
- Non-brew update: opens GitHub Releases page instead of broken auto-download
- Brew update: runs `brew update` before `brew upgrade` for third-party taps
- Helper Ping response: separate AppVersion field (fixes IPC protocol validation)
- Update popup double-click guard
- localStorage exception handling for skip version
- Detailed admin prompt explaining why password is needed

### Changed
- README/About description: "native macOS" → "cross-platform"

## [0.1.5] - 2026-04-07

### Added
- Health Check toggle in Settings (default: off, recommended with PersistentKeepalive)

### Changed
- Health Check default changed from on to off (consistent with other WG clients)
- README rewritten: removed aggressive tone, verified claims, acknowledged official app works for many users

## [0.1.4] - 2026-04-07

### Security
- Remove script execution (PreUp/PostUp/PreDown/PostDown) — eliminates local privilege escalation via ApproveScripts RPC
- Fix Windows IPC ACL: allow non-admin GUI to connect to helper pipe
- Harden update integrity: asset size validation + Content-Length check

### Fixed
- Kill switch pf rules: use anchor-only approach instead of modifying main ruleset (fixes Tahoe compatibility)
- Kill switch + DNS protection now toggleable while VPN is connected
- Kill switch reconnect deadlock: suspend/resume firewall rules during reconnect
- Log viewer scroll not working
- Tunnel list scroll overflow

### Added
- Handshake-based health check: detects dead tunnels and triggers reconnect after 180s
- Instant sleep/wake detection via NSWorkspace notification (polling fallback kept)
- Typed tunnel error enums (ErrAlreadyConnected, ErrNetwork, etc.)
- DNS post-write verification
- Crash recovery journal with pre-modification DNS snapshot
- Comprehensive unit tests (102 tests, race-clean)
- CHANGELOG.md
- Info-level logs for kill switch and DNS protection events

## [0.1.3] - 2026-04-07

### Fixed
- "Show Window" not working after closing the window (RegisterHook instead of OnWindowEvent)
- Dock icon hide/show when window is closed/reopened
- App icon showing Wails default (white W) instead of WireGuide red icon
- About/Settings dialog showing wrong version — now fetched dynamically from Go

### Added
- GitHub issue templates (bug report, feature request)
- CONTRIBUTING.md and PR template

## [0.1.2] - 2026-04-07

### Fixed
- Dock icon not hiding when window is closed
- Tunnel list not updating after rename

## [0.1.1] - 2026-04-06

### Fixed
- Daemon socket directory permissions (0700 → 0755)
- LaunchDaemon install flow rewrite (app first-launch, not cask postflight)

### Added
- Version display in Settings

## [0.1.0] - 2026-04-05

### Added
- Initial release
- WireGuard tunnel management (import, create, edit, export .conf files)
- Config editor with CodeMirror 6 syntax highlighting and autocompletion
- System tray with connection status badge
- Kill switch via macOS pf
- DNS protection (force DNS through VPN tunnel only)
- Auto-reconnect with exponential backoff
- Sleep/wake recovery
- Route monitor for gateway changes
- Conflict detection (Tailscale, other WG interfaces)
- Network diagnostics (ping, DNS leak test, route table)
- Auto-update (GitHub Releases + Homebrew)
- Real-time RX/TX speed graph
- i18n (English, Korean, Japanese)
- Dark / Light / System theme
