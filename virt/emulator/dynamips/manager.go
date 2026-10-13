package dynamips

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/tethux/tethux/internal/cleanup"
	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

// Manager prepares and releases durable router state storage.
type Manager struct {
	storage storage.Manager
}

// NewManager creates a router manager using storageManager for preparation.
func NewManager(storageManager storage.Manager) *Manager {
	return &Manager{storage: storageManager}
}

// PreparedResources records the state lease retained until the router is deleted.
type PreparedResources struct {
	State *storage.Prepared
}

// Prepare resolves a durable state reference into a provider-ready directory.
func (m *Manager) Prepare(ctx context.Context, cfg *Config) (*RuntimeConfig, *PreparedResources, error) {
	if m == nil || m.storage == nil || cfg == nil || cfg.ID == "" {
		return nil, nil, errs.New("prepare router", errs.ErrConfig, "storage manager, config, and node ID are required")
	}
	if refErr := cfg.State.Validate(); refErr != nil {
		return nil, nil, errs.Wrap("prepare router state", errs.ErrStorage, cfg.ID, refErr)
	}
	prepared, prepareErr := m.storage.Prepare(ctx, storage.PrepareRequest{
		Ref: cfg.State, NodeID: cfg.ID, AccessMode: storage.AccessReadWrite,
		ResourceType: storage.ResourceTypeDirectory, Create: true,
	})
	if prepareErr != nil {
		return nil, nil, errs.Wrap("prepare router state", errs.ErrStorage, cfg.ID, prepareErr)
	}
	resources := &PreparedResources{State: prepared}
	if prepared.Location.Kind != storage.LocationPath || !filepath.IsAbs(prepared.Location.Value) {
		locationErr := errs.New("prepare router state", errs.ErrUnsupported, "state storage must resolve to an absolute directory path")
		cleanupCtx, cancel := cleanup.NewContext(ctx)
		defer cancel()
		releaseErr := m.Release(cleanupCtx, resources)
		if releaseErr != nil {
			return nil, resources, errors.Join(locationErr, releaseErr)
		}
		return nil, nil, locationErr
	}
	return &RuntimeConfig{Config: *cfg, StateDirectory: prepared.Location.Value}, resources, nil
}

// Release releases prepared runtime material without deleting durable state.
// The router must have been deleted first. Failed releases remain retryable.
func (m *Manager) Release(ctx context.Context, resources *PreparedResources) error {
	if resources == nil || resources.State == nil {
		return nil
	}
	if m == nil || m.storage == nil {
		return errs.New("release router state", errs.ErrStorage, "storage manager is required")
	}
	releaseErr := m.storage.Release(ctx, resources.State)
	if releaseErr != nil {
		return errs.Wrap("release router state", errs.ErrStorage, resources.State.NodeID, releaseErr)
	}
	resources.State = nil
	return nil
}

// Create prepares storage and creates a stopped router.
// The caller owns returned resources, including on failed native cleanup or release.
func (m *Manager) Create(ctx context.Context, provider Provider, cfg *Config) (*emulator.Node, *PreparedResources, error) {
	if provider == nil {
		return nil, nil, errs.New("create router", errs.ErrConfig, "provider is required")
	}
	runtimeCfg, resources, prepareErr := m.Prepare(ctx, cfg)
	if prepareErr != nil {
		return nil, resources, prepareErr
	}
	node, createErr := provider.CreateRouter(ctx, runtimeCfg)
	if createErr != nil {
		if errors.Is(createErr, errs.ErrCleanup) {
			return nil, resources, createErr
		}
		cleanupCtx, cancel := cleanup.NewContext(ctx)
		defer cancel()
		releaseErr := m.Release(cleanupCtx, resources)
		if releaseErr != nil {
			return nil, resources, errors.Join(createErr, releaseErr)
		}
		return nil, nil, createErr
	}
	return node, resources, nil
}

// Delete deletes the native router before releasing its storage lease.
func (m *Manager) Delete(ctx context.Context, provider Provider, id string, resources *PreparedResources) error {
	if provider == nil {
		return errs.New("delete router", errs.ErrConfig, "provider is required")
	}
	if resources != nil && resources.State != nil && resources.State.NodeID != id {
		return errs.New("delete router", errs.ErrConfig, "storage lease belongs to another node")
	}
	deleteErr := provider.Delete(ctx, id)
	if deleteErr != nil {
		return deleteErr
	}
	return m.Release(ctx, resources)
}
