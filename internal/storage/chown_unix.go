//go:build darwin || linux

package storage

import (
	"os"
	"syscall"
)

// chownToDirOwner gives path the same owner UID as its containing directory
// (dir). Writers here may run as root — the privileged helper persists
// endpoint-failover endpoint changes through the user's tunnel store — and
// because every save lands via os.CreateTemp + os.Rename, the temp file's
// owner (the writing process) becomes the final file's owner. A root-owned
// 0600 .conf is unreadable by the unprivileged GUI, which then fails to load
// the tunnel and hides it from the list while the helper still counts it as
// active. Re-owning each written file to the directory's owner (the desktop
// user) keeps both processes able to read what the other wrote.
//
// chownToDirOwner 将 path 的属主 UID 归还给所在目录 dir 的属主。本层的写入方
// 可能以 root 运行（特权 helper 在端点自动切换时会经用户的隧道存储落盘），
// 而所有保存都经 os.CreateTemp + os.Rename 落地，临时文件的属主（即写入进程）
// 就是最终文件的属主。root 属主的 0600 .conf 对未提权的 GUI 不可读，导致隧道
// 加载失败并从列表消失，而 helper 侧仍将其计为活跃。把每个写出的文件归还给
// 目录属主（即桌面用户）可保证两个进程始终能读对方写的内容。
//
// Best-effort and silent: the save itself already succeeded, so a chown
// failure must not fail the save — the helper's directory repair and the
// next write retry will pick it up. The same-owner shortcut avoids a chown
// call entirely in the common unprivileged-GUI case (file already owned by
// the directory's owner), sidestepping any platform quirks around chown
// calls that do not actually change the owner.
//
// 尽力而为且静默：保存本身已成功，chown 失败不应导致保存失败——helper 的
// 目录修复与下一次写入重试会兜底。属主已一致时直接跳过，既省一次系统调用，
// 也规避各平台对「未实际变更属主的 chown」的差异化处理。
func chownToDirOwner(path, dir string) {
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return
	}
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		return
	}
	if fileStat, ok := fileInfo.Sys().(*syscall.Stat_t); ok && fileStat.Uid == dirStat.Uid {
		return
	}
	_ = os.Chown(path, int(dirStat.Uid), -1)
}
