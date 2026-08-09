package container

import "testing"

func TestParseImageRoundTrip(t *testing.T) {
	for _, ref := range []string{"alpine", "alpine:3.20", "localhost:5000/team/image:latest", "registry.example/team/image@sha256:0123456789abcdef"} {
		t.Run(ref, func(t *testing.T) {
			if got := ParseImage(ref).String(); got != ref {
				t.Fatalf("ParseImage(%q).String() = %q", ref, got)
			}
		})
	}
}
