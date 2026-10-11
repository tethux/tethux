package dynamips

import (
	"context"
	"errors"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

// Start boots a stopped router. Starting a running router is safe.
func (a *Adapter) Start(ctx context.Context, id string) error {
	state, stateErr := a.State(ctx, id)
	if stateErr != nil {
		return stateErr
	}
	if state == virt.NodeRunning {
		return nil
	}
	return a.changeState(ctx, id, "start router", func(router *Router) error { return router.VM.Start() })
}

// Stop stops guest execution while retaining native state files.
func (a *Adapter) Stop(ctx context.Context, id string) error {
	state, stateErr := a.State(ctx, id)
	if stateErr != nil {
		return stateErr
	}
	if state == virt.NodeStopped {
		return nil
	}
	return a.changeState(ctx, id, "stop router", func(router *Router) error { return router.VM.Stop() })
}

// Suspend pauses guest execution.
func (a *Adapter) Suspend(ctx context.Context, id string) error {
	return a.changeState(ctx, id, "suspend router", func(router *Router) error { return router.VM.Suspend() })
}

// Resume resumes a suspended guest.
func (a *Adapter) Resume(ctx context.Context, id string) error {
	return a.changeState(ctx, id, "resume router", func(router *Router) error { return router.VM.Resume() })
}

// Restart stops the router before booting it again.
func (a *Adapter) Restart(ctx context.Context, id string) error {
	if stopErr := a.Stop(ctx, id); stopErr != nil {
		return stopErr
	}
	return a.Start(ctx, id)
}

func (a *Adapter) changeState(ctx context.Context, id, operation string, change func(*Router) error) error {
	router, lookupErr := a.router(ctx, id)
	if lookupErr != nil {
		return lookupErr
	}
	changeErr := change(router)
	_, stateErr := a.State(context.WithoutCancel(ctx), id)
	if changeErr != nil {
		return errors.Join(errs.Wrap(operation, errs.ErrLifecycle, id, changeErr), stateErr)
	}
	return stateErr
}

// Delete releases a router and its attachments while retaining its state files.
// Failed cleanup remains tracked for retry. Deleting an absent router is safe.
func (a *Adapter) Delete(ctx context.Context, id string) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return errs.Wrap("delete router", errs.ErrCleanup, id, contextErr)
	}
	if a == nil {
		return nil
	}
	router, exists := a.Routers[id]
	if !exists {
		return nil
	}
	if cleanupErr := cleanupRouter(router); cleanupErr != nil {
		return cleanupErr
	}
	delete(a.Routers, id)
	return nil
}

// Close deletes owned routers without closing the caller-owned runtime.
func (a *Adapter) Close() error {
	if a == nil {
		return nil
	}
	var failures []error
	for id := range a.Routers {
		if deleteErr := a.Delete(context.Background(), id); deleteErr != nil {
			failures = append(failures, deleteErr)
		}
	}
	return errors.Join(failures...)
}
