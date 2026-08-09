package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tethux/tethux/internal/libtethux/storage"
)

func TestPrepareDirectory(t *testing.T) {
	root := t.TempDir()
	provider, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "nodes", "router", "data")
	if mkdirErr := os.MkdirAll(path, 0o750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	prepared, err := provider.Prepare(context.Background(), storage.PrepareRequest{Ref: storage.Ref{Provider: DefaultName, Key: "nodes/router/data"}})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Location.Kind != storage.LocationPath || prepared.Location.Value != path {
		t.Fatalf("unexpected location: %#v", prepared.Location)
	}
}

func TestRejectsEscapingAndSymlinkKeys(t *testing.T) {
	root := t.TempDir()
	provider, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []storage.Key{"../outside", "/absolute", "escape/object"} {
		if _, err := provider.Prepare(context.Background(), storage.PrepareRequest{Ref: storage.Ref{Provider: DefaultName, Key: key}}); err == nil {
			t.Errorf("expected key %q to be rejected", key)
		}
	}
}
