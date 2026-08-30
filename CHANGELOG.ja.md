# Changelog

All notable changes to WireGuide Plus will be documented in this file.

> 简体中文: [CHANGELOG.md](CHANGELOG.md) · English: [CHANGELOG.en.md](CHANGELOG.en.md) · 繁體中文: [CHANGELOG.zh-TW.md](CHANGELOG.zh-TW.md) · 한국어: [CHANGELOG.ko.md](CHANGELOG.ko.md)

## [1.1.2] - 2026-08-30

本リリースは Windows のファイルバージョン不一致を修正します。公開された 1.1.1 インストーラでは、実行ファイル（`wireguideplus-<arch>.exe`）の「ファイルのバージョン」が **1.1.0.1** と表示されていました（正しくは **1.1.1.0**）。

### 🐛 バグ修正

- **Windows 実行ファイルのファイルバージョン不一致を修正** — 根本原因: `goversioninfo v1.7` は `FixedFileInfo` 構造体を `Major/Minor/Patch/Build` の順序で宣言しており（Windows 標準レイアウトと Build/Patch が入れ替わっている）、JSON に数値を明示的に書くと入れ替わったバイナリバージョンになりました（`1.1.1.0` が `1.1.0.1` に）。`build/windows/versioninfo.json` の `FixedFileInfo` 数値は 0 に固定し、唯一の入力として `StringFileInfo` の4部構成バージョン文字列のみを使用し、goversioninfo がバイナリバージョンをそこから導出します（レイアウト非依存・常に一致）。`tools/genverinfo` は文字列バージョンのみをレンダリングし、`tools/bumpversion` は数値フィールドに触れません。検証済み: `1.1.2.0` 文字列を渡すと goversioninfo は `FixedFileInfo.FileVersion (1.1.2.0)` を出力し、インストール後はエクスプローラーのプロパティページと `FileVersionInfo` の両方が正しく表示されます。

### 🛠 内部

- バージョンを **1.1.2** に更新: `VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh` + `project.nsi`）、MSIX、Linux nfpm、macOS `Info.plist` をすべて同期。
- NSIS インストーラー/アンインストーラーの説明（`project.nsi`）を修正し、インストーラーとアンインストーラーのバージョン情報が出荷実行ファイルと一致するようにしました。

## [1.1.1] - 2026-08-30

本リリースは、システム高負荷時に Windows トレイ通知バブルの「メインウィンドウを開く」ボタンで発生する断続的な GUI フリーズを修正します。

### 🐛 バグ修正

- **通知バブルの「メインウィンドウを開く」が GUI を断続的にフリーズさせなくなりました** — CPU 競合が激しい状況（例: Windows メンテナンスプロセスがコアを占有）や WebView2 の遅延時、トレイ通知バブルの「Open Window」ボタンをクリックすると UI スレッドへの同期待ちが発生し、GUI 全体がフリーズしているように見えました（VPN トンネルは動作継続）。`showDock`（`internal/gui/dock_other.go`）は `application.InvokeAsync` 経由で Wails UI スレッド上で非同期実行されるようになり、呼び出し元は即座に戻り、ウィンドウの表示/フォーカスは UI スレッド内でインライン実行されるため、スレッド間待機は残りません。また、予期しない panic がメインスレッドのコールバックチェーンを壊さないよう recover ガードも追加しました。

### 🛠 内部

- バージョンを **1.1.1** に更新: `internal/update/checker.go` のメインバージョン、`build/config.yml`、`windows/info.json`（`1.1.1.0`）、`windows/wails.exe.manifest`、NSIS（`wails_tools.nsh`）、MSIX、Linux nfpm、`tools/genverinfo` をすべて同期。

## [1.1.0] - 2026-08-28

本リリースは識別性・プロキシの堅牢性・起動時の自動化ルール評価に注力しています。トレイの接続状態が高コントラストの文字グリフになり、プロキシは明確な3モード＋接続テスト付きになり、不正なプロキシ URL でも更新確認が失敗しなくなり、起動時は接続前に自動化ルールが評価されるようになりました。

### ✨ 新機能

