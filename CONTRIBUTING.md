# Contributing to WireGuide Plus

Thanks for your interest in contributing!

## Development Setup

### Prerequisites

Versions below are the ones **GitHub Actions actually uses** — they are the
contract, not a suggestion. Local dev may run newer, CI will not.

| Tool | CI pin (authoritative) | Local floor | Notes |
|---|---|---|---|
| Go | `1.25.12` (`ci.yml`, `release.yml`) | 1.25.12 | `go.mod` declares the same. 1.26.x builds locally but is **not** what CI compiles. |
| Node.js | `20.19.2` (`ci.yml`, `release.yml`) | 20.19.0 | Vite 8 requires `^20.19.0 \|\| >=22.12.0`. Plain "Node 20" is **not** enough: 20.0–20.18 fails to start Vite. |
| [Task](https://taskfile.dev/) | `v3.45.4` (installed in both workflows) | v3.45.4 | `go install github.com/go-task/task/v3/cmd/task@v3.45.4` |
| [Wails v3](https://v3alpha.wails.io/) | `v3.0.0-alpha.74` (installed in both workflows) | same | Must equal `go.mod` (`github.com/wailsapp/wails/v3 v3.0.0-alpha.74`). CLI and module are version-locked; a mismatch fails with confusing template errors. |
| [goversioninfo](https://github.com/josephspurrier/goversioninfo) | `v1.4.0` (`release.yml`, Windows job only) | same | Invoked directly by Taskfile `generate:syso`. Missing binary silently ships a build without version metadata — it is installed explicitly rather than assumed present. |
| GitHub Actions | **exact patch tags only** (`actions/checkout@v5.1.0`, never `@v5`) | same | Floating major tags resolve to whatever upstream moved them to; one such roll-up broke a release build and had to be frozen at `softprops/action-gh-release@v3.0.3`. Third-party actions therefore carry an exact patch, except `signpath/github-action-submit-signing-request`, which is pinned by commit SHA with its version in a trailing comment. `.github/dependabot.yml` (monthly, `github-actions` only) opens PRs when upstream publishes a newer patch; upgrading a whole major is still a deliberate human decision. |

- macOS (Apple Silicon), Windows 11, or Linux — all three are supported build/dev hosts

> **Version bumps:** `go.mod` and **every file in `.github/workflows/`**
> (`ci.yml`, `release.yml`, `fix-release-notes.yml`, `no-ai-scan.yml`) must move
> together with this table. In practice that means: `go-version`,
> `node-version`, the three `go install` steps (`wails3`, `task`,
> `goversioninfo`), and every action pin (`checkout`, `setup-go`, `setup-node`,
> `upload-artifact`). Bumping only one of them is the usual
> cause of "works on my machine, fails in CI".

### Build & Run

```bash
# Install frontend dependencies
cd frontend && npm ci && cd ..

# Development mode (hot reload)
task dev

# Production build
task build
```

### Publish-hygiene git hooks

Enable the hooks **once per clone** (`core.hooksPath` is a local git setting and
cannot be committed, so this is a manual step):

```bash
sh scripts/setup-hooks.sh
```

It points `core.hooksPath` at `scripts/git-hooks/`, which installs **two**
hooks — both are needed, because they see different things:

| Hook | What git passes it | What it scans |
| --- | --- | --- |
| `pre-commit` | *nothing* (git passes no arguments) | the staged diff |
| `commit-msg` | the message file path (`$1`) | the commit message, plus the staged diff |

The split is not cosmetic. A `pre-commit` hook is invoked *before* the commit
message is obtained, so `check-no-ai.sh "$1"` there would always pass an empty
string and the guard would silently fall back to scanning only the diff — which
is exactly how a message like `test WorkBuddy leak` used to slip through. Only
`commit-msg` can see the message.

Between them they block commit messages or published files that contain
assistant-tool names or the standalone `AI` token. See
`scripts/check-no-ai.sh` for the exact rules and its false-positive guards
(CSS `cursor`, the legacy public-resolver description in the changelogs, the
guard's own `*-no-ai-*` filenames, and ignore files are all deliberately
excluded).

The identical rule is enforced server-side by
`.github/workflows/no-ai-scan.yml`, so pushes stay protected even on a machine
where the local hooks were never enabled.

### Project Structure

- `internal/helper/` — Privileged daemon (runs as root), Automation evaluation
- `internal/tunnel/` — WireGuard engine and connection phases
- `internal/gui/` — Wails app, tray, event bridge
- `internal/app/` — GUI-side services bound to the frontend
- `internal/network/` — Platform-specific network config
- `internal/firewall/` — 防火墙后端（macOS `pf` / Linux `nftables` / Windows WFP）：当前只服务于**按隧道**的 System DNS 强制（`EnableDNSProtection`）；全局 kill switch 开关已从产品中移除，仅保留平台原语
- `internal/wifi/` — Automation rule model, network fingerprinting
- `internal/ipc/` — JSON-RPC 2.0 transport (Unix socket / named pipe)
- `internal/cli/` — `wireguideplus ctl` command-line interface
- `internal/update/` — Update checker and Ed25519 release verification
- `frontend/` — Svelte UI

## Pull Requests

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Run `go vet ./...` and `go test ./...` locally (CI runs the same on a Linux/macOS/Windows matrix for every PR)
4. Open a PR with a clear description of what and why

Keep PRs focused — one fix or feature per PR.

## Issues

Found a bug? Have a feature idea? Open an issue using the templates provided.

## Code Style

- Follow existing patterns in the codebase
- `go vet` and `go build` must pass with no errors
- Frontend: follow existing Svelte conventions
