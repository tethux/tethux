package ingest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCompletedCandidatesRequiresMatchingDoneMarker(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(
		root,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"normal",
		"018f0000-0000-7000-8000-000000000001.tar.zst",
	)
	if err := os.MkdirAll(filepath.Dir(archive), 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("archive")
	if err := os.WriteFile(archive, content, 0o600); err != nil {
		t.Fatal(err)
	}

	candidates, err := DiscoverCompletedCandidates(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("found %d candidates without completion marker", len(candidates))
	}

	checksum := fmt.Sprintf("%x\n", sha256.Sum256(content))
	if err = os.WriteFile(archive+".done", []byte(checksum), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, err = DiscoverCompletedCandidates(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("found %d completed candidates, want 1", len(candidates))
	}

	if err = os.WriteFile(archive+".done", []byte("bad checksum\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates, err = DiscoverCompletedCandidates(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("found %d candidates with invalid marker", len(candidates))
	}
}
