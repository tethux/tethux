package ci

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewArchiveWriterRejectsUnsafeRunID(t *testing.T) {
	_, err := NewArchiveWriter(ArchiveOptions{
		Root:       t.TempDir(),
		Repository: t.TempDir(),
		Workflow:   "test",
		Revision:   "revision",
		RunID:      "../../outside",
	})
	if err == nil {
		t.Fatal("expected unsafe archive run ID to be rejected")
	}
}

func TestNewArchiveWriterRejectsNonV7RunID(t *testing.T) {
	_, err := NewArchiveWriter(ArchiveOptions{
		Root:       t.TempDir(),
		Repository: t.TempDir(),
		Workflow:   "test",
		Revision:   "revision",
		RunID:      "550e8400-e29b-41d4-a716-446655440000",
	})
	if err == nil {
		t.Fatal("expected non-v7 archive run ID to be rejected")
	}
}

func TestNewArchiveWriterCanonicalizesUUIDRunID(t *testing.T) {
	writer, err := NewArchiveWriter(ArchiveOptions{
		Root:       t.TempDir(),
		Repository: t.TempDir(),
		Workflow:   "test",
		Revision:   "revision",
		RunID:      "019FE7E4-DAF2-7D54-9F77-980BC8619FCA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if writer.Options.RunID != "019fe7e4-daf2-7d54-9f77-980bc8619fca" {
		t.Fatalf("run ID was not canonicalized: %q", writer.Options.RunID)
	}
}

func TestCollectResultsAssignsAttemptsToDuplicateEvents(t *testing.T) {
	stage := t.TempDir()
	artifacts := filepath.Join(stage, "artifacts")
	if err := os.Mkdir(artifacts, 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("{\"name\":\"pull\",\"status\":\"passed\"}\n{\"name\":\"pull\",\"status\":\"passed\"}\n")
	if err := os.WriteFile(filepath.Join(artifacts, "events.jsonl"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	writer := &ArchiveWriter{Options: ArchiveOptions{RunID: "run"}, Stage: stage}
	results, err := writer.collectResults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Tests) != 2 || results.Tests[0].Attempt != 1 || results.Tests[1].Attempt != 2 {
		t.Fatalf("attempts = %#v, want 1 and 2", results.Tests)
	}
}