- **トレイ状態グリフ** — Windows トレイメニューの接続状態をテキストグリフで表示: `●` 塗りつぶし = 接続中、`○` 白抜き = 切断（Windows トレイのポップアップは GDI で描画されるためカラー絵文字を表示できず、`🟢` はグレーの輪郭に劣化して新旧状態の区別が難しかった）; macOS メニューバー（ネイティブ AppKit 描画）はカラー絵文字を維持。起動/遷移状態には専用のマーカーを用意。
- **プロキシのモードと接続テスト** — 設定 → プロキシで3つの明確なモードを提供: **ダイレクト**（システム/環境プロキシを完全に無視）、**GitHub ミラー**（`mirror`、例: `https://ghfast.top` 加速プレフィックス）、**手動プロキシ**（`manual`、http/https/socks5 の完全な URL）。新設の**「接続テスト」ボタン**: 保存前に GitHub Releases API への往復リクエストを実行し、成功と遅延を報告します。
- **プロキシが即時適用** — プロキシ設定の保存後、次の定期更新確認（および手動の「今すぐ確認」）は再起動なしで適用。GUI 起動時にも保存済みプロキシを直接適用し、「起動直後に壊れた設定で1回確認してしまう」事態を回避します。

### 🐛 バグ修正

- **不正なプロキシ URL で更新確認が失敗しなくなりました** — `config.json` の壊れた手動プロキシ（例: `proxy_url = "https://"`）が従来 `http.ProxyURL` に直接渡され、毎回の更新確認が `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify must be specified in the tls.Config` で失敗していました。URL は起動時と使用のたびに検証されるようになり（`internal/update/proxy.go`）、不正な値は `WARN update: ignoring invalid manual proxy URL` を記録してダイレクト接続にフォールバック — 確認が失敗しなくなりました。
- **「まず接続してからルールで切断」という起動時の違和感を修正** — 起動時ルール評価をヘルパー起動直後に移動（ログ `startup rule re-evaluation`）し、各トンネルの目標状態をまずルールで決定します。さらに `scheduleRuleCheck` のフォールバックを追加: 起動後60秒以内は、RPC による手動接続（例: 前回セッションの復元）を3秒後にルールへ再評価して補正します（30秒ポーリングを待ちません）。トリガー元もトラブルシューティング用にログ出力されます。
- **不正なミラープレフィックスが黙って確認を壊すのを修正** — `mirror` モードの加速プレフィックスもスキーム/ホストを検証し、不正な値は公式 API エンドポイントにフォールバックします。

### 🛠 内部

- バージョンを **1.1.0** に更新: `internal/update/checker.go` のメインバージョン、`build/config.yml`、`windows/info.json`（`1.1.0.0`）、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm をすべて同期。
- **Windows バージョンリソースの標準化** — `wails3 generate syso` が生成するリソースは言語が `0x0000`、`VS_FIXEDFILEINFO.ProductVersion` がゼロで、Windows エクスプローラー / `FileVersionInfo` が読めませんでした（プロパティのバージョン欄が空白）。`goversioninfo`（設定: `build/windows/versioninfo.json`）に切り替えて標準の `0409/04B0` リソースを生成。`generate:syso` タスクも更新され、exe とインストーラーのプロパティが正しく `1.1.0` を表示するようになりました。
- **Windows x86（32ビット）ビルドを新設** — `task windows:build ARCH=386` で32ビットバイナリと `wireguide-x86-installer.exe` インストーラーを生成します（NSIS スクリプトは x86 対応、Program Files にインストール、x86 版 `wintun.dll` を同梱）。
- **プラットフォーム境界を明確化** — iOS ビルドタスクと設定コメントを削除。Android / iOS は非対応（同時マルチトンネル不可、SSID ベースの自動接続不可）。README に明記。macOS / Linux 強化版は開発中。
- **システム統合の強化** — 新設の**「最小化で起動」**設定（メインウィンドウを表示せず直接システムトレイへ; 設定 → 起動）。新設の**トレイ接続通知**: 起動10秒後、およびネットワーク変更（Wi-Fi 切替、ケーブル抜去、ネットワーク喪失など）でトンネル接続状態が変わった10秒後に、安定した最新状態を表示します。バブルはアクションメニュー付き（メインウィンドウを開く / 切断）、手動で閉じるか、設定した表示時間（既定10秒、設定 → 起動 → 通知の表示時間で変更可; `internal/gui/notify_windows.go`）で自動的に閉じます。
- **デュアルアーキテクチャリリース** — すべてのビルドで32ビット（x86）と64ビット（amd64）の両バイナリとインストーラーを生成（`task windows:build:all`、wintun.dll のアーキテクチャ自動更新付き）。アプリ/インストーラーの説明を「マルチトンネル + 自動化」に統一し、クロスプラットフォームの文言を削除。
- **インストール体験** — インストーラーは Program Files が既定（32ビット版は Program Files (x86) を自動選択）、インストール中に変更可能。スタートメニューのショートカット（「WireGuide Plus のアンインストール」を含む、アイコンはアプリと同一）を既定で作成し、「ショートカットオプション」ページでオフにできます。デスクトップショートカットは常に作成されます（`build/windows/nsis/project.nsi`）。
- **開発・リリースドキュメント** — ビルド/パッケージングのドキュメントを README から独立した `docs/DEVELOPMENT.md` に移動。GitHub Release ワークフローに32ビット Windows アーティファクトと CI ツールチェーン（`goversioninfo`）を追加。ローカルの `v*` タグをプッシュすると（Windows x86+amd64、macOS arm64、Linux amd64+arm64）自動でビルド・署名・公開されます（`docs/release.md`）。
- Windows アダプター名マッチングを調整（`internal/wifi/known_windows.go`、`detect_windows.go`）し、物理アダプター検出をより正確に。
- ウィンドウタイトルを **WireGuide Plus** に統一。
- 更新確認をスケジューラー内で重複排除し、1ラウンドで複数回トリガーされないように（失敗は1回だけログ、リトライ間隔付き）。

