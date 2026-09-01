# Changelog

All notable changes to WireGuide Plus will be documented in this file.

> 简体中文: [CHANGELOG.md](CHANGELOG.md) · English: [CHANGELOG.en.md](CHANGELOG.en.md) · 繁體中文: [CHANGELOG.zh-TW.md](CHANGELOG.zh-TW.md) · 한국어: [CHANGELOG.ko.md](CHANGELOG.ko.md)

## [1.4.1] - 2026-09-02

### 🐛 修正

- **Windows：アプリ内アップデート後にアプリを自動起動** — 更新完了後にアプリが自動的に再起動されるようになりました。従来はサイレントインストーラーが完了ページ（「今すぐ実行」チェックボックスがある唯一の場所）をスキップするため、アップグレード完了後にウィンドウが表示されませんでした。アップデータ専用フラグ `/AUTOSTART` を検出したインストーラーが、UAC の管理者トークンではなくユーザートークンで新バージョンを起動します。

## [1.4.0] - 2026-09-02

macOS（Homebrew 以外）のアプリ内アップデートは Windows / Linux と完全に同等になりました。ブラウザに飛ばず、アプリ内で新バージョンをダウンロードして上書きインストールできます。

### ✨ 新機能

- **macOS アプリ内上書きインストール** — Homebrew 以外でインストールされた macOS ユーザーが「更新」を押すと、アプリ内でインストーラー（.dmg、予備として .zip）を「設定 → 更新」で構成したミラー / プロキシ経由でダウンロードし、SHA256 と Ed25519 署名を検証したうえで、コード署名（`codesign --verify`）も確認してから自動で置き換え・再起動します。アプリはどこから起動していても（`/Applications`、`~/Applications`、任意のフォルダ）その場で上書きされ、Dock アイコンや Finder 上の位置は保持されます。システムディレクトリへのインストールでは macOS のパスワードダイアログが出ます（Windows の UAC や Linux の polkit と同じ体験）。quarantine 属性も自動で除去し、Gatekeeper にブロックされません。Homebrew でインストールした macOS ユーザーは引き続き `brew upgrade` を使います。
- **ミラー / プロキシが全プラットフォームをカバー** — macOS のアプリ内アップデートのダウンロードは、Windows / Linux と同じく「設定 → 更新」で構成したミラー（GitHub 高速化）またはローカルプロキシを経由し、ブラウザに一切依存しません。

### 🐛 修正

- **macOS メニューバーの整理** — Wails デフォルトメニューバーの Help → Learn More が WebView 自体を wails.io に遷移させ、GUI に戻る手段がなくなる問題を修正：Learn More はシステムの既定ブラウザで GitHub プロジェクトページを開くようになり、WebView は一切遷移しません。実用性のない File / Edit / View / Window メニューも削除し（ズーム・フルスクリーン・最小化はアプリ画面とタイトルバーで操作可能）、App とカスタム Help のみを残します。Windows / Linux はこのメニューバーを表示しないため影響を受けません。

### 🛠 内部

- `internal/update/installer_darwin.go` を追加：macOS アプリバンドルのその場置き換えインストーラー（dmg マウント / zip 展開 → コード署名検証 → 特権スクリプトで killall + 置き換え + quarantine 除去 + 再起動）。Windows / Linux インストーラーと同じダウンロード・検証・進捗パイプラインを共有します。

## [1.3.7] - 2026-09-01

### 🐛 修正

- **Windows アプリ内アップデートのインストーラー起動失敗** — PowerShell `Start-Process -Verb RunAs` を Windows ShellExecute `runas` に置き換え、PowerShell 実行ポリシー/パス問題による `exit status 1` を回避。さらに、ダウンロードしたインストーラーを `%LOCALAPPDATA%\wireguideplus\updates` の永続ディレクトリにコピーし、UAC 確認後にインストーラーが exe を見つけられなくなるのを防ぐ。
- **Linux アプリ内アップデートの堅牢性** — 更新アセットも起動前に永続ディレクトリ（`$XDG_DATA_HOME/wireguideplus/updates`）へステージングし、AppImage の非同期起動と一時ファイル削除の競合を解消。拡張子判定を大文字小文字非依存にし `.AppImage` に対応。`.tar.gz` などの未知形式を実行ファイルとして起動せず、明確に失敗してダウンロードページにフォールバック。`pkexec` 失敗時は出力を保持し、polkit エージェント欠落の診断を容易にする。

