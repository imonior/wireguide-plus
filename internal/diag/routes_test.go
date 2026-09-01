package diag

import "testing"

func TestExpandDarwinNetAddr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// macOS netstat compresses the trailing ".0" octets of network
		// routes; the parser must expand them back to canonical form.
		{"127", "127.0.0.0/8"},
		{"169.254", "169.254.0.0/16"},
		{"192.168.1", "192.168.1.0/24"},
		{"10", "10.0.0.0/8"},
		// Already-canonical or non-network entries pass through.
		{"127.0.0.1", "127.0.0.1"},
		{"255.255.255.255", "255.255.255.255"},
		{"default", "default"},
		{"fe80::1%en0", "fe80::1%en0"},
		{"link#4", "link#4"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := expandDarwinNetAddr(tt.in); got != tt.want {
			t.Errorf("expandDarwinNetAddr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseDarwinRouteOutput(t *testing.T) {
	in := `Routing tables

Internet:
Destination        Gateway            Flags        Netif Expire
default            192.168.1.1        UGScg           en0
127                  127.0.0.1         UCS             lo0
127.0.0.1           127.0.0.1          UH              lo0
169.254            link#4             UCS             en0      !
192.168.1           link#4             UCSc           en0      !
fe80::1%en0         fe80::1%en0        UHLI            lo0
`
	routes := parseDarwinRouteOutput(in)

	want := []RouteEntry{
		{Destination: "default", Gateway: "192.168.1.1", Flags: "UGScg", Interface: "en0"},
		{Destination: "127.0.0.0/8", Gateway: "127.0.0.1", Flags: "UCS", Interface: "lo0"},
		{Destination: "127.0.0.1", Gateway: "127.0.0.1", Flags: "UH", Interface: "lo0"},
		{Destination: "169.254.0.0/16", Gateway: "link#4", Flags: "UCS", Interface: "en0"},
		{Destination: "192.168.1.0/24", Gateway: "link#4", Flags: "UCSc", Interface: "en0"},
		{Destination: "fe80::1%en0", Gateway: "fe80::1%en0", Flags: "UHLI", Interface: "lo0"},
	}
	if len(routes) != len(want) {
		t.Fatalf("parsed %d routes, want %d:\n%+v", len(routes), len(want), routes)
	}
	for i := range want {
		if routes[i] != want[i] {
			t.Errorf("route[%d] = %+v, want %+v", i, routes[i], want[i])
		}
	}
}
