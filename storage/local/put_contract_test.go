package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tethux/tethux/storage"
	storageerrs "github.com/tethux/tethux/storage/errs"
)

func TestPutRejectsUnsupportedOptionsBeforeWriting(t *testing.T) {
	options := map[string]storage.PutOptions{
		"generation":   {ExpectedGeneration: "old"},
		"create only":  {IfNotExists: true},
		"content type": {ContentType: "text/plain"},
		"metadata":     {Metadata: storage.Metadata{"owner": "router"}},
		"kind":         {Kind: storage.ArtifactConfig},
	}
	for name, opts := range options {
		for _, existing := range []bool{false, true} {
			t.Run(name+"/"+map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
				root := t.TempDir()
				provider, newErr := New(root)
				if newErr != nil {
					t.Fatal(newErr)
				}
				ref := storage.Ref{Provider: DefaultName, Key: "nested/object"}
				if existing {
					if putErr := provider.Put(t.Context(), ref, strings.NewReader("original"), storage.PutOptions{}); putErr != nil {
						t.Fatal(putErr)
					}
				}
				reader := &unreadInput{t: t}
				putErr := provider.Put(context.Background(), ref, reader, opts)
				if !errors.Is(putErr, storageerrs.ErrInvalidOptions) {
					t.Fatalf("Put error = %v", putErr)
				}
				// #nosec G304 -- fixed object name beneath t.TempDir.
				data, readErr := os.ReadFile(filepath.Join(root, "nested", "object"))
				if existing {
					if readErr != nil || string(data) != "original" {
						t.Fatalf("original changed: %q, %v", data, readErr)
					}
				} else if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("object created: %v", readErr)
				}
				entries, listErr := os.ReadDir(root)
				if listErr != nil {
					t.Fatal(listErr)
				}
				if !existing && len(entries) != 0 {
					t.Fatal("rejected write created directories")
				}
			})
		}
	}
}

type unreadInput struct{ t *testing.T }

func (r *unreadInput) Read([]byte) (int, error) {
	r.t.Fatal("rejected write consumed input")
	return 0, errors.New("unexpected read")
}
