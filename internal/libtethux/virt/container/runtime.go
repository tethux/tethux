package container

import (
	"net/netip"

	"github.com/tethux/tethux/internal/libtethux/virt"
)

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

type RuntimeVolumeMount struct {
	Source string
	Target string

	ReadOnly bool
}
