package local

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tethux/tethux/storage"
	storageerrs "github.com/tethux/tethux/storage/errs"
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
	req := storage.PrepareRequest{
		Ref:        storage.Ref{Provider: DefaultName, Key: "nodes/router/data"},
		NodeID:     "router-1",
		AccessMode: storage.AccessReadWrite,
	}
	prepared, err := provider.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Location.Kind != storage.LocationPath || prepared.Location.Value != path {
		t.Fatalf("unexpected location: %#v", prepared.Location)
	}
	if prepared.ID == "" {
		t.Fatal("expected prepared storage ID")
	}
	if prepared.NodeID != req.NodeID {
		t.Fatalf("unexpected node ID: %q", prepared.NodeID)
	}
	if prepared.AccessMode != req.AccessMode {
		t.Fatalf("unexpected access mode: %q", prepared.AccessMode)
	}
	if prepared.Ownership != storage.OwnershipExternal {
		t.Fatalf("unexpected ownership: %q", prepared.Ownership)
	}

	info, err := provider.Stat(context.Background(), req.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if info.Checksum != nil {
		t.Fatal("did not expect a checksum for a directory")
	}
}

func TestPrepareCreatesDirectory(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/data"},
		NodeID:       "router-1",
		ResourceType: storage.ResourceTypeDirectory,
		Create:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(prepared.Location.Value)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected prepared resource to be a directory")
	}
	if prepared.Ownership != storage.OwnershipExternal {
		t.Fatalf("unexpected ownership: %q", prepared.Ownership)
	}
}

func TestPrepareCreatesFile(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/config"},
		NodeID:       "router-1",
		ResourceType: storage.ResourceTypeFile,
		Create:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(prepared.Location.Value)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("expected prepared resource to be a regular file")
	}
}

func TestPrepareRejectsUnsupportedMode(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	_, err = provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:  storage.Ref{Provider: DefaultName, Key: "images/router.qcow2"},
		Mode: storage.PrepareCopy,
	})
	if err == nil {
		t.Fatal("expected copy mode to be rejected")
	}
}

func TestCommitSyncsPreparedFile(t *testing.T) {
	provider, newErr := New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	prepared, prepareErr := provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/disk"},
		ResourceType: storage.ResourceTypeFile,
		Create:       true,
	})
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}

	_, commitErr := provider.Commit(context.Background(), prepared, storage.CommitOptions{
		Durability: storage.DurabilityData,
	})
	if commitErr != nil {
		t.Fatal(commitErr)
	}
}

func TestCommitRejectsConditionalGeneration(t *testing.T) {
	provider, newErr := New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	prepared, prepareErr := provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/disk"},
		ResourceType: storage.ResourceTypeFile,
		Create:       true,
	})
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}

	_, commitErr := provider.Commit(context.Background(), prepared, storage.CommitOptions{
		ExpectedGeneration: "generation",
	})
	if !errors.Is(commitErr, storageerrs.ErrConflict) {
		t.Fatalf("error = %v, want conflict", commitErr)
	}
}

func TestMoveOverwritesExistingFile(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	src := storage.Ref{Provider: DefaultName, Key: "src.txt"}
	dst := storage.Ref{Provider: DefaultName, Key: "dst.txt"}
	err = provider.Put(context.Background(), src, strings.NewReader("source"), storage.PutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = provider.Put(context.Background(), dst, strings.NewReader("destination"), storage.PutOptions{})
	if err != nil {
		t.Fatal(err)
	}

	err = provider.Move(context.Background(), src, dst, storage.MoveOptions{Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}

	statErr := error(nil)
	_, statErr = provider.Stat(context.Background(), src)
	if !errors.Is(statErr, storageerrs.ErrNotFound) {
		t.Fatalf("source stat error = %v, want not found", statErr)
	}
	reader, err := provider.Open(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "source" {
		t.Fatalf("destination content = %q, want source", content)
	}
}

func TestMoveOverwritesExistingDirectoryWithoutRemovingFileDestination(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	src := storage.Ref{Provider: DefaultName, Key: "src"}
	dst := storage.Ref{Provider: DefaultName, Key: "dst"}
	if err := os.MkdirAll(filepath.Join(provider.Root(), "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(provider.Root(), "src", "value"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(provider.Root(), "dst"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(provider.Root(), "dst", "old"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := provider.Move(context.Background(), src, dst, storage.MoveOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(provider.Root(), "dst", "value")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(provider.Root(), "dst", "old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old destination entry error = %v, want not found", err)
	}
}

func TestMoveRefusesExistingDestinationWithoutOverwrite(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	src := storage.Ref{Provider: DefaultName, Key: "src.txt"}
	dst := storage.Ref{Provider: DefaultName, Key: "dst.txt"}
	err = provider.Put(context.Background(), src, strings.NewReader("source"), storage.PutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = provider.Put(context.Background(), dst, strings.NewReader("destination"), storage.PutOptions{})
	if err != nil {
		t.Fatal(err)
	}

	err = provider.Move(context.Background(), src, dst, storage.MoveOptions{})
	if !errors.Is(err, storageerrs.ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
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

func TestStatUsesMetadataGeneration(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ref := storage.Ref{Provider: DefaultName, Key: "images/router.qcow2"}
	content := []byte("router image contents")
	putErr := provider.Put(context.Background(), ref, bytes.NewReader(content), storage.PutOptions{})
	if putErr != nil {
		t.Fatal(putErr)
	}

	info, err := provider.Stat(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if info.Checksum != nil {
		t.Fatal("did not expect a checksum")
	}
	if info.Generation == "" {
		t.Fatal("expected metadata generation")
	}
}

func TestStatDirectoryHasNoChecksumOrGeneration(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ref := storage.Ref{Provider: DefaultName, Key: "nodes/router/data"}
	err = os.MkdirAll(filepath.Join(provider.Root(), string(ref.Key)), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	info, err := provider.Stat(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if info.Type != storage.ObjectDir {
		t.Fatalf("type = %q, want directory", info.Type)
	}
	if info.Checksum != nil {
		t.Fatal("did not expect checksum for directory")
	}
	if info.Generation != "" {
		t.Fatalf("generation = %q, want empty", info.Generation)
	}
}

func TestInfoReportsCapabilities(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	info := provider.Info()
	if info.Name != DefaultName {
		t.Fatalf("name = %q, want %q", info.Name, DefaultName)
	}
	if !info.Capabilities.AtomicReplace {
		t.Fatal("expected local atomic replace support")
	}
	if !info.Capabilities.AtomicMove {
		t.Fatal("expected local atomic move support")
	}
	if info.Capabilities.ConditionalWrite {
		t.Fatal("conditional writes are not implemented yet")
	}
}
