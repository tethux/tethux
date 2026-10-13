package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/tethux/tethux/internal/cleanup"
	"github.com/tethux/tethux/storage"
	storageerrs "github.com/tethux/tethux/storage/errs"
)

// Manager prepares storage-backed domain configuration before provider calls.
type Manager struct {
	storage storage.Manager
}

// NewManager returns a domain manager backed by storageManager.
func NewManager(storageManager storage.Manager) *Manager {
	return &Manager{
		storage: storageManager,
	}
}

// PreparedResources tracks storage leases owned by a domain operation.
type PreparedResources struct {
	Disks []*storage.Prepared
}

// Prepare resolves a portable domain configuration into provider-ready paths.
// On error, returned resources remain owned by the caller.
func (m *Manager) Prepare(
	ctx context.Context,
	cfg *Config,
) (*RuntimeConfig, *PreparedResources, error) {
	if m == nil || m.storage == nil {
		return nil, nil, storageerrs.New("domain", storageerrs.ErrInvalidOptions, "storage manager is not configured")
	}
	if cfg == nil {
		return nil, nil, storageerrs.New("domain", storageerrs.ErrInvalidOptions, "configuration is nil")
	}
	resources := &PreparedResources{
		Disks: make([]*storage.Prepared, 0, len(cfg.Disks)),
	}

	disks := make([]RuntimeDisk, 0, len(cfg.Disks))

	for _, disk := range cfg.Disks {
		access := storage.AccessReadWrite
		if disk.ReadOnly {
			access = storage.AccessReadOnly
		}

		prepared, err := m.storage.Prepare(
			ctx,
			storage.PrepareRequest{
				Ref:        disk.Source,
				NodeID:     cfg.ID,
				AccessMode: access,
			},
		)
		if err != nil {
			return m.rollback(ctx, resources, err)
		}

		resources.Disks = append(resources.Disks, prepared)

		if prepared.Location.Kind != storage.LocationPath {
			locationErr := storageerrs.New("domain", storageerrs.ErrInvalidResourceType, fmt.Sprintf("disk %s resolved to unsupported location %q", disk.Source, prepared.Location.Kind))
			return m.rollback(ctx, resources, locationErr)
		}

		disks = append(disks, RuntimeDisk{
			Source:   prepared.Location.Value,
			Device:   string(disk.Device),
			Bus:      string(disk.Bus),
			Target:   disk.Target,
			Format:   string(disk.Format),
			ReadOnly: disk.ReadOnly,
		})
	}

	return &RuntimeConfig{
		NodeConfig:   cfg.NodeConfig,
		Machine:      cfg.Machine,
		Architecture: cfg.Architecture,
		Firmware:     cfg.Firmware,
		Disks:        disks,
		Interfaces:   cfg.Interfaces,
		BootOrder:    cfg.BootOrder,
	}, resources, nil
}

// Release returns every storage resource acquired by Prepare.
func (m *Manager) Release(
	ctx context.Context,
	resources *PreparedResources,
) error {
	if resources == nil {
		return nil
	}

	remaining, releaseErr := storage.ReleaseResources(ctx, m.storage, resources.Disks)
	resources.Disks = remaining
	return releaseErr
}

// Create prepares cfg and asks provider to create the domain.
// On error, the caller owns returned nodes and resources; delete nodes before release.
func (m *Manager) Create(
	ctx context.Context,
	provider Provider,
	cfg *Config,
) (*Node, *PreparedResources, error) {
	if provider == nil {
		return nil, nil, storageerrs.New("domain", storageerrs.ErrInvalidOptions, "provider is nil")
	}
	runtimeCfg, resources, err := m.Prepare(ctx, cfg)
	if err != nil {
		return nil, resources, err
	}

	node, err := provider.CreateDomain(ctx, runtimeCfg)
	if err != nil {
		if node != nil {
			return node, resources, err
		}
		_, remaining, rollbackErr := m.rollback(ctx, resources, err)
		return nil, remaining, rollbackErr
	}

	return node, resources, nil
}

// rollback retains failed leases so callers can retry cleanup.
func (m *Manager) rollback(ctx context.Context, resources *PreparedResources, cause error) (*RuntimeConfig, *PreparedResources, error) {
	cleanupCtx, cancel := cleanup.NewContext(ctx)
	defer cancel()
	releaseErr := m.Release(cleanupCtx, resources)
	if releaseErr != nil {
		return nil, resources, errors.Join(cause, releaseErr)
	}
	return nil, nil, cause
}
