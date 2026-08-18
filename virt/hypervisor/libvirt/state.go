package libvirt

import (
	"github.com/tethux/tethux/virt"
	libvirtgo "libvirt.org/go/libvirt"
)

func nodeState(state libvirtgo.DomainState) virt.NodeState {
	switch state {
	case libvirtgo.DOMAIN_RUNNING, libvirtgo.DOMAIN_BLOCKED:
		return virt.NodeRunning
	case libvirtgo.DOMAIN_PAUSED, libvirtgo.DOMAIN_PMSUSPENDED:
		return virt.NodeSuspended
	case libvirtgo.DOMAIN_SHUTDOWN:
		return virt.NodeStopping
	default:
		return virt.NodeStopped
	}
}
