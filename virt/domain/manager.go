package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/tethux/tethux/storage"
)

type Manager struct {
	storage storage.Manager
}

func NewManager(storageManager storage.Manager) *Manager {
	return &Manager{
		storage: storageManager,
	}
}

type PreparedResources struct {
	Disks []*storage.Prepared
}

func (m *Manager) Prepare(
	ctx context.Context,
	cfg *Config,
) (*RuntimeConfig, *PreparedResources, error) {
	if m == nil || m.storage == nil {
		return nil, nil, errors.New("domain storage manager is not configured")
	}
	if cfg == nil {
		return nil, nil, errors.New("domain configuration is nil")
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
			releaseErr := m.Release(context.WithoutCancel(ctx), resources)
			return nil, nil, errors.Join(err, releaseErr)
		}

		if prepared.Location.Kind != storage.LocationPath {
			releaseErr := m.Release(context.WithoutCancel(ctx), resources)
			locationErr := fmt.Errorf(
				"domain disk %s resolved to unsupported location %q",
				disk.Source,
				prepared.Location.Kind,
			)
			return nil, nil, errors.Join(locationErr, releaseErr)
		}

		resources.Disks = append(resources.Disks, prepared)

		disks = append(disks, RuntimeDisk{
			Source:   prepared.Location.Value,
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

func (m *Manager) Release(
	ctx context.Context,
	resources *PreparedResources,
) error {
	if resources == nil {
		return nil
	}

	var errs []error

	for _, disk := range resources.Disks {
		if err := m.storage.Release(ctx, disk); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *Manager) Create(
	ctx context.Context,
	provider Provider,
	cfg *Config,
) (*Node, *PreparedResources, error) {
	if provider == nil {
		return nil, nil, errors.New("domain provider is nil")
	}
	runtimeCfg, resources, err := m.Prepare(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	node, err := provider.CreateDomain(ctx, runtimeCfg)
	if err != nil {
		releaseErr := m.Release(context.WithoutCancel(ctx), resources)
		return nil, nil, errors.Join(err, releaseErr)
	}

	return node, resources, nil
}
