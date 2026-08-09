package container

import (
	"net/netip"

	"github.com/tethux/tethux/virt"
)

// RuntimeConfig is a container configuration with runtime-ready resources.
type RuntimeConfig struct {
	virt.NodeConfig

	Image Image

	Entrypoint []string
	Cmd        []string
	Env        []string

	Volumes []RuntimeVolumeMount

	CapAdd      []string
	CapDrop     []string
	Privileged  bool
	NetworkMode string

	Hostname   string
	DNS        []netip.Addr
	ExtraHosts []string

	Labels map[string]string
}

// RuntimeVolumeMount maps a prepared host path into a container.
type RuntimeVolumeMount struct {
	Source string
	Target string

	ReadOnly bool
}
