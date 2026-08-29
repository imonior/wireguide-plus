# Local Historical Release Archive

This directory stores **local backups** of released artifacts (installers,
portable zips, dmg, deb, tar.gz, SHA256SUMS, signatures, …) for rollback,
comparison, and offline distribution. It is NOT part of the build output.

## Rules

- Binary files are **never committed** to git (repo bloat). They are ignored
  by the `releases/*` rule in `.gitignore`.
- The `README.md` / `README.zh.md` files ARE tracked, so the directory's
  purpose survives clones and the directory is never treated as throwaway.
- Routine cleanup commands will NOT touch this directory:
  - `git clean -fd`: safe (ignored files are kept)
  - `task build` / `wails3` builds: only write to `bin/`
  - CI (GitHub Actions): runs on isolated runners, never touches local files
- ⚠️ The ONLY command that removes these files is `git clean -fdx`
  (`-x` deletes ignored files too) — **never run it casually**. If you truly
  need to wipe the archive, preview first with `git clean -ndx releases/`.

## Manifest

> After every release, copy the artifacts here and register them below.

| Version | Platform | File | SHA256 |
|---------|----------|------|--------|
| v1.1.0 | Windows amd64 | wireguideplus-1.1.0-amd64-installer.exe | register here |
| v1.1.0 | Windows amd64 | wireguideplus-1.1.0-amd64-portable.zip | register here |
| ... | ... | ... | ... |

## Commands

```powershell
# Copy historical artifacts into the archive (example)
Copy-Item "D:\releases-backup\*" .\releases\

# Compute hashes to register in the manifest
Get-FileHash .\releases\*.exe -Algorithm SHA256 | Format-Table
```

---

简体中文版：见 [README.zh.md](README.zh.md)
