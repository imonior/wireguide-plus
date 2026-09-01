package tunnel

import (
	"strings"
	"testing"

	"github.com/imonior/wireguide-plus/internal/config"
)

// buildIpcConfig is exercised with a resolved config (endpoints as literal
// IPs), so the test configs use literal IP endpoints.

func testAWGConfig() *config.WireGuardConfig {
	return &config.WireGuardConfig{
		Protocol: config.ProtocolAmneziaWG,
		Interface: config.InterfaceConfig{
			PrivateKey: "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
			Address:    []string{"10.0.0.2/24"},
			MTU:        1420,
			Jc:         5,
			Jmin:       100,
			Jmax:       500,
			S1:         10,
			S2:         20,
			H1:         "123456-123500",
			H4:         "123456-123500",
		},
		Peers: []config.PeerConfig{
			{
				PublicKey:  "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=",
				Endpoint:   "203.0.113.5:51820",
				AllowedIPs: []string{"0.0.0.0/0"},
			},
		},
	}
}

func TestBuildIpcConfigEmitsAWGFields(t *testing.T) {
	uapi, err := buildIpcConfig(testAWGConfig())
	if err != nil {
		t.Fatalf("buildIpcConfig: %v", err)
	}

	for _, want := range []string{
		"jc=5\n",
		"jmin=100\n",
		"jmax=500\n",
		"s1=10\n",
		"s2=20\n",
		"h1=123456-123500\n",
		"h4=123456-123500\n",
	} {
		if !strings.Contains(uapi, want) {
			t.Errorf("UAPI output missing %q:\n%s", want, uapi)
		}
	}

	// AWG device-level fields must be emitted after replace_peers and
	// before the first peer's public_key (amneziawg-go parses them in the
	// device-config phase).
	afterReplace := uapi[strings.Index(uapi, "replace_peers=true\n"):]
	peerSection := strings.Index(afterReplace, "public_key=")
	if peerSection < 0 {
		t.Fatalf("no peer section in UAPI:\n%s", uapi)
	}
	deviceSection := afterReplace[:peerSection]
	for _, want := range []string{"jc=5", "h4=123456-123500"} {
		if !strings.Contains(deviceSection, want) {
			t.Errorf("AWG field %q not in device section:\n%s", want, deviceSection)
		}
	}
}

func TestBuildIpcConfigWireGuardHasNoAWGFields(t *testing.T) {
	cfg := testAWGConfig()
	cfg.Protocol = config.ProtocolWireGuard
	// Even with AWG params populated, a wireguard-config build must not
	// emit them — wireguard-go's IpcSet rejects unknown keys.
	uapi, err := buildIpcConfig(cfg)
	if err != nil {
		t.Fatalf("buildIpcConfig: %v", err)
	}
	for _, bad := range []string{"jc=", "jmin=", "jmax=", "s1=", "h1="} {
		if strings.Contains(uapi, bad) {
			t.Errorf("wireguard UAPI must not contain %q:\n%s", bad, uapi)
		}
	}
}

func TestBuildIpcConfigSkipsZeroAWGFields(t *testing.T) {
	cfg := testAWGConfig()
	cfg.Interface.S3 = 0
	cfg.Interface.S4 = 0
	cfg.Interface.H2 = ""
	cfg.Interface.H3 = ""
	uapi, err := buildIpcConfig(cfg)
	if err != nil {
		t.Fatalf("buildIpcConfig: %v", err)
	}
	if strings.Contains(uapi, "s3=") || strings.Contains(uapi, "h2=") {
		t.Errorf("zero/empty AWG fields must be omitted:\n%s", uapi)
	}
}
