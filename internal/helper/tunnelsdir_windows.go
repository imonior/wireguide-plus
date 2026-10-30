//go:build windows

package helper

// repairTunnelsDir is a no-op on Windows: NTFS ACL inheritance already makes
// tunnel files written by the elevated helper readable by the unprivileged
// GUI, and Windows has no UID-based chown to apply.
//
// repairTunnelsDir 在 Windows 上是空操作：NTFS 的 ACL 继承已保证提权 helper
// 写出的隧道文件对未提权的 GUI 可读，Windows 也没有基于 UID 的 chown 可用。
func repairTunnelsDir(dir string, uid int) error { return nil }
