# Local Historical Release Archive

This directory stores **manually backed-up** local build artifacts
(installer, portable zip, standalone exe, wintun driver, …) for rollback,
comparison, and offline distribution. It is NOT part of the build output and
is **not** updated automatically on every release.

## Backup policy (manual)

- This is **not** an automatic copy after each release. Historical assets of
  every release are permanently kept on GitHub Releases and never lost.
- Only **locally tested builds** are archived here: build locally → test OK →
  when you consider it worth keeping, copy the files in manually.
- Hashes cannot be generated automatically after you upload the files to
  GitHub manually, so register the SHA256 in the manifest below **by hand**.

## Rules

- Anything newly dropped in here is ignored by the `releases/*` rule in
  `.gitignore`, so the directory cannot accumulate binaries by accident.
- The four v1.1.0 files listed in the manifest below are the deliberate
  exception: they match that ignore rule and are tracked anyway, which means
  they were added with `git add -f` on purpose. They are the rollback backup of
  the last build that was tested locally end to end — do not delete them, and
  do not "clean up" the repository because of their size. (The same release is
  also on GitHub, but its assets still carry the pre-rename `WireGuide-*`
  names, so the archive and the release page are not interchangeable 1:1.)
- `README.md` / `README.zh.md` are tracked too, so the directory's purpose
  survives clones and the directory is never treated as throwaway.
- What can and cannot destroy the tracked backups:
  - `git clean -fd` and `git clean -fdx` cannot touch them — clean only removes
    untracked files. `-x` additionally wipes any *new* ignored drop-in here, so
    still preview with `git clean -ndx releases/` before running it.
  - `git rm`, `git checkout` of a tree without them, and history rewrites do
    destroy them; the bytes then survive only in the commit that added them, and
    in nothing at all for a shallow clone or an archive export.
  - `task build` / `wails3` builds only write to `bin/`.
  - CI (GitHub Actions) runs on isolated runners and never touches local files.

## Manifest

Current archive: v1.1.0 locally tested build (filenames do not embed the
version; entries are distinguished by registration date):

| Version | Platform | File | SHA256 |
|---------|----------|------|--------|
| v1.1.0 | Windows amd64 | wireguideplus-amd64-installer.exe | 72957D9839707D5037AC82A8BA62AA41118C229BF10B380541D87BFA628EFFF1 |
| v1.1.0 | Windows amd64 | wireguideplus-amd64-portable.zip | 4947E758419720858889CE0487E99A57DF0214132E58111B28BE498A3AA31C2F |
| v1.1.0 | Windows amd64 | wireguideplus-amd64.exe | A6C34FEF72B170F8F41CF27B469ACCC3546801415E201AD5F6A5A9EAFC99F31C |
| v1.1.0 | Windows amd64 | wintun-amd64.dll | E5DA8447DC2C320EDC0FC52FA01885C103DE8C118481F683643CACC3220DAFCE |

Registered: 2026-08-30

## Commands (when adding a new backup)

```powershell
# Copy local build artifacts into the archive (example)
Copy-Item "D:\build-out\*" .\releases\

# Compute hashes and register them in the manifest above by hand
Get-FileHash .\releases\*.exe -Algorithm SHA256 | Format-Table
```

---

简体中文版：见 [README.zh.md](README.zh.md)
