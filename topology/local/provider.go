package local

import (
	"context"
	"errors"
	"time"

	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt/container"
	"github.com/tethux/tethux/virt/container/containerd"
	"github.com/tethux/tethux/virt/container/docker"
	"github.com/tethux/tethux/virt/container/podman"
)

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
