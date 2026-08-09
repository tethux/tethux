package moby

import (
	"github.com/tethux/tethux/virt"

	moby "github.com/moby/moby/api/types/container"
)

type stateInput interface {
	*moby.State | moby.ContainerState
}

func mapState[T stateInput](s T) virt.NodeState {
	switch v := any(s).(type) {
	case *moby.State:
		switch {
		case v.Paused:
			return virt.NodeSuspended
		case v.Restarting:
			return virt.NodeStarting
		case v.Running:
			return virt.NodeRunning
		case v.Status == moby.StateRemoving:
			return virt.NodeStopping
		default:
			return virt.NodeStopped
		}
	case moby.ContainerState:
		switch v {
		case moby.StateRunning:
			return virt.NodeRunning
		case moby.StatePaused:
			return virt.NodeSuspended
		case moby.StateRestarting:
			return virt.NodeStarting
		case moby.StateRemoving:
			return virt.NodeStopping
		default:
			return virt.NodeStopped
		}
	}
	return virt.NodeStopped
}
