# Release signing & rotation

## 0. Cutting a release (local push → published)

No manual GitHub steps needed: pushing a `v*` tag triggers
`.github/workflows/release.yml`, which in parallel builds every artifact,
verifies/signs them (SignPath optional), generates release notes with
git-cliff (config: `cliff.toml`), creates the GitHub Release, attaches all
assets plus the Ed25519-signed `SHA256SUMS` / `SHA256SUMS.sig`, and bumps
the Homebrew cask.

### Workflow triggers

The workflows fire on **events**, not on any analysis of what changed:

- `.github/workflows/ci.yml` — runs **only** on `pull_request` and manual
  `workflow_dispatch`. Pushes to `main` (merged PRs, doc-only edits,
  everything) never light it up by design, so routine commits are cheap.
- `.github/workflows/release.yml` — runs **only** when a tag matching `v*`
  is pushed (`on.push.tags`). There is no `paths` filter and no content
  sniffing: any `v*` tag triggers the full cross-platform build, regardless
  of what the diff contains.
- `.github/workflows/fix-release-notes.yml` — manual `workflow_dispatch`
  utility: regenerates the body of an **already-published tag** with git-cliff
  (the tag is a required input) and overwrites it via `gh release edit`. Used
  when a noise commit (e.g. GitHub's "Add files via upload") slipped into a
  published changelog after a `cliff.toml` parser change.

To build without a tag, trigger `ci.yml` manually via `workflow_dispatch`.

### Versioning & tag rules

- A release is identified **by its tag**. Bump the version for every
  functional change (and keep `CHANGELOG.md` current): new tag → new release
  page, fresh assets, cask bump. GitHub does not compare version numbers, so
  the tag name is the only signal the pipeline sees.
- The app version itself lives in **exactly one place** — the repository-root
  `VERSION` file. `task bump:version` (`tools/bumpversion`) rewrites every
  static build/package metadata file from it in one pass (build/config.yml,
  Windows version resources / manifest / NSIS / MSIX, Linux nfpm, macOS
  Info.plist). The Go binaries read `VERSION` at build time via
  `-ldflags "-X github.com/imonior/wireguide-plus/internal/version.Version=<v>"`,
  so a version bump touches three things and nothing else: the `VERSION` file,
  `CHANGELOG.md`, and the git tag.
- **Never force-push an existing tag** (`git push -f origin v1.1.0`). GitHub
  forbids re-pushing a tag that already exists; force-pushing one makes
  `softprops/action-gh-release` **reuse the old release page** — assets mix
  with the previous build, the body is not refreshed (`update_release_body`
  is intentionally not set), and the Homebrew cask never moves.
- Pre-release tags follow `vX.Y.Z-<suffix>` (e.g. `v1.2.0-rc1`): the release
  is marked `prerelease` and the cask job skips them.

```bash
# 1. (one-time) confirm the signing keypair is in place and matches:
gh secret list                         # UPDATE_SIGNING_KEY must be present
grep UPDATE_SIGNING_PUBKEY .github/workflows/release.yml
go run ./tools/updatesign pub          # must print the same public hex

# 2. bump the version — the VERSION file is the single source of truth:
echo 1.1.2 > VERSION
task bump:version                      # rewrites every static build/package metadata file
#    (or, one step: task bump:version 1.1.2)
#    Go binaries need no edits: build/*/Taskfile.yml inject VERSION at build time.

# 3. make sure CHANGELOG.md (and its en / zh-TW / ja / ko siblings) already
#    describe the new version, then cut:
git tag v1.1.2 && git push origin v1.1.2

# 4. watch the pipeline
gh run watch
```

Release assets produced per tag:

| Asset | Job |
| --- | --- |
| `wireguideplus-x86-installer.exe` (32-bit installer) | build-windows (x86) |
| `wireguideplus-amd64-installer.exe` (64-bit installer) | build-windows (amd64) |
| `wireguideplus-arm64-installer.exe` (ARM64 installer) | build-windows (arm64) |
| `wireguideplus-x86-portable.zip` / `wireguideplus-amd64-portable.zip` / `wireguideplus-arm64-portable.zip`（每个 zip 内含 `wireguideplus-<arch>.exe` + 对应 `wintun-<arch>.dll`；bare exe 与 bare `wintun-<arch>.dll` 均不单独发布） | build-windows |
| `WireGuide-darwin-arm64.zip` (portable, contains `wireguideplus.app`) | build-macos |
| `WireGuide-darwin-arm64.dmg` (drag-and-drop installer) | build-macos |
| `WireGuide-linux-amd64.deb` / `WireGuide-linux-arm64.deb` (installers) | build-linux |
| `WireGuide-linux-amd64-portable.tar.gz` / `WireGuide-linux-arm64-portable.tar.gz` (portable, bare `wireguideplus` binary) | build-linux |
| `SHA256SUMS` + `SHA256SUMS.sig` | release |

> Windows always ships 32-bit, 64-bit and ARM64 installers per the release
> policy; the CI matrix keys off the `arch` (GOARCH) / `asset` (x86/amd64/
> arm64) pair defined in `release.yml`. Every artifact name embeds the
> architecture (`wireguideplus-<arch>-installer.exe`, `-portable.zip`, bare
> exe `wireguideplus-<arch>.exe`), and the installed program is also
> installed as `wireguideplus-<arch>.exe`.

### Release notes

The GitHub Release body is generated by **git-cliff** from the actual git
log (config: `cliff.toml`), not by GitHub's built-in PR-only generator:

1. `orhun/git-cliff-action@v4` runs with `--latest --strip header` and
   writes `CHANGELOG.md`.
2. `softprops/action-gh-release@v2` attaches it via `body_path`.

Everything in the body — the commit groups (led by the "⚠️ Upgrade notes"
block for commits prefixed `BREAKING:` / `Upgrade:`), the closing "📦
Downloads" asset guide, and the compare link — is template-controlled in
`cliff.toml` and fully customizable. To switch generators, set
`generate_release_notes: true` or point `body_path` at a static file instead.

The auto-update verifier in `internal/update/checker.go` defends
against a compromised GitHub account by verifying an Ed25519
signature over `SHA256SUMS` with a public key embedded in the binary
at compile time:

- **Public key** — hex, injected at build time via
  `-ldflags "-X github.com/imonior/wireguide-plus/internal/update.expectedPublicKey=<hex>"`.
  Release CI passes it as the `SIGN_PUBKEY` task variable (see
  `UPDATE_SIGNING_PUBKEY` in `.github/workflows/release.yml`); the
  platform Taskfiles fold it into `BUILD_FLAGS` alongside
  `-tags production`, which turns enforcement ON
  (`internal/update/require_signed_release.go`).
- **Private key** — a 32-byte Ed25519 seed, hex-encoded, stored in the
  `UPDATE_SIGNING_KEY` GitHub Actions secret. The release workflow
  signs `SHA256SUMS` → `SHA256SUMS.sig` (raw 64-byte signature) and
  publishes it next to `SHA256SUMS`. A pre-publish check derives the
  public key from the secret and fails the release if it doesn't match
  `UPDATE_SIGNING_PUBKEY` — a mismatched pair would ship binaries that
  reject our own releases.
- **Maintainer backup** — the same seed lives at
  `~/.wireguide/release-signing.key` (mode 0600) on the release
  machine. **Back this file up somewhere safe** (password manager /
  offline). If both the secret and the backup are lost, the key cannot
  be recovered — rotate (§3).

Everything is driven by `tools/updatesign` (stdlib-only; its
sign/verify pair matches the client verifier exactly):

```bash
go run ./tools/updatesign gen -out <seed-file>   # new key; prints PUBLIC hex, never the seed
go run ./tools/updatesign pub                    # seed from $UPDATE_SIGNING_KEY (or -key <file>); prints public hex
go run ./tools/updatesign sign -in SHA256SUMS    # writes SHA256SUMS.sig
go run ./tools/updatesign verify -pub <hex> -in SHA256SUMS
```

---

## 1. One-time setup (already done)

```bash
go run ./tools/updatesign gen -out ~/.wireguide/release-signing.key
#  → prints the public key; paste into UPDATE_SIGNING_PUBKEY in release.yml
gh secret set UPDATE_SIGNING_KEY < ~/.wireguide/release-signing.key
```

Older binaries built with `expectedPublicKey == ""` (all releases up
to and including v0.3.1, and every dev build) skip verification and
rely on SHA256 alone — they are unaffected by any of this.

## 2. Signing a release

Nothing manual: the tag-triggered workflow bakes the public key into
every platform build, then signs `SHA256SUMS` in the `release` job and
attaches `SHA256SUMS.sig` to the GitHub Release. The step **fails the
whole release** if the secret is missing or mismatched, because the
just-built binaries would otherwise refuse all future auto-updates.

## 3. Rotating the key

Rotate when the key may have leaked (GitHub org compromise, laptop
loss if the backup was on it) or on long-cadence hygiene:

1. `go run ./tools/updatesign gen -out <new-seed-file>` on a clean machine.
2. Update `UPDATE_SIGNING_PUBKEY` in `release.yml` with the new public
   key and `gh secret set UPDATE_SIGNING_KEY < <new-seed-file>`.
3. Cut the cutover release. **Its `SHA256SUMS.sig` must verify for the
   binaries already in the wild**, which check the OLD public key — so
   sign that one release manually with the old key
   (`go run ./tools/updatesign sign -key <old-seed-file> ...`, replace
   the workflow-produced .sig on the Release) or temporarily keep the
   old pair in CI for it.
4. Once users have crossed the cutover, new releases are new-key only.
   Keep the cutover release downloadable indefinitely — clients that
   skip it verify new releases against the old key and must go through
   it (or reinstall manually).

There is deliberately no signed key-transition manifest — the fleet is
small and the manual cutover above is simpler to reason about.

---

## Implementation pointers

- Verifier: `internal/update/checker.go` — `verifyChecksumSignature`
  (raw 64-byte `.sig`, hex pubkey, signature over the exact
  `SHA256SUMS` bytes); enforcement gate in
  `internal/update/require_signed_release.go` (`-tags production`).
- Signer/keygen: `tools/updatesign/main.go`
- CI wiring: `.github/workflows/release.yml` (`UPDATE_SIGNING_PUBKEY`
  env, `SIGN_PUBKEY` task var, "Sign SHA256SUMS" step) and the
  `BUILD_FLAGS` vars in `build/{darwin,windows,linux}/Taskfile.yml`.
- Tests: `internal/update/checker_test.go` — `TestVerifyEd25519_*`,
  `TestVerifyChecksumSignature_*`
