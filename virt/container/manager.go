package container

import (
	"context"
	"errors"
	"fmt"

	"github.com/tethux/tethux/internal/cleanup"
	"github.com/tethux/tethux/storage"
	containererrs "github.com/tethux/tethux/virt/container/errs"
)

// Manager prepares and releases storage used by container workloads.
type Manager struct {
	storage storage.Manager
}

// NewManager creates a Manager using storageManager for volume preparation.
func NewManager(storageManager storage.Manager) *Manager {
	return &Manager{
		storage: storageManager,
	}
}

// PreparedResources tracks resources that must be released after a workload.
type PreparedResources struct {
	Volumes []*storage.Prepared
}

// Prepare resolves a portable container configuration into runtime resources.
// On error, returned resources remain owned by the caller.
func (m *Manager) Prepare(
	ctx context.Context,
	cfg *ContainerConfig,
) (*RuntimeConfig, *PreparedResources, error) {
	if m == nil || m.storage == nil || cfg == nil {
		return nil, nil, containererrs.New("manager", containererrs.ErrInvalidConfig, "storage manager and config are required")
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
			return m.rollback(ctx, resources, containererrs.Wrap("manager", containererrs.ErrInvalidConfig, volume.Source.String(), err))
		}

		resources.Volumes = append(resources.Volumes, prepared)

		if prepared.Location.Kind != storage.LocationPath {
			return m.rollback(ctx, resources, containererrs.New("manager", containererrs.ErrInvalidConfig, fmt.Sprintf("bind mount %s resolved to %q", volume.Source, prepared.Location.Kind)))
		}

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

// Release releases all storage recorded in resources.
func (m *Manager) Release(
	ctx context.Context,
	resources *PreparedResources,
) error {
	if resources == nil {
		return nil
	}

	remaining, releaseErr := storage.ReleaseResources(ctx, m.storage, resources.Volumes)
	resources.Volumes = remaining
	return releaseErr
}

// Create prepares cfg and asks p to create the container.
// On error, the caller owns returned nodes and resources; delete nodes before release.
func (m *Manager) Create(
	ctx context.Context,
	p ContainerProvider,
	cfg *ContainerConfig,
) (*ContainerNode, *PreparedResources, error) {
	if p == nil {
		return nil, nil, containererrs.New("manager", containererrs.ErrInvalidConfig, "provider is nil")
	}
	runtimeCfg, resources, err := m.Prepare(ctx, cfg)
	if err != nil {
		return nil, resources, err
	}

	node, err := p.CreateContainer(ctx, runtimeCfg)
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
