package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConf drops a .conf into dir with the given endpoint line.
// writeConf 把带指定 Endpoint 行的 .conf 写入 dir。
func writeConf(t *testing.T, dir, name, endpoint string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := "[Interface]\nPrivateKey = AAAA\n\n[Peer]\nPublicKey = BBBB\nEndpoint = " + endpoint + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestTunnelSignatureDetectsInPlaceRewrite is the regression guard for the
// hero showing a stale endpoint: an endpoint switch rewrites the SAME .conf
// (same name set), so a name-only signature reports "no change" and the
// frontend never re-reads the tunnel.
// TestTunnelSignatureDetectsInPlaceRewrite 是 hero 显示旧地址这一问题的回归
// 防线：端点切换就地改写同一个 .conf（文件名集合不变），只看名字的签名会报
// 「无变化」，前端便永远不会重读隧道。
func TestTunnelSignatureDetectsInPlaceRewrite(t *testing.T) {
	dir := t.TempDir()
	conf := writeConf(t, dir, "peer.conf", "a.example.net:51820")

	before := tunnelSignature(dir)
	if before == "" {
		t.Fatal("empty signature for a directory holding one .conf")
	}

	// Same file, different address — exactly what a switch leaves behind.
	// 同一个文件、不同的地址——正是切换后磁盘上的样子。
	writeConf(t, dir, "peer.conf", "203.0.113.7:51820")
	// Force a distinct mtime as well. On a filesystem with coarse timestamp
	// granularity the size change alone may not be observable, and a switch
	// can just as easily keep the byte length identical.
	// 同时强制一个不同的 mtime。时间戳粒度较粗的文件系统上单靠 size 变化未必
	// 可见，而切换也完全可能让字节长度保持不变。
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(conf, stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if after := tunnelSignature(dir); after == before {
		t.Fatalf("signature unchanged after an in-place rewrite (before=%q after=%q)", before, after)
	}
}

// TestTunnelSignatureStableWhenNothingChanged guards the other direction: a
// stable directory must not emit "tunnels_changed" every 3 s, which would
// make the frontend re-read and re-sort the whole list continuously.
// TestTunnelSignatureStableWhenNothingChanged 守住另一头：目录没变时不得每 3 秒
// 发一次 "tunnels_changed"，否则前端会不停重读并重排整个列表。
func TestTunnelSignatureStableWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	writeConf(t, dir, "peer.conf", "a.example.net:51820")

	first := tunnelSignature(dir)
	second := tunnelSignature(dir)
	if first != second {
		t.Fatalf("signature is not stable: %q vs %q", first, second)
	}
}

// TestTunnelSignatureDetectsSetChanges keeps the original contract intact:
// add / delete / rename must still be caught by the name half.
// TestTunnelSignatureDetectsSetChanges 保住原有契约：增 / 删 / 改名仍须被
// 名字那一半捕获。
func TestTunnelSignatureDetectsSetChanges(t *testing.T) {
	dir := t.TempDir()
	writeConf(t, dir, "one.conf", "a.example.net:51820")
	base := tunnelSignature(dir)

	writeConf(t, dir, "two.conf", "b.example.net:51820")
	added := tunnelSignature(dir)
	if added == base {
		t.Fatal("adding a tunnel did not change the signature")
	}

	if err := os.Remove(filepath.Join(dir, "two.conf")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed := tunnelSignature(dir); removed != base {
		t.Fatalf("removing a tunnel did not restore the signature: %q vs %q", removed, base)
	}

	if err := os.Rename(filepath.Join(dir, "one.conf"), filepath.Join(dir, "renamed.conf")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed := tunnelSignature(dir); renamed == base {
		t.Fatal("renaming a tunnel did not change the signature")
	}
}

// TestTunnelSignatureIgnoresNonConfFiles pins the filter: the sidecar
// .meta.json is written far more often (probe targets, automation opt-out)
// and must not turn every metadata edit into a tunnel-list refresh.
// TestTunnelSignatureIgnoresNonConfFiles 固定过滤规则：旁车 .meta.json 的写入
// 频繁得多（探测目标、自动化开关），不能让每次元数据改动都触发列表刷新。
func TestTunnelSignatureIgnoresNonConfFiles(t *testing.T) {
	dir := t.TempDir()
	writeConf(t, dir, "peer.conf", "a.example.net:51820")
	base := tunnelSignature(dir)

	if err := os.WriteFile(filepath.Join(dir, "peer.meta.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	after := tunnelSignature(dir)
	if after != base {
		t.Fatalf("meta file leaked into the signature: %q vs %q", after, base)
	}
	if strings.Contains(after, "meta.json") {
		t.Fatalf("signature mentions a non-.conf file: %q", after)
	}
}

// TestTunnelSignatureEmptyDir keeps the unreadable/absent directory case a
// stable empty string, so a missing dir reads as "unchanged" instead of
// flapping between "" and a value on every tick.
// TestTunnelSignatureEmptyDir 让不可读/不存在的目录保持稳定的空串，这样缺失的
// 目录读作「未变化」，而不会每一拍在 "" 与某个值之间抖动。
func TestTunnelSignatureEmptyDir(t *testing.T) {
	if got := tunnelSignature(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Fatalf("missing dir should sign as empty, got %q", got)
	}
	if got := tunnelSignature(t.TempDir()); got != "" {
		t.Fatalf("empty dir should sign as empty, got %q", got)
	}
}
