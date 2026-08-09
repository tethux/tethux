package container

import (
	"context"
	"errors"
	"fmt"

	"github.com/tethux/tethux/internal/libtethux/storage"
	containererrs "github.com/tethux/tethux/internal/libtethux/virt/container/errs"
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
	Volumes []*storage.Prepared
}

func (m *Manager) Prepare(
	ctx context.Context,
	cfg *ContainerConfig,
) (*RuntimeConfig, *PreparedResources, error) {
	if cfg == nil {
		return nil, nil, containererrs.New("manager", containererrs.ErrInvalidConfig, "config is nil")
	}

	runtimeVolumes := make(
		[]RuntimeVolumeMount,
		0,
		len(cfg.Volumes),
	)

	resources := &PreparedResources{
		Volumes: make(
			[]*storage.Prepared,
			0,
			len(cfg.Volumes),
		),
	}

	for _, volume := range cfg.Volumes {
		accessMode := storage.AccessReadWrite

		if volume.ReadOnly {
			accessMode = storage.AccessReadOnly
		}

		prepared, err := m.storage.Prepare(
			ctx,
			storage.PrepareRequest{
				Ref:        volume.Source,
				NodeID:     cfg.ID,
				AccessMode: accessMode,
			},
		)
		if err != nil {
			_ = m.Release(ctx, resources)

			return nil, nil, containererrs.Wrap("manager", containererrs.ErrInvalidConfig, volume.Source.String(), err)
		}

		if prepared.Location.Kind != storage.LocationPath {
			_ = m.Release(ctx, resources)

			return nil, nil, containererrs.New("manager", containererrs.ErrInvalidConfig, fmt.Sprintf("bind mount %s resolved to %q", volume.Source, prepared.Location.Kind))
		}

		resources.Volumes = append(
			resources.Volumes,
			prepared,
		)

		runtimeVolumes = append(
			runtimeVolumes,
			RuntimeVolumeMount{
				Source:   prepared.Location.Value,
				Target:   volume.Target,
				ReadOnly: volume.ReadOnly,
			},
		)
	}

	return &RuntimeConfig{
		NodeConfig: cfg.NodeConfig,

		Image: cfg.Image,

		Entrypoint: cfg.Entrypoint,
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,

		Volumes: runtimeVolumes,

		CapAdd:      cfg.CapAdd,
		CapDrop:     cfg.CapDrop,
		Privileged:  cfg.Privileged,
		NetworkMode: cfg.NetworkMode,

		Hostname:   cfg.Hostname,
		DNS:        cfg.DNS,
		ExtraHosts: cfg.ExtraHosts,

		Labels: cfg.Labels,
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

	for _, volume := range resources.Volumes {
		if err := m.storage.Release(ctx, volume); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *Manager) Create(
	ctx context.Context,
	p ContainerProvider,
	cfg *ContainerConfig,
) (*ContainerNode, *PreparedResources, error) {
	runtimeCfg, resources, err := m.Prepare(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	node, err := p.CreateContainer(ctx, runtimeCfg)
	if err != nil {
		_ = m.Release(ctx, resources)
		return nil, nil, err
	}
	return node, resources, nil
}
