package storage_test

import (
	"strings"
	"testing"

	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/storage/local"
)

func TestReleaseResourcesPreservesDurableSource(t *testing.T) {
	provider, newErr := local.New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	ref := storage.Ref{Provider: provider.Name(), Key: "disk"}
	if putErr := provider.Put(t.Context(), ref, strings.NewReader("persistent"), storage.PutOptions{}); putErr != nil {
		t.Fatal(putErr)
	}
	prepared, prepareErr := provider.Prepare(t.Context(), storage.PrepareRequest{Ref: ref})
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	resources, releaseErr := storage.ReleaseResources(t.Context(), provider, []*storage.Prepared{prepared})
	if releaseErr != nil || len(resources) != 0 {
		t.Fatalf("release: %v, %v", resources, releaseErr)
	}
	info, statErr := provider.Stat(t.Context(), ref)
	if statErr != nil || info.Size != int64(len("persistent")) {
		t.Fatalf("durable object: %v, %v", info, statErr)
	}
	if _, retryErr := storage.ReleaseResources(t.Context(), provider, resources); retryErr != nil {
		t.Fatal(retryErr)
	}
}
