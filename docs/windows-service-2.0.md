# WireGuide Plus 2.0 — Windows System Service Development Guide

> Target version: 2.0
> Scope: **Windows only**. Turn the 1.x "GUI-driven + elevated helper"
> architecture into a "Windows system service" architecture.
> Related code: `main.go`, `internal/helper`, `internal/ipc`, `internal/gui`,
> `internal/elevate`, `internal/cli`, `internal/autostart`,
> `build/windows/nsis`
>
> 简体中文版：见 [windows-service-2.0.zh.md](windows-service-2.0.zh.md)

---

## 0. Scope: Why Windows Only

The 2.0 service-ization effort is **specific to Windows**:

| Platform | Current privileged mechanism | 2.0 work needed |
|---|---|---|
| Windows | UAC-spawned elevated helper tied to the GUI (no service at all) | **Full system-service re-architecture — this guide** |
| macOS | Helper already runs as a LaunchDaemon (`/Library/LaunchDaemons/com.wireguideplus.helper.plist`, `RunAtLoad=false` — see `internal/elevate/spawn_darwin.go`) | None for 2.0. Making it persist at boot is a one-line plist change, deferred |
| Linux | Helper spawned via `pkexec` on demand (`internal/elevate/spawn_linux.go`) | None for 2.0. A small systemd unit would cover "run at boot"; low value, deferred |

In short: macOS and Linux already have (or trivially can get) system-level
daemon mechanisms. Windows is the only platform with **no service layer
today** — the elevated helper lives and dies with the GUI. This guide
therefore targets Windows exclusively.

**The management GUI remains the primary management surface in 2.0.** It
just becomes a *client* of the service instead of the service's owner: no
more UAC prompt per session, same tray + settings + automation UX.

## 1. Goals & Benefits

2.0's core goal: the core engine (tunnel management, firewall, automation)
runs as a **Windows system service**, starts at boot, and works unattended —
with or without the GUI.

| Benefit | Description |
|---|---|
| Ready at boot | Service starts before logon via SCM; no login/UAC prompt needed |
| Unattended | WiFi-rule auto-connect, reconnect, crash recovery all work headless |
| Reliability | SCM recovery policy (auto-restart on crash) beats the current UAC flow |
| Session 0 | Tunnel survives lock screen / logoff |
| Decoupling | GUI is a replaceable client; can be updated/closed independently |

## 2. Current State

### 2.1 1.x architecture

```
┌─ User session ───────────────────────────┐
│  GUI (Wails3 + React)                    │
│    │  ensureHelper(): UAC spawn / dial   │
│    ▼                                     │
│  elevated helper process (admin)         │
│    tunnel mgmt / WFP firewall / reconnect│
│    / WiFi automation                     │
└──────────────────────────────────────────┘
   IPC: named pipe `\\.\pipe\wireguideplus`
   Security: SDDL = SY+BA full access, spawning user GRGW;
             client verifies pipe owner ∈ {SY, BA}
```

- **Entry point**: `main.go` flag dispatch (`--helper` / GUI / `ctl`)
- **Helper core**: `Run(addr, ownerUID, ownerSID, dataDir)` in
  `internal/helper/helper.go`, already provides:
  - `tunnel.Manager`: wireguard-go + wintun, connect/disconnect/status
  - `firewall`: WFP kill switch / DNS protection / endpoint protection
    (`internal/firewall/wfp_windows.go`)
  - `reconnect.Monitor`: reconnect with firewall suspend/resume
  - `wifi.Monitor` + rule evaluation: SSID/subnet-triggered auto-connect
    (`internal/helper/automation_rules.go`)
  - Crash recovery: `tunnel.RecoverFromCrash` + `fw.RecoverFromCrash`
  - Event broadcast: `ipc.Server.Broadcast` (status diff, WiFi change, reconnect state)
  - Restores persisted settings on start (health check, pin-interface, log level)
- **Lifecycle (the key GUI-driven design in 1.x)**:
  - `shutdownGrace = 10s`: GUI disconnected + no active tunnel → self-exit
    (`armShutdownTimer`)
  - `startupGrace = 60s`: no GUI connection after start → self-exit
  - Never exits while a tunnel is active (wg-quick semantics)
- **IPC security model** (`internal/ipc/transport_windows.go`):
  - Pipe SDDL: `D:(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<ownerSID>)` —
    **directly reusable for the service**
  - Client `verifyPipeOwner`: pipe owner must be ∈ {SY, BA} (anti-spoofing)
  - Server `verifyPeer`: connecting process must belong to `ownerSID`
  - Connection cap 32 (protects against same-UID resource exhaustion)
