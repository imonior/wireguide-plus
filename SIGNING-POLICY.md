# Code Signing Policy

This document describes how Windows binaries published by the
`imonior/wireguide-plus` project are signed, who can request a signature,
and what the integrity guarantees are. Required by SignPath Foundation
for projects using their free OSS code-signing service.

## Why code signing matters

Code signing provides three guarantees that a plain SHA-256 checksum
cannot:

1. **Integrity.** The Authenticode signature covers every byte of the
   installer. Any corruption or malicious modification — in transit, on
   a mirror, or on the user's disk — invalidates the signature and the
   binary is rejected by Windows.
2. **Authenticity / origin.** The certificate chain ties each binary to
   the SignPath Foundation certificate, which only signs artifacts
   produced by this project's own CI pipeline. Users can verify *who*
   published a file, not just that its hash matches a number on a web
   page.
3. **User trust and SmartScreen.** Binaries signed by a widely trusted
   certificate avoid the most alarming SmartScreen and UAC warnings, so
   users are less likely to be intimidated into discarding a legitimate
   installer — and less likely to accept an unsigned lookalike.

SHA-256 sums still ship with every release (`SHA256SUMS`, see
`docs/release.md`) as a defense-in-depth layer, but the signature is
the primary, user-verifiable trust anchor for the Windows installers.

## Scope

The signing policy applies to the Windows NSIS installer artifacts (every release
ships all three architectures):

- `wireguideplus-x86-installer.exe` (32-bit installer)
- `wireguideplus-amd64-installer.exe` (64-bit installer)
- `wireguideplus-arm64-installer.exe` (ARM64 installer)

macOS and Linux artifacts are not in scope (macOS uses ad-hoc signing
via `codesign`; Linux ships unsigned).

## Roles

WireGuide Plus is currently maintained by a single person. The same
individual fulfills the three SignPath-required roles:

| Role | Filled by |
|------|-----------|
| Committer (writes code, tags releases) | `<your-github-username>` |
| Reviewer (audits the change before release) | `<your-github-username>` |
| Approver (authorizes the signing request) | `<your-github-username>` |

TODO: replace the placeholders with the maintainer's GitHub username once
the SignPath Foundation OSS application is approved. The repo itself stays
at `github.com/imonior/wireguide-plus`; the vulnerability-report link below
is unchanged.

If additional maintainers join the project, the table will be updated
and signing approval will become a two-person process (the committer
of a release cannot also approve its signing).

## Signing approval workflow

Every signing request requires explicit human approval from an
Approver before SignPath issues the signature. This is enforced at
the SignPath policy level (`release-signing` policy, `Manual approval
required: yes`); the workflow that uploads the unsigned artifact
cannot trigger a signature without a separate, interactive approval
step in the SignPath portal.

While the project has a single maintainer, that maintainer reviews
the diff between the previous release tag and the current one before
issuing approval, and signing is deferred until that review is
complete. The single-maintainer exemption is acknowledged with
SignPath Foundation; if the project gains additional maintainers,
the policy moves to two-person approval (the release Committer
cannot also be the Approver for that release's signing request).

## Account security

All maintainers with signing-approval access have multi-factor
authentication enabled on:

- Their GitHub account (used for repository write + release tagging)
- Their SignPath account (used for signing approval)

GitHub MFA enforcement is also configured at the organization /
repository level so that the requirement cannot be silently relaxed.
Loss of an MFA device for the sole maintainer triggers the SignPath
emergency-access procedure (key revocation + new project enrollment),
not a recovery workaround.

## Privacy &amp; data handling

WireGuide Plus does not transmit telemetry, analytics, crash reports, or
configuration content to the maintainer, SignPath, or any third party.
WireGuard tunnels carry only the peer traffic the user has explicitly
configured; nothing about the user's machine, session, or usage is
exfiltrated as a side effect.

The application initiates two classes of outbound HTTP requests that
are NOT triggered by a tunnel:

1. **Update check.** A background scheduler queries the GitHub
   Releases API for this repository's latest tag — once at startup
   (after a 30-120 s jittered delay) and approximately every 24
   hours thereafter, plus an opportunistic recheck when the window
   regains focus after 4+ hours of idle. The request carries no
   identifying payload beyond the standard HTTP user-agent and
   what GitHub's public API logs by default (caller IP). Users can
   disable this entirely in Settings (`auto-update check` toggle);
   when disabled, no scheduled HTTP request fires.
2. **Manual update**. When the user clicks "Check now" or "Update"
   in the Updates UI, the application fetches the release feed and,
   for "Update", downloads the new installer / .app bundle directly
   from `github.com/imonior/wireguide-plus/releases/...`. Both are
   user-initiated.

No third-party domains beyond `api.github.com` and
`github.com/imonior/wireguide-plus` are contacted by the application
itself.

## Signing requests are accepted only from

- The `release.yml` workflow in this repository, running on GitHub
  Actions on Windows runners
- Triggered exclusively by a pushed tag matching `v*` (e.g. `v0.3.1`,
  `v0.4.0-rc1`)
- From the `main` branch's tagged history

These origin constraints are enforced by SignPath's "Trusted Build
System" verification, which cross-checks the artifact upload against
GitHub's API for the workflow run, repository, ref, and commit SHA.
Token theft alone cannot produce a signed binary — the token submitter
must additionally be a GitHub Actions workflow on this repository at
the expected ref.

## Reproducibility

Every signed binary is produced from a public commit on the `main`
branch. To reproduce locally:

```sh
git checkout v<version>
wails3 task windows:package ARCH=amd64   # or ARCH=386 for the 32-bit build
```

The resulting `.exe` SHA-256 will differ from the published binary
only by the Authenticode signature appended at the end; strip the
signature (`signtool remove`) to compare against the local build.

## Vulnerability reports

Security issues that should not be disclosed publicly may be sent to
the maintainer via GitHub's private vulnerability reporting:
<https://github.com/imonior/wireguide-plus/security/advisories/new>.

The maintainer commits to acknowledging reports within 7 days and
issuing a fix or mitigation within 30 days for confirmed
vulnerabilities.

## License compatibility

WireGuide is MIT-licensed. No commercial dual-licensing or proprietary
components. The project ships only its own source, the `wireguard-go`
userspace WireGuard implementation (MIT), and `wintun.dll` (Apache 2.0)
as a vendored binary fetched and SHA-256 verified at build time.

## Attribution

Per SignPath Foundation requirements, the project README and release
notes display the line:

> Free code signing provided by SignPath.io, certificate by SignPath
> Foundation.

with hyperlinks to `https://signpath.io` and `https://signpath.org`
respectively.
