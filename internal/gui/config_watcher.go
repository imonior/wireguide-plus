package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// startConfigWatcher polls the data files the CLI (or any external
// process) can change and notifies the frontend so a running GUI reflects
// them immediately, without going through the helper:
//
//   - config.json → "config_changed" (settings incl. automation rules)
//   - the tunnels dir listing → "tunnels_changed" (import / delete / rename
//     AND any in-place rewrite of an existing .conf)
//
// The second signal covers content edits on purpose. An endpoint switch —
// whether the helper rotates a candidate or the user picks one — rewrites
// the SAME .conf in place: the file set is unchanged, so a name-only
// signature was blind to it and the hero kept showing the address the
// tunnel had already stopped dialling.
// 第二个信号刻意覆盖内容改写。端点切换（不论 helper 轮换候选还是用户手选）
// 都是就地改写同一个 .conf：文件集合不变，只看文件名的签名对它完全无感，
// 于是 hero 一直显示隧道早已不拨的旧地址。
//
// A 3 s mtime/listing poll is used (fsnotify isn't a dependency); the
// latency is imperceptible for catching CLI-side edits, and it runs for
// the whole GUI lifetime including while hidden in the tray — 1 s bought
// nothing but 3× the stat+readdir churn. Reacting to the GUI's own
// writes is harmless — the frontend re-reads and re-applies the same
// values idempotently.
func startConfigWatcher(app *application.App, configPath, tunnelsDir string, done <-chan struct{}, wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		fileMtime := func(p string) time.Time {
			fi, err := os.Stat(p)
			if err != nil {
				return time.Time{}
			}
			return fi.ModTime()
		}

		lastCfg := fileMtime(configPath)
		lastTun := tunnelSignature(tunnelsDir)

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			if m := fileMtime(configPath); !m.Equal(lastCfg) {
				lastCfg = m
				app.Event.Emit("config_changed", struct{}{})
			}
			if sig := tunnelSignature(tunnelsDir); sig != lastTun {
				lastTun = sig
				app.Event.Emit("tunnels_changed", struct{}{})
			}
		}
	}()
}

// tunnelSignature fingerprints the tunnels directory as sorted
// "<name>|<size>|<mtime>" rows, one per .conf file.
//
// The name half catches add/delete/rename regardless of directory-mtime
// quirks; the size+mtime half catches in-place rewrites, which a name-only
// signature cannot see. Both halves are load-bearing: an endpoint switch
// rewrites an existing .conf without touching the file set, and silently
// dropping that signal is exactly what left the hero advertising a stale
// address. Package-level (rather than a closure inside the watcher) so the
// behaviour is reachable from tests.
// tunnelSignature 把隧道目录指纹化为排序后的 "<name>|<size>|<mtime>" 行，
// 每个 .conf 一行。
//
// 名字那一半覆盖增删改名（不受目录 mtime 怪癖影响）；size+mtime 那一半覆盖
// 就地改写——只看名字的签名看不见它。两半都不可少：端点切换只改写既有 .conf
// 而不改变文件集合，悄悄丢掉这个信号正是 hero 长期挂着旧地址的原因。放在包级
// （而非 watcher 内的闭包）以便测试可达。
func tunnelSignature(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var rows []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		// A file vanishing between readdir and stat is a transient race,
		// not a change of interest: stamp it as empty so the row stays
		// stable and the next tick settles it.
		// 文件在 readdir 与 stat 之间消失属瞬态竞态，不是我们关心的变化：
		// 戳为空值使该行保持稳定，下一拍自然收敛。
		size, mod := int64(0), int64(0)
		if info, err := e.Info(); err == nil {
			size = info.Size()
			mod = info.ModTime().UnixNano()
		}
		rows = append(rows, fmt.Sprintf("%s|%d|%d", e.Name(), size, mod))
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// configFilePath returns the config.json path inside a config dir.
func configFilePath(configDir string) string {
	return filepath.Join(configDir, "config.json")
}
