package db

import (
	"context"
	"testing"
)

func TestGetSchemaInfoGroupsColumnsByObject(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir() + "/schema.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	info, err := store.GetSchemaInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]struct{}, len(info.Objects))
	for _, object := range info.Objects {
		if _, duplicate := seen[object.Name]; duplicate {
			t.Fatalf("schema object %q was returned more than once", object.Name)
		}
		seen[object.Name] = struct{}{}
		if len(object.Columns) == 0 {
			t.Fatalf("schema object %q has no columns", object.Name)
		}
	}

	if _, ok := seen["runs"]; !ok {
		t.Fatal("runs table missing from schema")
	}
}
