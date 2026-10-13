package dynamips

import (
	"context"
	"errors"
	"testing"

	"github.com/tethux/tethux/internal/testutil"
	"github.com/tethux/tethux/storage"
	//repolint:ignore package-boundaries -- real local storage fixture
	"github.com/tethux/tethux/storage/local"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func TestManagerFailureOwnership(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		location      bool
		release       bool
		nativeCleanup bool
	}{
		{name: "creation"},
		{name: "unsupported location", location: true},
		{name: "release failure", release: true},
		{name: "location and release failure", location: true, release: true},
		{name: "native cleanup failure", nativeCleanup: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := testutil.NewStorage(t)
			store.RequireRollback = true
			if scenario.location {
				store.InvalidLocation = 1
			}
			if scenario.release {
				store.FailRelease = 1
			}
			manager := NewManager(store)
			cause := testutil.ErrCreate
			if scenario.nativeCleanup {
				cause = errors.Join(cause, errs.ErrCleanup)
			}
			provider := &failingRouterProvider{cause: cause}
			cfg := &Config{Config: emulator.Config{NodeConfig: virt.NodeConfig{ID: "router"}}, State: storage.Ref{Provider: local.DefaultName, Key: "state"}}
			_, resources, createErr := manager.Create(t.Context(), provider, cfg)
			if createErr == nil {
				t.Fatal("injected failure accepted")
			}
			var retained []*storage.Prepared
			if resources != nil && resources.State != nil {
				retained = append(retained, resources.State)
			}
			store.CheckOwnership(t, retained)
			if scenario.nativeCleanup && (!errors.Is(createErr, errs.ErrCleanup) || len(retained) != 1) {
				t.Fatalf("native ownership lost: %v", createErr)
			}
			if scenario.release && !errors.Is(createErr, testutil.ErrRelease) {
				t.Fatalf("cleanup cause lost: %v", createErr)
			}
			if len(retained) != 0 {
				store.FailRelease = 0
				store.RequireRollback = false
				if releaseErr := manager.Release(t.Context(), resources); releaseErr != nil {
					t.Fatal(releaseErr)
				}
				store.CheckOwnership(t, nil)
			}
		})
	}
}

type failingRouterProvider struct {
	Provider
	cause error
}

func (p *failingRouterProvider) CreateRouter(context.Context, *RuntimeConfig) (*emulator.Node, error) {
	return nil, p.cause
}