- **Data dir**: `systemDataDir()` = `%PROGRAMDATA%\wireguideplus` (helper state)
- **User config**: `storage.TunnelStore` in the user's home. ⚠️ On Windows,
  `deriveUserAppSupport` (`internal/helper/userdir_windows.go`) locates the
  user dir via the `APPDATA` env var passed through UAC — **an SCM-started
  service has no user environment**, so this path breaks under the service;
  Phase 2 must address it
- **CLI**: `wireguideplus ctl …` is already a standalone IPC client
  (Tailscale-style)
- **Install**: NSIS (`build/windows/nsis/project.nsi`) + `internal/autostart`
  (Windows uses HKCU Run key)

### 2.2 Gaps vs 2.0

| # | Gap | Impact |
|---|---|---|
| 1 | Lifecycle is GUI-driven (exits on disconnect) | Service must persist; exit logic needs a mode switch |
| 2 | Spawned by the GUI via UAC (`elevate.SpawnHelper`) | Replace with SCM registration + auto-start |
| 3 | Single-user binding (`ownerSID`) | Service runs in session 0; multi-user handling needed |
| 4 | User config lives in the user home | Service (system identity) can't resolve user paths |
| 5 | No service-control protocol | Need Start/Stop/Pause mapping + install/uninstall subcommands |
| 6 | Logging: file/broadcast only | Add Windows Event Log for unattended observability |
| 7 | Update flow assumes the GUI manages the helper | Service update needs SCM stop/start orchestration |

## 3. Target Architecture

```
┌─ SCM ────────────────────────────────────────────┐
│  wireguideplussvc (Windows service, LocalSystem)  │
│   ── the existing helper core, persistent,        │
│      no GUI dependency                            │
│   tunnel mgmt / WFP firewall / reconnect /        │
│   WiFi automation                                 │
└──────┬────────────────────────────────────────────┘
       │ named pipe (SDDL unchanged: SY/BA/user)
       ├──────────────┬───────────────────┐
       ▼              ▼                   ▼
   GUI client     ctl CLI (existing)   (optional) HTTP admin
   (Wails,         `wireguideplus ctl`   surface (reuse the
    no UAC spawn)                         -tags server idea)
```

**Component principle**: no new engine. The service = the existing
`helper.Run` wrapped in an `svc.Handler` shell; the GUI = the existing Wails
front end with `ensureHelper` changed to "dial the service, guide install if
missing"; the CLI is reused as-is.

## 4. Technology Choices

| Option | Description | Verdict |
|---|---|---|
| **A. `golang.org/x/sys/windows/svc`** | Official SCM support, zero extra deps; `svc.Run` + `Handler.Execute` for Start/Stop/Pause | ✅ Recommended. Windows-specific, precise control, consistent with the existing `x/sys/windows` dependency |
| B. `kardianos/service` | Cross-platform wrapper (win/mac/linux, one API) | Fallback. Consider only if unifying launchd/systemd management later |
| C. External wrappers (NSSM/winsw) | Wrap any exe as a service | ❌ External dependency; no SCM lifecycle events |

- **Service account**: `LocalSystem` (needs to write `%PROGRAMDATA%`, load
  the wintun driver, drive WFP, access the network). Do NOT use
  `NetworkService` (no driver loading / WFP rights).
- **Pipe name**: the service uses a global pipe, e.g.
  `\\.\pipe\wireguideplus` (distinct from `ipc.DefaultSocketPath()`); extend per
  user if multi-user lands (see 5.3).

## 5. Implementation Roadmap

### Phase 0 — Service shell (helper core untouched)

**Changes**
1. New package `internal/service/service_windows.go`:
   - Implement `svc.Handler`: `Execute(args, requests, changes)` loop handling
     `SVC_CONTROL_STOP` → `h.server.Shutdown()` (reuse the existing clean-exit
     path); `SVC_CONTROL_INTERROGATE` → report Running.
   - `Run()` builds the same dependencies as `helper.Run`
     (`tunnel.NewManager`, `firewall.NewPlatformFirewall`,
     `reconnect.NewMonitor`, `wifi.NewMonitor`) and enters the SCM control
     loop via **`svc.Run(serviceName, handler)`**.
2. `main.go`: add a `--service` branch — `svc.Run` first; if there is no SCM
   context, print a hint and exit.
3. New `ctl svc install|uninstall|status|start|stop` subcommands
   (`internal/cli/cli.go` switch):
   - `install`: `OpenSCManager` + `CreateService`
     (`SERVICE_AUTO_START`, `SERVICE_WIN32_OWN_PROCESS`, args
     `--service --data-dir <ProgramData>`)
   - `uninstall`: stop service → `DeleteService`
