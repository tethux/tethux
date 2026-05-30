package main

import (
	"strings"
	"testing"
)

func TestFormatFrame(t *testing.T) {
	frame := []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x00,
		'h', 'i',
	}

	got := formatFrame(frame)

	for _, want := range []string{
		"dst=ff:ff:ff:ff:ff:ff",
		"src=02:00:00:00:00:01",
		"ethertype=0x0800",
		`payload="hi"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatFrame() missing %q in %q", want, got)
		}
	}
}

func TestFormatShortFrame(t *testing.T) {
	got := formatFrame([]byte{0x01, 0x02})
	if !strings.Contains(got, "short-frame") {
		t.Fatalf("expected short-frame marker, got %q", got)
	}
}
