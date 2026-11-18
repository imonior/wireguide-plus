package app

import (
	"strings"
	"testing"
)

func TestGetTunnelFields(t *testing.T) {
	content := `# my tunnel
[Interface]
PrivateKey = AAAA
Address = 10.0.0.2/24
Jc = 4
# comment

[Peer]
PublicKey = BBBB
AllowedIPs = 0.0.0.0/0
`
	f, err := (&TunnelService{}).GetTunnelFields(content)
	if err != nil {
		t.Fatal(err)
	}
	if f.Interface["PrivateKey"] != "AAAA" || f.Interface["Jc"] != "4" {
		t.Errorf("interface fields wrong: %v", f.Interface)
	}
	if len(f.Peers) != 1 || f.Peers[0]["PublicKey"] != "BBBB" {
		t.Errorf("peer fields wrong: %v", f.Peers)
	}
}

func TestSetTunnelFieldsReplaceAndPreserve(t *testing.T) {
	content := `[Interface]
PrivateKey = AAAA
Address = 10.0.0.2/24
# keep me
ListenPort = 51820

[Peer]
PublicKey = BBBB
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`
	in := &TunnelFields{
		Interface: map[string]string{
			"PrivateKey": "CCCC",
			"Jc":         "4", // missing from text → append
			"ListenPort": "",  // present in text → delete
		},
		Peers: []map[string]string{
			{"Endpoint": "vpn.example.com:51820"}, // missing → append
		},
	}
	out, err := (&TunnelService{}).SetTunnelFields(content, in)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := (&TunnelService{}).GetTunnelFields(out)
	if f.Interface["PrivateKey"] != "CCCC" {
		t.Errorf("PrivateKey not replaced: %q", f.Interface["PrivateKey"])
	}
	if f.Interface["Jc"] != "4" {
		t.Errorf("Jc not appended: %v", f.Interface)
	}
	if _, exists := f.Interface["ListenPort"]; exists {
		t.Errorf("ListenPort should be deleted")
	}
	if f.Peers[0]["Endpoint"] != "vpn.example.com:51820" {
		t.Errorf("Endpoint not appended: %v", f.Peers[0])
	}
	if f.Peers[0]["PublicKey"] != "BBBB" {
		t.Errorf("PublicKey should be preserved")
	}
	if !strings.Contains(out, "# keep me") {
		t.Errorf("comment lost:\n%s", out)
	}
	if strings.Count(out, "[Interface]") != 1 || strings.Count(out, "[Peer]") != 1 {
		t.Errorf("section headers duplicated:\n%s", out)
	}
}

func TestSetTunnelFieldsMissingKeyLandsInOwnSection(t *testing.T) {
	content := `[Interface]
PrivateKey = AAAA

[Peer]
PublicKey = BBBB
`
	in := &TunnelFields{
		Interface: map[string]string{"MTU": "1420"},
		Peers:     []map[string]string{{"PersistentKeepalive": "25"}},
	}
	out, err := (&TunnelService{}).SetTunnelFields(content, in)
	if err != nil {
		t.Fatal(err)
	}
	// MTU must land in [Interface] (before [Peer]); PersistentKeepalive in [Peer].
	if !strings.Contains(out, "MTU = 1420\n[Peer]") {
		t.Errorf("MTU not appended inside [Interface]:\n%s", out)
	}
	if !strings.Contains(out, "PersistentKeepalive = 25") {
		t.Errorf("PersistentKeepalive missing:\n%s", out)
	}
}

func TestSetTunnelFieldsRejectsInjection(t *testing.T) {
	in := &TunnelFields{Interface: map[string]string{"PrivateKey\nJunk = x": "v"}}
	if _, err := (&TunnelService{}).SetTunnelFields("[Interface]\n", in); err == nil {
		t.Fatal("expected injection rejection")
	}
}

func TestSetTunnelFieldsExtraPeersIgnored(t *testing.T) {
	content := "[Interface]\nPrivateKey = AAAA\n\n[Peer]\nPublicKey = B\n\n[Peer]\nPublicKey = C\n"
	in := &TunnelFields{
		Interface: map[string]string{},
		Peers: []map[string]string{
			{"Endpoint": "a:1"},
			{"Endpoint": "b:2"},
			{"Endpoint": "c:3"}, // extra — ignored
		},
	}
	out, err := (&TunnelService{}).SetTunnelFields(content, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "Endpoint =") != 2 {
		t.Errorf("extra model peer should be ignored:\n%s", out)
	}
}

func TestDerivePublicKey(t *testing.T) {
	svc := &TunnelService{}
	// Fixed vector: private key bytes 01..32; public key computed independently
	// via wgtypes' own generator and pinned here so the test catches a change
	// in derivation rules, not just self-consistency.
	// 固定向量：私钥字节 01..32；公钥由 wgtypes 生成器独立算出并钉在此处，
	// 这样测试能捕获派生规则的变化，而不只是自洽。
	const (
		private = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
		public  = "B6N8vBQgk8i3VdwbEOhstCY3StFqqFPtC9/AsrhtHHw="
	)
	got, err := svc.DerivePublicKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if got != public {
		t.Errorf("public key = %q, want %q", got, public)
	}

	// Surrounding whitespace (pasted from the editor) must be tolerated.
	// 编辑器粘贴时带入的首尾空白应被容忍。
	if got, err = svc.DerivePublicKey("  " + private + "\n"); err != nil || got != public {
		t.Errorf("trimmed key: got %q / %v, want %q / nil", got, err, public)
	}

	for _, bad := range []string{"", "not-base64!!!", "AAAA"} {
		if got, err = svc.DerivePublicKey(bad); err == nil {
			t.Errorf("DerivePublicKey(%q) = %q, want error", bad, got)
		}
	}
}
