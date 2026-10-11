package dynamips

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/storage/local"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func testStoredState(t *testing.T, adapter *Adapter) {
	t.Helper()
	root := t.TempDir()
	store, storageErr := local.New(root)
	if storageErr != nil {
		t.Fatal(storageErr)
	}
	manager := NewManager(store)
	ctx := context.Background()
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		t.Fatal(cwdErr)
	}
	configs := []*Config{
		{
			Config: emulator.Config{NodeConfig: virt.NodeConfig{
				ID: "stored-one", Name: "stored-one", Image: "unused-ios.bin",
			}},
			State:      storage.Ref{Provider: store.Name(), Key: "routers/one"},
			Platform:   PlatformConfig{Name: "c7200", C7200: &C7200Config{NPE: "npe-400"}},
			InstanceID: 20, StartupConfig: []byte("hostname StoredOne\nend\n"), PrivateConfig: []byte{},
		},
		{
			Config: emulator.Config{NodeConfig: virt.NodeConfig{
				ID: "stored-two", Name: "stored-two", Image: "unused-ios.bin",
			}},
			State:      storage.Ref{Provider: store.Name(), Key: "routers/two"},
			Platform:   PlatformConfig{Name: "c7200", C7200: &C7200Config{NPE: "npe-400"}},
			InstanceID: 21, StartupConfig: []byte("hostname StoredTwo\nend\n"), PrivateConfig: []byte{},
		},
	}
	leases := make([]*PreparedResources, len(configs))
	t.Cleanup(func() {
		for index, cfg := range configs {
			if cleanupErr := manager.Delete(ctx, adapter, cfg.ID, leases[index]); cleanupErr != nil {
				t.Error(cleanupErr)
			}
		}
	})
	for index, cfg := range configs {
		_, resources, createErr := manager.Create(ctx, adapter, cfg)
		leases[index] = resources
		if createErr != nil {
			t.Fatal(createErr)
		}
		statePath := resources.State.Location.Value
		for _, filename := range []string{"nvram", "lock", "log.txt"} {
			if _, statErr := os.Stat(filepath.Join(statePath, filename)); statErr != nil {
				t.Fatalf("state file %s: %v", filename, statErr)
			}
		}
	}
	duplicate := *configs[0]
	duplicate.ID, duplicate.Name, duplicate.InstanceID = "shared-state", "shared-state", 22
	if _, _, createErr := manager.Create(ctx, adapter, &duplicate); !errors.Is(createErr, errs.ErrDuplicate) {
		t.Fatalf("active routers must not share a state directory: %v", createErr)
	}
	current, currentErr := os.Getwd()
	if currentErr != nil || current != cwd {
		t.Fatalf("working directory changed: %s %v", current, currentErr)
	}
	if _, statErr := os.Stat(filepath.Join(cwd, "nvram")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("NVRAM escaped its prepared directory: %v", statErr)
	}
	for index, cfg := range configs {
		statePath := leases[index].State.Location.Value
		if deleteErr := manager.Delete(ctx, adapter, cfg.ID, leases[index]); deleteErr != nil {
			t.Fatal(deleteErr)
		}
		if leases[index].State != nil {
			t.Fatal("state lease was not released")
		}
		if _, statErr := os.Stat(filepath.Join(statePath, "nvram")); statErr != nil {
			t.Fatalf("native deletion removed durable NVRAM: %v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(statePath, "lock")); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("native deletion retained the VM lock: %v", statErr)
		}
		original := cfg.StartupConfig
		cfg.StartupConfig, cfg.PrivateConfig = nil, nil
		_, resources, reopenErr := manager.Create(ctx, adapter, cfg)
		leases[index] = resources
		if reopenErr != nil {
			t.Fatal(reopenErr)
		}
		saved, extractErr := adapter.Routers[cfg.ID].VM.ExtractConfig()
		if extractErr != nil || !bytes.Equal(saved.Startup, original) {
			t.Fatalf("persisted startup config differs: got=%q error=%v", saved.Startup, extractErr)
		}
	}
}
