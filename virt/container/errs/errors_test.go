package errs

import (
	"errors"
	"testing"
)

func TestOpErrorWrapsKindAndCause(t *testing.T) {
	cause := errors.New("daemon unavailable")
	err := Wrap("containerd", ErrFailedToStartContainer, "node-a", cause)

	if !errors.Is(err, ErrFailedToStartContainer) {
		t.Fatal("stable error category was not preserved")
	}
	if !errors.Is(err, cause) {
		t.Fatal("runtime cause was not preserved")
	}
	if got, want := err.Error(), "containerd: failed to start container: node-a: daemon unavailable"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