4. **NSIS integration** (`build/windows/nsis/project.nsi`): register the
   service silently on install; stop + delete on uninstall. Pick ONE of
   service-vs-`internal/autostart` HKCU Run key (remove the Run key once the
   service is installed).

**Acceptance**
- `wireguideplus ctl svc install && ctl svc start` → service visible in
  services.msc, startable/ stoppable.
- Stopping the service tears down tunnels + firewall rules (existing
  `cleanup()` path).
- GUI mode behavior unchanged (regression).

### Phase 1 — Lifecycle decoupling (the key change)

**Problem**: `internal/helper/helper.go`'s `startupGrace`/`shutdownGrace`
logic would make the service self-exit when no GUI is connected — exactly
what a service must not do.

**Changes**
1. Add a run-mode field to `Helper` (e.g. `mode`: `modeGUI` / `modeService`),
   passed in from `--service`.
2. In `armShutdownTimer`: under `modeService`, **skip all self-exit timers**
   (`OnConnect`/`OnDisconnect` still fire — they drive event push and
   `HasControlConn`, but never `shutdown()`).
3. `main.go` `--service` path calls `helper.Run(..., helper.WithMode(service))`
   or a new `helper.RunService(...)`.
4. Keep the "active tunnel keeps the helper alive" guard as a last line of
   defense when SCM Stop arrives (Stop still goes through `Shutdown`).

**Acceptance**
- Service runs with the GUI never opened; tunnels/WiFi automation work.
- GUI open → connect → close: the service stays up.
- `sc stop wireguideplussvc` stops cleanly.

### Phase 2 — Multi-user & Session 0

**Problem**: the service runs as LocalSystem in session 0; user config
(`storage.TunnelStore`) lives in each user's home. The current Windows
implementation (`deriveUserAppSupport`) relies on the `APPDATA` env var
passed through UAC — **an SCM-started service has no user environment**, so
paths must be explicit.

