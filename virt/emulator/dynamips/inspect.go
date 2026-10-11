package dynamips

import (
	"context"
	"sort"

	native "github.com/tethux/dynamips-downgrade"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func (a *Adapter) router(ctx context.Context, id string) (*Router, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, errs.Wrap("find router", errs.ErrLifecycle, id, contextErr)
	}
	if a == nil {
		return nil, errs.New("find router", errs.ErrRuntime, id)
	}
	router, exists := a.Routers[id]
	if !exists || router.VM == nil {
		return nil, errs.New("find router", errs.ErrNotFound, id)
	}
	return router, nil
}

// State reads the native execution state of a managed router.
func (a *Adapter) State(ctx context.Context, id string) (virt.NodeState, error) {
	router, lookupErr := a.router(ctx, id)
	if lookupErr != nil {
		return "", lookupErr
	}
	status, statusErr := router.VM.Status()
	if statusErr != nil {
		return "", errs.Wrap("read router state", errs.ErrLifecycle, id, statusErr)
	}
	var state virt.NodeState
	switch status {
	case native.VMHalted, native.VMShutdown:
		state = virt.NodeStopped
	case native.VMRunning:
		state = virt.NodeRunning
	case native.VMSuspended:
		state = virt.NodeSuspended
	default:
		return "", errs.New("read router state", errs.ErrLifecycle, "unknown native status")
	}
	router.Node.State = state
	return state, nil
}

// Reload refreshes a router's runtime node from native state.
func (a *Adapter) Reload(ctx context.Context, id string) (*virt.Node, error) {
	if _, stateErr := a.State(ctx, id); stateErr != nil {
		return nil, stateErr
	}
	node := a.Routers[id].Node.Node
	return &node, nil
}

// List returns managed routers sorted by ID with refreshed native state.
func (a *Adapter) List(ctx context.Context) ([]*virt.Node, error) {
	if a == nil {
		return nil, errs.New("list routers", errs.ErrRuntime, "")
	}
	ids := make([]string, 0, len(a.Routers))
	for id := range a.Routers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes := make([]*virt.Node, 0, len(ids))
	for _, id := range ids {
		node, reloadErr := a.Reload(ctx, id)
		if reloadErr != nil {
			return nil, reloadErr
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
