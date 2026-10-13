package container

import (
	"context"
	"errors"
	"testing"

	"github.com/tethux/tethux/internal/testutil"
	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/storage/local"
)

func TestManagerFailureOwnership(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		prepare  int
		location int
		release  int
		canceled bool
		create   bool
		partial  bool
	}{
		{name: "first preparation", prepare: 1},
		{name: "second preparation", prepare: 2},
		{name: "unsupported location", location: 2},
		{name: "rollback failure", prepare: 2, release: 1},
		{name: "location and release failure", location: 2, release: 2},
		{name: "canceled preparation", prepare: 2, canceled: true},
		{name: "creation failure", create: true},
		{name: "partial native creation", create: true, partial: true},
		{name: "creation and cleanup failure", create: true, release: 2},
		{name: "canceled creation", create: true, canceled: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := testutil.NewStorage(t)
			store.FailPrepare, store.InvalidLocation, store.FailRelease = scenario.prepare, scenario.location, scenario.release
			store.RequireRollback = true
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario.canceled {
				store.Cancel = cancel
			}
			manager := NewManager(store)
			refs := []storage.Ref{{Provider: local.DefaultName, Key: "one"}, {Provider: local.DefaultName, Key: "two"}}
			cfg := &ContainerConfig{Volumes: []VolumeMount{{Source: refs[0]}, {Source: refs[1]}}}
			var resources *PreparedResources
			var operationErr error
			if scenario.create {
				provider := &failingProvider{partial: scenario.partial}
				if scenario.canceled {
					provider.cancel = cancel
				}
				var node *ContainerNode
				node, resources, operationErr = manager.Create(ctx, provider, cfg)
				if scenario.partial && node == nil {
					t.Fatal("partial native handle lost")
				}
			} else {
				_, resources, operationErr = manager.Prepare(ctx, cfg)
			}
			if operationErr == nil {
				t.Fatal("injected failure was accepted")
			}
			if scenario.prepare != 0 && !errors.Is(operationErr, testutil.ErrPrepare) {
				t.Fatalf("lost preparation failure: %v", operationErr)
			}
			if scenario.create && !errors.Is(operationErr, testutil.ErrCreate) {
				t.Fatalf("lost creation failure: %v", operationErr)
			}
			var retained []*storage.Prepared
			if resources != nil {
				retained = resources.Volumes
			}
			store.CheckOwnership(t, retained)
			if scenario.release != 0 {
				if !errors.Is(operationErr, testutil.ErrRelease) || len(retained) != 1 {
					t.Fatalf("lost cleanup failure or lease: %v, %v", operationErr, retained)
				}
				store.FailRelease = 0
				retryCtx, stop := context.WithCancel(context.Background())
				defer stop()
				store.RequireRollback = false
				if retryErr := manager.Release(retryCtx, resources); retryErr != nil {
					t.Fatal(retryErr)
				}
				store.CheckOwnership(t, resources.Volumes)
				if retryErr := manager.Release(retryCtx, resources); retryErr != nil {
					t.Fatal(retryErr)
				}
			}
		})
	}
}

type failingProvider struct {
	ContainerProvider
	cancel  context.CancelFunc
	partial bool
}

func (p *failingProvider) CreateContainer(context.Context, *RuntimeConfig) (*ContainerNode, error) {
	if p.cancel != nil {
		p.cancel()
	}
	if p.partial {
		return &ContainerNode{}, testutil.ErrCreate
	}
	return nil, testutil.ErrCreate
}