**Decision (recommended for v2.0 scope)**
- **Option A (recommended, small scope)**: move the config root to the
  system level `%PROGRAMDATA%\wireguideplus\` (incl. `tunnels/`,
  `config.json`); let the service read/write it directly. `storage.TunnelStore`
  gains a configurable root parameter (today it is `App Support/tunnels`).
  Single-config is fine for v2.0; evolve later.
- **Option B (full multi-user)**: per-user config dirs + per-user pipe names
  (`\\.\pipe\wireguideplus-<SID>`) + active-session context switching. High
  complexity — defer to 2.1.
- **GUI behavior**: detect the config location and migrate 1.x configs
  (copy to the new location).

**Changes**
1. `systemDataDir()` (`main.go`) under `--service` is fixed to
   `%PROGRAMDATA%\wireguideplus`.
2. `storage.TunnelStore` constructor gains a root argument; migrate existing
   `tunnels/`.
3. Pipe SDDL: under service mode, `ownerSID` semantics change — the service
   is owned by LocalSystem, so `verifyPeer` should accept "any locally
   logged-on user" (enumerate active sessions) instead of a single SID; or
   start permissive (BA members) and record a security review.

**Acceptance**
- After install, any administrator login can control tunnels via GUI/ctl.
- Tunnels survive lock/logoff; on re-login the GUI shows real state
  (event replay or status pull).

### Phase 3 — GUI client adaptation

**Changes**
1. `internal/gui/helper_lifecycle.go` `ensureHelper`:
   - Dial the service pipe first (new `ipc.Dial` to the service address).
   - On failure → prompt "service not running", offer one-click install/start
     (call `ctl svc install`, or a new `Svc.Install` RPC).
   - **Remove** the UAC spawn path (`elevate.SpawnHelper`) and the
     version-mismatch force-restart logic (service upgrades are managed by
     SCM — see Phase 5).
2. Version check: the service already reports `AppVersion` (`PingResponse`);
   when the service is older, prompt the user to run the updater instead of
   killing it.
3. `internal/autostart`: the Windows path becomes "ensure the service
   exists"; remove the HKCU Run key logic (macOS/Linux unchanged).

**Acceptance**
- Fresh machine: install → first GUI launch → guided service install → use;
  no UAC anywhere.
- GUI crash/kill: service and tunnels unaffected; reopening the GUI
  reconnects instantly.

### Phase 4 — Control surface & logging

**Changes**
1. New RPCs: `Svc.Status` (SCM state + tunnel state), `Svc.SetAutostart`.
2. Logging:
   - Keep `broadcastHandler` (GUI subscription) + `helper-stderr.log`.
   - Add Windows Event Log output
     (`golang.org/x/sys/windows/svc/eventlog`): service start/stop, tunnel
     connect/disconnect, firewall alerts, crash recovery.
3. SCM-side crash recovery: set `SERVICE_CONFIG_FAILURE_ACTIONS` on
   `CreateService` (failure → restart after 5s, max 3), complementing the
   existing `RecoverFromCrash`.

**Acceptance**
- `eventvwr` shows key service/tunnel events.
- Killing the service process → SCM auto-restart → tunnels recover (visible
  in logs).

### Phase 5 — Update & rollback

**Changes**
1. `internal/update` service adaptation: the GUI performs updates through
   "stop service via SCM → replace exe → start service"
   (`ctl svc stop` + file replace + `ctl svc start`) instead of killing the
   service directly.
2. Service self-update can lag (2.1): v2.0 keeps "GUI-triggered, SCM-managed
   restart".
3. Signature verification unchanged (Ed25519 + SHA256SUMS).

**Acceptance**
- Upgrade/downgrade keeps service and GUI versions consistent
  (`PingResponse.AppVersion`); tunnels reconnect across the update window.

### Phase 6 — Security hardening

1. Service account minimization: confirm the LocalSystem surface; if needed,
   split "service (LocalSystem) + worker (least privilege)" (the tailscaled
   model).
2. Re-audit pipe SDDL: `SY/BA` full access, regular users `GRGW` only
   (current definition already does this); verify no interactive-desktop
   injection surface from session 0.
3. Input validation: the service trusts no client (`maxConcurrentConns`,
   `verifyPeer`, `ReadDeadline` — keep and test all existing protections).
4. Uninstall safety: stopping the service can drop the kill switch — warn the
   user about network protection before stopping, then clean firewall rules.

## 6. Key Design Decisions

| Decision | Verdict | Rationale |
|---|---|---|
| Service account | LocalSystem | Needs wintun driver + WFP + ProgramData writes |
| Service stack | `x/sys/windows/svc` | Zero extra deps, full lifecycle events |
| Service = existing helper | Yes (shell it) | Engine is complete; avoid a rewrite |
| Multi-user | Option A (system-level config root) | Keep v2.0 scope bounded |
| GUI spawn | Remove UAC; guide service install | Eliminate per-session UAC |
| Logging | File + Event Log | Observability for unattended mode |
| Update | GUI-triggered, SCM-managed restart | Service self-update lands in 2.1 |

## 7. Test Plan

### Unit
- `armShutdownTimer` behavior in both modes (GUI regression / service persist)
- `storage.TunnelStore` new root parameter
- `ctl svc` subcommand parsing & error handling

### Integration (Windows real machine)
1. Fresh install → `ctl svc install` → service registered, `AUTO_START`
2. Boot without login → service running → GUI dials directly after login
3. Headless: WiFi-rule auto-connect, auto-reconnect after network drop (~30s)
4. Kill the service process → SCM auto-restart → crash recovery brings the tunnel back
5. `sc stop` / uninstall: tunnels down, firewall cleaned, no kill-switch residue
6. Upgrade: GUI triggers → stop → replace → start → versions match
7. Multi-user login switch: tunnels uninterrupted; second user's GUI state correct
8. Regression: macOS/Linux LaunchAgent/LaunchDaemon/systemd paths unaffected

### Manual
- Cover full GUI-mode regression with `docs/manual-test-checklist.md`.

## 8. Milestones & Deliverables

| Milestone | Content | Touch surface |
|---|---|---|
| M1 | Phase 0: service shell + install/uninstall + NSIS | `internal/service`(new), `main.go`, `cli.go`, `project.nsi` |
| M2 | Phase 1: lifecycle decoupling | `helper.go` (mode field + timer branch) |
| M3 | Phase 2: system-level config root + session 0 | `storage`, `systemDataDir`, `transport_windows.go` |
| M4 | Phase 3: GUI client adaptation | `helper_lifecycle.go`, `autostart` |
| M5 | Phases 4-6: control/logging/update/security | `ipc`, `update`, NSIS |
| M6 | Full test + release | Test plan + release flow |

**Release note**: 2.0 is a breaking upgrade — call out behavior changes
(service-based lifecycle, config-dir migration, UAC removal).

## 9. Risks & Notes

1. **Kill switch vs service lifecycle**: force-stopping the service
   (user/abnormal) briefly drops kill-switch protection — SCM recovery +
   existing crash recovery are the main lines of defense; test thoroughly.
2. **Config migration**: 1.x users have configs in `%APPDATA%`; a failed
   migration hides their tunnels. The migration must be idempotent and
   rollback-able.
3. **Multi-user simplification**: with Option A, multiple Windows accounts
   share one config — make it explicit in the UI ("single shared config") or
   add an account-isolation hint.
4. **Keep `--helper` mode**: macOS/Linux and Windows dev/debug still rely on
   the existing UAC helper path. Service-ization adds a new path rather than
   replacing it (evaluate removal after release).
