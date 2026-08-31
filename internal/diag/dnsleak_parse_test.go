package diag

import "testing"

func TestParseResolvectlStatus(t *testing.T) {
	out := `Global
       LLMNR setting: yes
       DNSOverTLS setting: no

Link 2 (enp3s0)
      Current Scopes: DNS
           DNS Servers: 192.168.1.1
                        8.8.8.8
Link 3 (wlan0)
           DNS Servers: 1.1.1.1
Link 4 (wg0)
           DNS Servers: 10.10.0.1
                        2001:db8::53
`
	servers := parseResolvectlStatus(out)
	if len(servers) != 5 {
		t.Fatalf("want 5 resolvers, got %d: %+v", len(servers), servers)
	}
	byIP := map[string]systemResolver{}
	for _, s := range servers {
		byIP[s.IP] = s
	}
	if s := byIP["192.168.1.1"]; !s.IsLocal || s.Iface != "enp3s0" {
		t.Errorf("192.168.1.1: want local/enp3s0, got %+v", s)
	}
	if s := byIP["8.8.8.8"]; !s.IsLocal || s.Iface != "enp3s0" {
		t.Errorf("8.8.8.8 (wrapped continuation): want local/enp3s0, got %+v", s)
	}
	if s := byIP["1.1.1.1"]; !s.IsLocal || s.Iface != "wlan0" {
		t.Errorf("1.1.1.1: want local/wlan0, got %+v", s)
	}
	if s := byIP["10.10.0.1"]; !s.IsVPN || s.Iface != "wg0" {
		t.Errorf("10.10.0.1: want vpn/wg0, got %+v", s)
	}
	if s := byIP["2001:db8::53"]; !s.IsVPN || s.Iface != "wg0" {
		t.Errorf("2001:db8::53 (IPv6 on wg): want vpn/wg0, got %+v", s)
	}
}

func TestParseDarwinScutil(t *testing.T) {
	out := `DNS configuration

resolver #1
  search domain[0] : home
  nameserver[0] : 192.168.1.1
  if_index : 4 (en0)
  flags    : Request A records

scoped resolver #2
  nameserver[0] : 8.8.8.8
  if_index : 8 (utun0)
  flags    : Scoped

resolver #3
  nameserver[0] : 10.0.0.1
  nameserver[1] : 2001:db8::1
  if_index : 9 (utun4)
`
	servers := parseDarwinScutil(out)
	if len(servers) != 4 {
		t.Fatalf("want 4 resolvers, got %d: %+v", len(servers), servers)
	}
	byIP := map[string]systemResolver{}
	for _, s := range servers {
		byIP[s.IP] = s
	}
	if s := byIP["192.168.1.1"]; !s.IsLocal || s.Iface != "en0" {
		t.Errorf("192.168.1.1: want local/en0, got %+v", s)
	}
	if s := byIP["8.8.8.8"]; !s.IsVPN || s.Iface != "utun0" {
		t.Errorf("8.8.8.8: want vpn/utun0, got %+v", s)
	}
	if s := byIP["10.0.0.1"]; !s.IsVPN || s.Iface != "utun4" {
		t.Errorf("10.0.0.1: want vpn/utun4, got %+v", s)
	}
	if s := byIP["2001:db8::1"]; !s.IsVPN || s.Iface != "utun4" {
		t.Errorf("2001:db8::1: want vpn/utun4, got %+v", s)
	}
}

func TestIfaceNameIsVPN(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"wg0", true},
		{"wlan0", false},
		{"enp3s0", false},
		{"eth0", false},
		{"utun0", true},
		{"tun0", true},
		{"tap0", true},
		{"ppp0", true},
		{"tailscale0", true},
	}
	for _, c := range cases {
		if got := ifaceNameIsVPN(c.name); got != c.want {
			t.Errorf("ifaceNameIsVPN(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