## [1.0.0] - 2026-08-28

マイルストーンリリース: A11y アクセシビリティのセマンティックリファクタリング、Windows のネットワーク出口ルーティングロジックの変更、Wails3 のビルド/アイコン/権限の整理、さらに简体中文 UI とトレイトグルを追加。

### ✨ 新機能

- **简体中文 UI（中国語 UI）** — 全199文字列をカバーする完全な简体中文翻訳: トンネル一覧、履歴、ツール（DNS リークテスト / ルートテーブル）、ログ、設定、更新、自動化エディター。初回起動時はシステム言語を自動追従（`zh-*` ロケールを検出）、または設定 → 一般 → 言語で手動切り替え（永続化）。
- **トレイトグル** — システムトレイの各トンネルが独立したクリック可能なトグルに: チェックで接続、チェック解除で切断。接続絵文字（🟢 接続中 / 🟡 接続中 / ○ 切断）はラベルの横に維持されます。手動で切断したトンネルは、再接続または WireGuide の再起動まで自動化ルールの対象外（manual-off）のままです。

#### フロントエンド A11y アクセシビリティリファクタリング

> 対象: 全プラットフォーム（Windows/macOS/Linux）の Svelte フロントエンド。Windows だけではありません。

- すべてのモーダルオーバーレイからスクリムの `role="button"` と `tabindex="0"` を削除し、純粋なマスクに戻しました — スクリーンリーダーが全画面背景を操作可能なボタンと誤認しなくなります。
- すべてのダイアログは `tabindex="-1"` を使用し、標準の `role="dialog" aria-modal="true"` を維持（WCAG ダイアログセマンティクスに準拠）。
- ESC クローズを統一: 未対応だったダイアログ（インポート結果、履歴、更新通知、自動化エディター）は、コンポーネント最上位で `<svelte:window on:keydown>` をマウントします（ハンドラーがダイアログ状態をチェック。Svelte では `{#if}` 内にマウントできないため）。残りは App.svelte のグローバルキャプチャハンドラーを再利用 — CodeMirror のキーキャプチャを壊さずに複数ダイアログの ESC 競合を回避します。
- `Settings.svelte`: `<nav role="tablist">` をプレーンな `<div>` に置き換え、タブセマンティクスの警告を解消。ペインリサイザーは `role="separator"` を維持しつつ `tabindex="0"` と実キーボード操作（矢印キーでリサイズ、Enter/Space でリセット）を追加。
- `frontend/vite.config.js`: svelte プラグインの `onwarn` で静的誤検知（`a11y_click_events_have_key_events`、`a11y_no_static_element_interactions`、`a11y_no_noninteractive_tabindex`、`a11y_no_noninteractive_element_interactions`）をフィルタ。本番ビルドの警告はゼロに、ロジック変更なし。
- 変更ファイル: `src/App.svelte`、`src/lib/History.svelte`、`src/lib/ConflictWarning.svelte`、`src/lib/TunnelDetail.svelte`、`src/lib/UpdateNotice.svelte`、`src/lib/Settings.svelte`、`src/lib/AutomationEditor.svelte`

#### Windows バックグラウンドヘルパー: ネットワーク出口ルーティングロジック

> 対象: Windows 専用の Go ヘルパーコード。他 OS は影響を受けません。

