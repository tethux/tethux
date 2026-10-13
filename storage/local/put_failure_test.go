package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tethux/tethux/storage"
)

type partialFailure struct{ cause error }

func (r partialFailure) Read(buffer []byte) (int, error) { return copy(buffer, "partial"), r.cause }

func TestPutReadFailurePreservesDestination(t *testing.T) {
	root := t.TempDir()
	provider, newErr := New(root)
	if newErr != nil {
		t.Fatal(newErr)
	}
	ref := storage.Ref{Provider: DefaultName, Key: "object"}
	if putErr := provider.Put(t.Context(), ref, strings.NewReader("original"), storage.PutOptions{}); putErr != nil {
		t.Fatal(putErr)
	}
	cause := errors.New("injected read failure")
	putErr := provider.Put(t.Context(), ref, partialFailure{cause: cause}, storage.PutOptions{})
	if !errors.Is(putErr, cause) {
		t.Fatalf("lost read failure: %v", putErr)
	}
	// #nosec G304 -- fixed object name beneath t.TempDir.
	data, readErr := os.ReadFile(filepath.Join(root, "object"))
	if readErr != nil || string(data) != "original" {
		t.Fatalf("destination changed: %q, %v", data, readErr)
	}
	entries, listErr := os.ReadDir(root)
	if listErr != nil || len(entries) != 1 {
		t.Fatalf("temporary files retained: %v, %v", entries, listErr)
	}
}
