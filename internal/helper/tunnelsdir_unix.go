//go:build darwin || linux

package helper

import "os"

// repairTunnelsDir hands the user's tunnels directory and every file in it
// back to the desktop user identified by uid. Unlike prepareLogsDir it does
// NOT create the directory: a tunnels dir that does not exist yet has nothing
// to repair, and creating it as root would leave it root-owned and lock the
// unprivileged GUI out of creating its own tunnels there. The helper (and
// before the ownership hand-back existed, only the helper) rewrites .conf
// and .meta.json files during endpoint failover, so this repairs ownership
// of the damaged files already on disk.
//
// repairTunnelsDir 把用户的隧道目录及其中的所有文件归还给 uid 标识的桌面用户。
// 与 prepareLogsDir 不同，它不创建目录：尚不存在的隧道目录没有可修复的内容，
// 而以 root 身份创建会让目录变成 root 属主，反而把未提权的 GUI 挡在门外。
// helper 在端点切换时会重写 .conf 与 .meta.json（在属主归还机制出现之前，
// 只有 helper 会写这些文件），因此这里负责修复磁盘上已损坏文件的属主。
func repairTunnelsDir(dir string, uid int) error {
	if dir == "" || uid < 0 {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return chownR(dir, uid)
}