- 起動時に主要アップストリーム物理アダプターの LUID を取得し、システム初期の既定出口物理インターフェースを記録します。この LUID は起動時スナップショットであり、実行中のネットワーク切り替えでは自動更新されません。
- ネットワークインターフェースのフィルタリングを修正: TUN/トンネル/ループバック仮想アダプターを除外し、物理アダプターのみをアップストリーム候補に。TUN 仮想アダプターを物理としてバインド/ロックしなくなりました。
- WireGuard UDP 出口は完全に Windows ルーティングテーブル + インターフェース別 InterfaceMetric ホップ数に委譲。固定物理アダプターへの強制バインドを廃止。
- スプリットトンネル（`full_tunnel=false`）の制約を追加: Peer Endpoint IP を `AllowedIPs` に明示的に含める必要があり、ハンドシェイク UDP パケットのルートドロップ（`no-handshake`）を防止します。
- ログ: `network primary upstream interface initial luid` がトラブルシューティング用に主要物理アダプターの LUID を出力。ログの `tunnel connected` は TUN アダプターが準備できたことのみを意味し、リモートピアのハンドシェイク成功を意味しません。
- トラブルシューティングのヒント: Windows では `Find-NetRoute -RemoteIPAddress <peer-ip>` で対象 IP の実出口アダプターを特定。PowerShell の `Get-NetAdapter.Luid` は構造体のため Go の uint64 出力と直接比較できません。

### 🛠 ビルドとプロジェクト

ほとんどが Windows のビルド挙動です。クロスプラットフォーム部分は明記しています。

1. **Wails3 Windows アイコン生成の挙動**（Windows のみ）— `task build` のフルビルドは自動的に `wails3 generate icons` を実行し、`build/appicon.png` を読み込んで `windows/icon.ico` を上書きします。手動編集した `windows/icon.ico` はフルビルドで上書きされます。`windows/icon.ico` が最終的に exe に埋め込まれるアイコンで、`build/appicon.png` はソースアセットにすぎません。`task windows:build` のデバッグビルドはアイコン生成をスキップし既存の `windows/icon.ico` を維持します。exe / ウィンドウタイトルバー / タスクバーのアイコンは exe 内の ico リソースを再利用。システムトレイアイコンには別途 Go の `embed` リソースが必要です。
2. **Windows バージョン情報の管理**（Windows のみ）— exe のファイル詳細は `windows/info.json` から取得。`FileVersion` は4部構成の数字 `major.minor.patch.build` である必要があります。UI 表示のバージョンは Go 定数（`internal/update/checker.go`）で管理され、`info.json` と手動で同期する必要があります。将来は ldflags によるビルド時注入で単一のバージョンソースにできます。
3. **Windows UAC / 管理者権限**（Windows のみ）— 現在のアーキテクチャ: GUI がヘルパーサブプロセスを起動します。ヘルパーは TUN アダプターの操作とルート変更を行うため管理者権限が必要で、サブプロセスの昇格は UAC プロンプトを引き起こします — Windows のセキュリティを黙って完全に回避することはできません。短期: `windows/wails.exe.manifest` に `requireAdministrator` を追加し、UAC プロンプトを exe のダブルクリック起動時に移動（それでもユーザー確認が必要）。長期: ヘルパーを Windows システムサービス（LocalSystem、バックグラウンド）にリファクタリングし、GUI は通常ユーザーとして IPC で通信 — UAC プロンプトを完全に排除。

### 🐛 調査

調査メモ、コード変更なし、開発者向け参照用。

- 症状: ヘルパーが `tunnel connected` とログするのに GUI が `no handshake` と表示。
  - 根本原因: TUN デバイスの作成 ≠ リモートピアとの WireGuard 暗号化ハンドシェイク完了。wg カーネルの `latest handshake` 状態を読んで実際の接続性を判断します。
  - スプリットトンネルの落とし穴: Peer IP が `AllowedIPs` にない → ハンドシェイク UDP パケットがルートドロップされます。
  - その他の可能性: Windows の外向きファイアウォールが WireGuard UDP をブロック、エンドポイントドメインの DNS 解決失敗。
- `0.0.0.0` で待ち受けるローカルプロキシ: プロキシプロセスのトラフィックは独立しており、自動的に WireGuard トンネルに流れません。トラフィック方向は Windows ルーティングテーブルとトンネルの `AllowedIPs` が共同で決定します。

### 📝 注記

1. **変更の範囲**
   - Svelte フロントエンドの A11y コード: **全プラットフォーム（Windows / Linux / macOS）に適用** — ESC 処理とアクセシビリティセマンティクスはすべてのデスクトッププラットフォームに影響します。
   - ヘルパーのネットワーク出口ルーティング: **Windows 専用の Go コード変更**。他 OS は影響なし。
   - ビルド、manifest、ico、info.json、UAC: **Windows のみ**。
2. フロントエンド A11y の変更はヘルパーのバックグラウンドネットワークロジックから完全に分離されており、トンネル作成・ルーティング・自動化 Wi-Fi ルールに影響しません。
3. ヘルパーが記録するアップストリーム LUID は起動時スナップショットのみで、Wi-Fi / 有線の切り替え時には自動更新されません。

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
