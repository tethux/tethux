package local

import (
	"context"
	"errors"
	"time"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/container"
	"github.com/tethux/tethux/virt/container/containerd"
	"github.com/tethux/tethux/virt/container/docker"
	"github.com/tethux/tethux/virt/container/podman"
)

func (options *Options) containerProvider(name string) (container.ContainerProvider, error) {
	if name == "" && options.Provider != nil {
		return options.Provider, nil
	}
	if provider := options.Containers[name]; provider != nil {
		return provider, nil
	}
	if options.Provider != nil && options.Provider.Info().Name == name {
		return options.Provider, nil
	}
	return nil, errs.New("select container provider", errs.ErrUnsupported, name)
}

// ContainerProvider returns the provider selected for a container node.
func (run *Running) ContainerProvider(id topology.NodeID) (container.ContainerProvider, error) {
	provider, exists := run.providers[id]
	if !exists {
		return nil, errs.New("find container provider", errs.ErrUnsupported, string(id))
	}
	return provider, nil
}

// Node resolves a logical node to its provider's refreshed runtime snapshot.
func (run *Running) Node(ctx context.Context, id topology.NodeID) (*virt.Node, error) {
	runtimeID, exists := run.nodes[id]
	if !exists {
		return nil, errs.New("find topology node", errs.ErrNotFound, string(id))
	}
	if resources, isDomain := run.domains[id]; isDomain {
		return resources.provider.Reload(ctx, runtimeID)
	}
	if _, isRouter := run.routers[id]; isRouter {
		return run.dynamips.Reload(ctx, runtimeID)
	}
	if _, isEndpoint := run.endpoints[id]; isEndpoint {
		return run.shitnet.Reload(ctx, runtimeID)
	}
	return run.providers[id].Reload(ctx, runtimeID)
}

// SelectContainerProvider selects a reachable local container provider.
// The name auto tries Docker before Podman. Explicit names never fall back.
func SelectContainerProvider(ctx context.Context, name string) (container.ContainerProvider, error) {
	names := []string{name}
	if name == "auto" {
		names = []string{"docker", "podman"}
	}
	var failures []error
	for _, candidate := range names {
		var provider container.ContainerProvider
		var err error
		switch candidate {
		case "docker":
			provider, err = docker.New()
		case "podman":
			provider, err = podman.New()
		case "containerd":
			provider, err = containerd.New()
		default:
			return nil, errs.New("select topology provider", errs.ErrUnsupported, "choose auto, docker, podman, or containerd")
		}
		if err == nil {
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err = provider.List(probeCtx)
			cancel()
		}
		if err == nil {
			return provider, nil
		}
		failures = append(failures, err)
	}
	return nil, errs.Wrap("select topology provider", errs.ErrStart, "no local container provider is available", errors.Join(failures...))
}
