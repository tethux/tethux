package moby

import (
	"github.com/tethux/tethux/virt"

	moby "github.com/moby/moby/api/types/container"
)

func mapInspectState(state *moby.State) virt.NodeState {
	switch {
	case state.Paused:
		return virt.NodeSuspended
	case state.Restarting:
		return virt.NodeStarting
	case state.Running:
		return virt.NodeRunning
	case state.Status == moby.StateRemoving:
		return virt.NodeStopping
	default:
		return virt.NodeStopped
	}
}

func mapState(state moby.ContainerState) virt.NodeState {
	switch state {
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
