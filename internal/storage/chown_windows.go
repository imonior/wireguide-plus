//go:build windows

package storage

// chownToDirOwner is a no-op on Windows: NTFS ACL inheritance already makes
// files created by either side (the unprivileged GUI or the elevated helper)
// readable by the other, and Windows has no UID-based chown to apply.
//
// chownToDirOwner 在 Windows 上是空操作：NTFS 的 ACL 继承已保证任一侧（未提权
// 的 GUI 或提权的 helper）创建的文件对另一方可读，Windows 也没有基于 UID 的
// chown 可用。
func chownToDirOwner(path, dir string) {}
