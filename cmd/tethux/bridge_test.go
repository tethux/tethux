package main

import (
	"testing"
	"time"

	"github.com/0xveya/tethux/internal/libtethux"
)

func TestParsePortSpecs(t *testing.T) {
	ports, err := parsePortSpecs([]string{
		"id=left,scheme=udp,listen=127.0.0.1:10001,remote=127.0.0.1:11001,latency=5ms",
		"id=uplink,scheme=pcap,if=enp0s1,mtu=9000,snaplen=9100,immediate=false",
		"id=vm,scheme=tap,if=tap0",
	})
	if err != nil {
		t.Fatalf("parsePortSpecs() error = %v", err)
	}

	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}

	if ports[0].Scheme != libtethux.UDPScheme || ports[0].Latency != 5*time.Millisecond {
		t.Fatalf("unexpected udp port: %#v", ports[0])
	}

	if ports[1].Scheme != libtethux.PcapScheme || ports[1].Interface != "enp0s1" || ports[1].ImmediateMode {
		t.Fatalf("unexpected pcap port: %#v", ports[1])
	}

	if ports[2].Scheme != libtethux.TAPScheme || ports[2].Interface != "tap0" {
		t.Fatalf("unexpected tap port: %#v", ports[2])
	}
}

func TestParsePortSpecsRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"id=left,scheme=udp,listen=127.0.0.1:10001",
		"id=left,scheme=raw",
		"id=left,scheme=tap",
		"id=left,scheme=bogus",
	}

	for _, raw := range tests {
		if _, err := parsePortSpecs([]string{raw}); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestParseUDPShorthandSpecs(t *testing.T) {
	ports, err := parseUDPShorthandSpecs([]string{
		"left:127.0.0.1:10001:127.0.0.1:11001",
		"right:127.0.0.1:10002:127.0.0.1:11002",
		"tap:10.0.0.5:12000:198.51.100.10:13000",
	}, 1600)
	if err != nil {
		t.Fatalf("parseUDPShorthandSpecs() error = %v", err)
	}

	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}

	if ports[2].ID != "tap" || ports[2].Listen != "10.0.0.5:12000" || ports[2].Remote != "198.51.100.10:13000" || ports[2].MTU != 1600 {
		t.Fatalf("unexpected shorthand port: %#v", ports[2])
	}
}