## [1.3.6] - 2026-09-01

このバージョンでは、リネーム前（"wireguide"）データの移行を**ユーザーが見える対話的なフロー**に変更しました。起動時に旧バージョンの設定・トンネル・ログをスキャンし、何を移行するか・名前の競合をどう扱うかを選択でき、新旧フォルダーを比較してから実行できます。静かな上書きは廃止です。

### ✨ 新機能

- **旧データ移行ダイアログ** — 起動時にリネーム前の "wireguide" ディレクトリ内の config.json、history.json、tunnels/*.conf、ログを自動検出。ダイアログではカテゴリ別の件数と名前の競合を表示し、ワンクリックの「すべて移行」で旧ディレクトリを整理して状態を記録します。
- **新旧フォルダーの比較** — ダイアログから旧 / 現在の設定・ログフォルダーを直接開き、移行前に内容を確認できます。
- **移行オプション** — 名前の競合時は「既存ファイルを上書き」にチェック可能。ログは既定では移行せず、必要に応じて有効化します。
- **後で通知・再通知しない** — 「次回起動時に再通知」はダイアログを閉じるだけで何も記録せず、次回起動時に再検出します。「再通知しない」は選択を永続化してダイアログを表示しなくなり、設定 → 詳細 → 旧データからいつでも再スキャンできます。

### 🎨 UI 改善

- **テーマ化スクロールバー** — グローバルのスクロールバーをテーマトークンで描画（細いスクロールバー＋角丸スライダー）。長いリスト（トンネル・ログ・履歴）が Windows WebView の標準スタイルに戻らなくなります。
- **ダイアログのトークン化** — 更新通知などのポップアップをテーマ変数（`--overlay-bg` / `--bg-card` / `--shadow-md`）に統一し、手書きのダークモード media query を削除。ライト/ダークで一貫した見た目になります。
- **AWG バッジのテーマ統一** — トンネル一覧・詳細の AmneziaWG バッジをハードコード色から `var(--purple)` トークンに変更し、両テーマで自然に表示されます。

### 🛠 内部

- 初回起動時の静かな自動移行（従来 `GetPaths` 内で実行）を廃止し、明示的な `DetectLegacyData` / `MigrateLegacyData` の対話フローに置き換え。CLI コマンドは自動移行せず、初回 GUI 起動が案内を担当します。

## [1.3.5] - 2026-09-01

このバージョンでは **AmneziaWG（AWG）プロトコル対応**を追加しました — DPI による識別を回避する難読化 WireGuard フォークです。AWG 設定は難読化パラメータ（Jc/Jmin/Jmax/S1-S4/H1-H4）から自動検出され、amneziawg-go バックエンドで動作し、UI 上は「AmneziaWG」バッジで表示されます。AWG バックエンドが不安定なマシン向けに、設定 → 詳細でサポートを無効化できます。

### ✨ 新機能

- **AmneziaWG（AWG）プロトコル対応** — AmneziaWG 設定のインポート・接続に対応。Jc/Jmin/Jmax/S1-S4/H1-H4 キーから自動検出し、一覧・詳細画面に「AmneziaWG」バッジを表示。ハンドシェイク / トラフィックなどの状態表示は WireGuard トンネルと同様です。
- **AmneziaWG 有効化設定** — 設定 → 詳細に「AmneziaWG サポートを有効化」スイッチを追加（既定オン）。オフにすると AWG トンネルの接続は明確なエラーで拒否され、接続途中の失敗になりません。

### 🛠 内部

- amneziawg-go ベースの新プロトコルバックエンドをエンジン抽象層に統合 — 両プロトコルが同一の接続パイプラインを共有。AWG の状態は常にプロセス内 UAPI で提供。Windows のソケット固定は両バックエンドで有効。

## [1.3.1] - 2026-09-01

このバージョンでは、Windows のアプリ内アップデートでインストーラー起動時に UAC 昇格を要求しなかった問題を修正し、macOS が Apple Silicon 実機で検証済みであることを正式に記載しました。

### 🐛 修正

- **Windows インストーラーの UAC 昇格** — アプリ内アップデートでインストーラーを起動する前に昇格コンテキストを要求するようにし、手動ダブルクリックの動作と一致させました。

### 🛠 内部

- **macOS 実機検証** — プラットフォームサポートに macOS（Apple Silicon）が実機で検証済みであることを明記。

## [1.3.0] - 2026-09-01

このバージョンでは、アプリを **WireGuide Plus** に全面リブランドしました：ウィンドウタイトル、トレイ、自動起動項目、helper ログ、Homebrew cask、更新一時ファイル、nftables テーブル名など、全経路が plus 命名に統一され、アップグレード時に旧バージョンが残した起動項目・デーモン・ファイアウォールテーブルも自動的に掃除されます。さらに macOS のメニューバーアイコンとルーティング診断表示も改善しました。

### ✨ 新機能

- **macOS メニューバーアイコンをアプリアイコンに変更** — アプリアイコンの赤いバリアントを使用し、ライト / ダークどちらのメニューバーでも視認性を確保。アイコンが埋め込まれていない場合は従来のモノクロ W テンプレートにフォールバックします。
- **macOS ルート診断の正規化** — `netstat -rn` はネットワークルートを圧縮表示します（127.0.0.0/8 は "127"、192.168.1.0/24 は "192.168.1"）。診断ページで正規のドット付き 10 進数 + プレフィックスに展開して表示します。

### 🛠 内部

- **WireGuide Plus への全面リブランド**：macOS 自動起動 `com.wireguideplus.gui`、LaunchDaemon と helper ログパス、pf anchor `com.apple/wireguideplus`、Linux デスクトップアイコン、Windows 自動起動レジストリ値、wintun アダプター名 `WireGuidePlus-<hash>`、FWPM セッション / Provider / SubLayer 名、nftables テーブル `wireguideplus` / `wireguideplus_dns`、Homebrew cask `wireguideplus` と Caskroom パス、更新一時ファイルと競合検出ソケットパス、リリース機の鍵ディレクトリ `~/.wireguideplus`、テスト環境変数 `WIREGUIDEPLUS_RESOURCE_*`、macOS の認証プロンプト文言をすべて統一。
- **アップグレード時のクリーンアップ**：旧バージョンが残した `com.wireguide.gui` LaunchAgent、`com.wireguide.helper` LaunchDaemon と helper、旧 helper ログ、旧 pf anchor `com.apple/wireguide`、`wireguide.desktop`、旧 wintun アダプター `WireGuide-<hash>`、旧 nft テーブル、旧 FWPM Provider を削除します。
- **リリース成果物のリネーム**：macOS zip / dmg と Linux deb を `WireGuidePlus-*` に変更。NSIS の PATH ヒントと MSIX テンプレートの実行ファイル名も同期しました。
- **テストスクリプト同期**：systemd unit とテスト用 socket を `wireguideplus-*` プレフィックスに統一しました。

## [1.2.5] - 2026-09-01

本バージョンでは DNS リークテストを再構築し、新たに「パブリック DNS クロスチェック」を追加しました：ローカル設定のリゾルバに加えて、既知のパブリック DNS にもクエリを送って突き合わせます。システムリゾルバは取得元インターフェースごとに「ローカル / VPN / パブリック」に分類・表示し、パブリックリストはネットワークからの更新と自由な編集に対応しました。「ブラウザーでテスト」ボタンから browserleaks.com を開いてブラウザーレベルの DNS / WebRTC リークチェックもできます。Windows の接続通知ポップアップがフリーズする問題も修正し、アプリアイコンも一新しました。

### ✨ 新機能

- **パブリック DNS クロスチェック** — ローカル設定の DNS に加えて、既知のパブリックリゾルバ（Google、Cloudflare、OpenDNS、Quad9、Alibaba、Tencent DNSPod、114DNS、Baidu、AdGuard、NextDNS、Comodo、および一般的な IPv6 アドレス）にもプローブを送り、DNS クエリがトンネル内のみを経由しているかを突き合わせます。パブリックリゾルバの応答は「到達可能」を意味するだけで、リークではありません。
- **ネットワークからのリスト取得** — 「ネットワークから取得」で public-dns.info から現時点で最も信頼できるリゾルバ（最大 30 件、10 秒タイムアウト）を取得し、前回成功したリストもキャッシュするためオフラインでも利用できます。
- **パブリックリゾルバリストのカスタマイズ** — エントリ（IP またはホスト名）を自由に追加 / 削除 / 編集でき、設定に保存されます。リストを空にすると既定のクロスチェックリストに戻り、パブリックプローブは常に有効です。
- **システムリゾルバの分類表示** — 取得元インターフェースで分類：物理アダプター（無線 / 有線）は「ローカル」、トンネルインターフェースは「VPN」、それ以外は「パブリック」。ローカルが先頭に表示され、取得元インターフェース名も表示します（Windows はアダプターごとの DNS を列挙、Linux は resolvectl 出力を解析）。
- **ブラウザーでテスト** — 「ブラウザーでテスト」ボタンで既定のブラウザーを開き、browserleaks.com でブラウザーレベルの DNS / WebRTC リーク検出を実行します（プローブデータは第三者サイトに送信されます）。

### 🐛 修正

- **Windows 通知ポップアップのフリーズ** — ポップアップのメッセージループが、それを生成した OS スレッドに固定されておらず、goroutine がスレッド間を移動するとクリック / クローズ / タイマーメッセージが届かずフリーズしたように見えました。ポップアップの生存期間中スレッドをロックすることで、正常に閉じられるようになりました。
- **ポップアップ文字描画の堅牢化** — 文字描画を `UTF16FromString` に切り替えてエラーを処理し、不正な UTF-16 文字列によるクラッシュを防ぎました。

### 🛠 内部

- CLI `dnsleak` コマンドも同期強化：リゾルバ行に `vpn / local / public` タグとステータスを表示し、設定のカスタムパブリックリストを使用します。
- リーク判定を修正：物理（非 VPN）リゾルバが応答した場合のみリークと判定。VPN リゾルバは VPN 状態、パブリックリゾルバの応答は「正常」（リークではない）と表示。
- DNS リークモジュールにプローブプラン / 解析のテストを追加し、bindings を再生成。
- 各プラットフォームでアプリアイコンを更新し、ビルドタスクを簡素化。

## [1.1.10] - 2026-08-31

このリリースでは 1.1.9 で報告された 3 つの画面の問題を修正し、設定操作を改善します：DNS リークテスト画面の幅制限を撤廃して本機 DNS をマークし、ログのレベル絞り込みを完全一致にし、通知時間とプロキシ選択の保存・表示を修正しました。カスタムミラー / ローカルプロキシの入力欄は最後に保存したアドレスを記憶します。

### 🐛 修正

- **DNS リークテストの幅** — 640px の最大幅制限を削除し、「履歴」「ルート」と同じくウィンドウに合わせて拡大表示。
- **本機 DNS マーク** — テスト対象はすべてシステムの DNS 設定（手動または DHCP）由来のため、各行に「システム」タグを表示し、VPN 提供 DNS と区別しやすくしました。
- **ログレベルの絞り込み** — DEBUG / INFO / WARN / ERROR ボタンをクリックするとそのレベルの記録だけを表示（従来は「指定以上」で、該当レコードがないと絞り込みが効いていないように見えました）。
- **通知時間の設定** — ドロップダウンを「ログ保持 / 履歴保持 / 言語」と同じ動的オプション方式に変更し、変更が正しく保存され再表示されるようにしました。
- **プロキシモードの表示** — 設定画面を開き直すと「直接」に戻っていた問題を修正（Svelte は関数内で参照するフィールドを追跡できないため `value={関数()}` は初回のみ評価されていました）。リアクティブ再計算により保存したミラー / 手動モードが表示されます。
- **プロキシアドレスの記憶** — 「カスタムミラー」やローカルプロキシに戻したとき、最後に保存したアドレスを入力欄に自動復元（過去に保存したミラープレフィックスなど）。履歴がない場合は空欄とヒントを表示。

## [1.1.9] - 2026-08-31

このリリースではアプリ内更新が「ダウンロード成功後にインストールできない」問題を修正します：更新処理がインストーラー起動前に一時ダウンロードファイルを削除していたため、Windows でインストーラーの起動時に「ファイルが見つかりません」となり、リリースページにフォールバックしていました。

### 🐛 修正

- **アプリ内更新でインストールできない** — `runUpdateNative` は `Install` の前に `os.Remove(path)` で一時ダウンロードしたインストーラーを削除していましたが、Windows のインストール処理はそのファイルを直接実行するため（`fork/exec …wireguide-update-*.exe: The system cannot find the file specified`）、ダウンロード 100% の後に必ず起動に失敗していました。インストーラー起動後に一時ファイルを解放するよう修正。Windows ではインストーラー実行中はファイルがロックされ削除に失敗することもありますが、OS が %TEMP% を自動で掃除するため無害です。
- **一度手動アップグレードが必要** — 1.1.7 / 1.1.8 の更新処理にも同じ問題があるため、それらのバージョンからのアプリ内更新は依然失敗します。1.1.9 は一度手動でインストールしてください（設定 → 更新 → リリースページを開く）。以降はアプリ内更新が正常に動作します。

## [1.1.8] - 2026-08-31

このリリースでは自動化ルールの判定セマンティクスとエディターの案内を揃え、旧形式ルールの扱いをさらに堅牢にします：ルールは上から順に評価され最初に一致したものが適用され、同じ動作の条件は OR 関係、それ以外（otherwise）は最後に置くフォールバックで、通常は上のルールと逆の動作にします。条件タイプのない旧形式ルールが無駄な再読み込みを引き起こすこともなくなりました。

### ✨ 改善

- **自動化ルールの意味ガイドを統一** — エディターのヒントと「それ以外」行の説明を更新：「それ以外」は上のどのルールにも一致しないときに働き、最後に置いてフォールバックとして使い、動作は通常上のルールと逆（5言語すべて同期）。判定ロジック自体は変更なし：順次評価・最初一致、`none_match` は無条件マッチ——ご期待の動作と一致しています。

### 🐛 修正

- **旧形式ルールで無駄な再読み込みが起きないように** — ディスクとローカルの比較時に読み込みと同じ型推論を使用（`type` のない旧「それ以外」ルールを network に回帰させない）。設定変更のたびに外部編集と誤判定して余分な再読み込みが発生する問題を修正。

### 🛠 内部

- bindings を再生成し、Go API と完全一致することを確認（差分なし）。
- バージョンを **1.1.8** に更新：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm すべて同期。

## [1.1.7] - 2026-08-31

このリリースでは 1.1.6 で報告された問題を修正します：自動化ルールが失われないように、DNS リーク検出にステータスと暗号化方式を表示、ルートテーブルで VPN / 直接ルートを区別、ログフィルターの修正、通知時間とプロキシ表示の問題を解決。接続履歴の保持期間設定とインストール後の「実行」オプションも追加しました。

### 🐛 修正

- **自動化ルールが失われない（otherwise を含む）** — 条件タイプのないルールを不完全と誤判定して破棄しないように。フォームで表現できないディスク上のルールもそのまま保持され、設定を開いただけでルールが消えることはありません。
- **DNS リーク検出の結果を補完** — 各 DNS サーバーのプローブ状態（VPN / リーク / OK / 応答なし）と遅延を正しく表示し、実際に使われている出口 DNS を「使用中」マークで明示。
- **DNS 暗号化方式の検出** — 各リゾルバのトランスポートを検出：平文 UDP/53、DoT（TCP/853 TLS）、DoH（TCP/443 候補）。テスト後に結果の解説とリーク防止策（VPN DNS、暗号化 DNS、フルトンネルモードなど）を表示。
- **ルートテーブルで VPN / 直接を区別** — バックエンドがアクティブなトンネルインターフェースを照合して `is_vpn` を判定し、名前からの推測ではなく正確な VPN / Direct バッジを表示。
- **ログフィルターの修正** — ログイベントに `category` フィールドを付与し、カテゴリフィルターが実際に機能するように。レベル/カテゴリボタンに各件数を表示し、分布が一目でわかります。
- **通知時間の設定** — 一部の Svelte バージョンでドロップダウンが空白になり選択時間が表示されない問題を修正。
- **プロキシ表示の整合性** — direct モードでプロキシアドレスが残らないようにし、CLI でのプロキシモード変更も設定 UI にリアルタイム反映。

### ✨ 改善

- **接続履歴の保持期間** — 設定 → 詳細に「履歴の保持期間」（デフォルト 7 日、無効化可）を追加。超過分は自動的に削除されます（200 件の上限は引き続き適用）。
- **インストール後に実行** — Windows インストーラーの完了ページに「WireGuide Plus を実行」オプション（デフォルトでチェック）を追加。

### 🛠 内部

- バージョンを **1.1.7** に更新：`VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、NSIS、MSIX、Linux nfpm すべて同期。

## [1.1.6] - 2026-08-30

本リリースは更新メカニズムを刷新します。Windows / Linux ではアプリ内で直接ダウンロードしてインストールできるようになり（GitHub ページへの遷移だけではなく）、更新通知に「今すぐアップデート」と「リリースページを開く」の 2 つのボタンとリアルタイムのダウンロード進捗を追加。ミラーモードではアセットのダウンロードもアクセラレータを通ります。

### ✨ 新機能

- **アプリ内直接アップデート（Windows / Linux）** — 更新通知に「今すぐアップデート」ボタンを追加。ダウンロード完了後に SHA256（リリース版では Ed25519 署名も）を検証し、通過後はインストーラを起動してアプリを終了します。macOS の Homebrew インストールは従来どおり `brew upgrade` を使用します。
- **「リリースページを開く」代替ボタン** — ダウンロード失敗・検証失敗・リリースノートを確認したい場合に、対応する GitHub Release ページをワンクリックでブラウザで開きます。
- **リアルタイムのダウンロード進捗** — アップデート中にダウンロード済み / 合計サイズとパーセンテージを表示（GitHub API が報告するアセットサイズを基準にするため、チャンク転送でも正確です）。
- **ミラーモードがアセットのダウンロードにも適用** — GitHub アクセラレータミラー（mirror）を設定すると、アセットとチェックサムファイルのダウンロードもミラープレフィックスで書き換えられます（従来は API チェックのみがミラーを使用し、バイナリは GitHub 直結のままでした）。

### 🛠 内部

- ダウンロード・インストール失敗時も沈黙せず、ログを記録してリリースページを開くフォールバックに移行。常に新しいバージョンへの経路が確保されます。
- ダウンロード進捗コールバック、ミラーダウンロードの書き換え、`RunUpdate` の防御的分岐のユニットテストを追加。
- バージョンを **1.1.6** に更新: `VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` をすべて同期。

## [1.1.5] - 2026-08-30

本リリースはログシステムを大幅に強化し（更新チェック・設定監査・カテゴリ分類・保存期間の自動削除）、設定の不具合を修正し、デフォルトで無効の WireGuard スクリプト対応を復活させます。

### ✨ 新機能

- **更新チェックの完全ログ** — 手動・自動チェックとも、実際にリクエストした endpoint・ローカルバージョン・最新バージョン・`not_modified`・エラー/リトライ情報を記録します。失敗（403、タイムアウトなど）は `category=update` 付きで Log 画面に表示・フィルタ可能です。
- **設定変更の監査ログ** — 保存のたびに変更された設定（プロキシモード、kill switch など）と主要な値を記録します。プロキシの資格情報はマスクされます（`http://***@host`）。
- **ログのカテゴリ分類とフィルタ** — `ipc.LogEntry` に `category` フィールドを追加（app / update / settings / tunnel / network / system）。Log 画面にカテゴリフィルタ行を追加（All が先頭・デフォルト選択）。各行にカテゴリを表示し、コピー時にも含まれます。
- **ログ保存期間（デフォルト 7 日）** — 日次ローテーション（`wireguideplus-YYYY-MM-DD.log`）で保存し、設定可能な保存期間を超えたファイルを自動削除します。
- **WireGuard スクリプト対応（PreUp / PostUp / PreDown / PostDown、デフォルト無効）** — wg-quick と同じ動作（Unix は `sh -c`、Windows は `cmd.exe /C`）。helper 内で 30 秒のタイムアウト付きで実行し、出力は 1000 文字に切り詰めます。デフォルト無効（設定 → 詳細）。完全なシステム権限で実行されるため、有効化時には目立つセキュリティ警告を表示します。PostUp の失敗で接続は中断されません。
- **DNS leak test の強化** — 各 DNS サーバーにプローブ状態（vpn / ok / leak / timeout）と遅延を表示。Windows の DNS 収集で IPv4 と IPv6 の両方を扱います。
- **フォルダを開くショートカット** — 設定に、トンネル設定フォルダとログ保存フォルダを開くクリック可能なリンクを追加（クロスプラットフォーム）。

### 🐛 バグ修正

- **通知の表示時間設定が保存できない問題** — 設定画面を離れて再度開いても値がリセットされなくなりました。
- **設定のログレベルに All がなかった問題** — ドロップダウンに `All` を追加（Log 画面のデフォルトと一致）。シンク側でレコードが除外されません。

### 🛠 内部

- **ログレベル All を全経路で有効化** — helper / GUI のログハンドラが `all`（`slog.Level(-8)`）を解釈し、レコードを落としません。
- バージョンを **1.1.5** に更新: `VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` をすべて同期。

## [1.1.3] - 2026-08-30

本リリースは Windows の自動更新が機能しなかった問題を修正します。v1.1.0 のアセット改名以降、Windows のリリースアセット（`wireguideplus-<arch>-installer.exe` / `wireguideplus-<arch>-portable.zip`）には OS トークンが含まれていませんが、更新チェッカーはアセット名に OS トークンとアーキテクチャの両方を要求していました。そのため Windows は自身のアセットに一切マッチせず、「更新あり・一致するアセットなし」と表示されて自動更新できませんでした。

### 🐛 バグ修正

- **Windows の自動更新アセットマッチングを修正** — `matchAsset`（`internal/update/checker.go`）は Windows では、アーキテクチャで固定され Windows 固有の拡張子（`.exe` / `.msi` / `.zip`）を持つアセット名を OS トークンなしでも受け付けるようにしました。macOS / Linux アセットは引き続き自 OS のトークン（`darwin` / `linux`）を要求するため、トークンなしの Windows アセット名に誤マッチしません。回帰テストは、3 つの Windows アーキテクチャの正常マッチと、Linux / macOS がトークンなしの Windows アセット名を拒否する逆アサーションをカバーします。

### 🛠 内部

- バージョンを **1.1.3** に更新: `VERSION`、`build/config.yml`、`windows/info.json`、`windows/versioninfo.json`、`windows/wails.exe.manifest`、NSIS、MSIX、Linux nfpm、macOS `Info.plist` をすべて同期。

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
