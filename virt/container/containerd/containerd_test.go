package containerd

import "testing"

func TestIsLoopbackRegistry(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"127.0.0.1:5000/tethux/fixture-a:1": true,
		"localhost:5000/tethux/fixture-a:1": true,
		"[::1]:5000/tethux/fixture-a:1":     true,
		"registry.example/tethux/image:1":   false,
		"alpine:3.20":                       false,
	}
	for ref, want := range tests {
		if got := isLoopbackRegistry(ref); got != want {
			t.Errorf("isLoopbackRegistry(%q) = %v, want %v", ref, got, want)
		}
	}
}
