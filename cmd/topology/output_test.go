package topology

import (
	"net/netip"
	"testing"
)

func TestFirstAddress(t *testing.T) {
	cases := []struct{ name, output, address string }{
		{"guest address", "30: eth0    inet 10.77.0.3/24 scope global eth0\n", "10.77.0.3"},
		{"loopback before guest", "1: lo inet 127.0.0.1/8 scope host lo\n30: eth0 inet 10.77.0.1/24 scope global eth0\n", "10.77.0.1"},
		{"no configured address", "", ""},
		{"IPv6 only", "30: eth0 inet6 fe80::1/64 scope link eth0\n", ""},
		{"malformed prefix", "30: eth0 inet not-an-address scope global eth0\n", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := firstAddress(test.output)
			want := netip.Addr{}
			if test.address != "" {
				want = netip.MustParseAddr(test.address)
			}
			if got != want {
				t.Fatalf("address = %v, want %v", got, want)
			}
		})
	}
}
