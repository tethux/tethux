package libvirt

import (
	"context"
	"time"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func (p *Provider) lookup(id string) (*libvirtgo.Domain, error) {
	domain, err := p.conn.LookupDomainByUUIDString(id)
	if err == nil {
		return domain, nil
	}
	domain, nameErr := p.conn.LookupDomainByName(id)
	if nameErr != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, nameErr)
	}
	return domain, nil
}

func (p *Provider) lifecycle(id string, operation func(*libvirtgo.Domain) error) error {
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { _ = domain.Free() }()
	if operationErr := operation(domain); operationErr != nil {
		return errs.Wrap(errs.ErrLifecycle, id, operationErr)
	}
	return nil
}

func (p *Provider) Start(_ context.Context, id string) error {
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { _ = domain.Free() }()
	state, _, stateErr := domain.GetState()
	if stateErr != nil {
		return errs.Wrap(errs.ErrInspect, id, stateErr)
	}
	if nodeState(state) == virt.NodeRunning {
		return nil
	}
	createErr := domain.Create()
	if createErr != nil {
		return errs.Wrap(errs.ErrLifecycle, id, createErr)
	}
	return nil
}

func (p *Provider) Stop(ctx context.Context, id string) error {
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { _ = domain.Free() }()
	state, _, stateErr := domain.GetState()
	if stateErr != nil {
		return errs.Wrap(errs.ErrInspect, id, stateErr)
	}
	if nodeState(state) == virt.NodeStopped {
		return nil
	}
	shutdownErr := domain.Shutdown()
	if shutdownErr != nil {
		return errs.Wrap(errs.ErrLifecycle, id, shutdownErr)
	}
	return waitStopped(ctx, domain, id)
}

func (p *Provider) PowerOff(_ context.Context, id string) error {
	return p.lifecycle(id, (*libvirtgo.Domain).Destroy)
}

func (p *Provider) Suspend(_ context.Context, id string) error {
	return p.lifecycle(id, (*libvirtgo.Domain).Suspend)
}

func (p *Provider) Resume(_ context.Context, id string) error {
	return p.lifecycle(id, (*libvirtgo.Domain).Resume)
}

func (p *Provider) Restart(_ context.Context, id string) error {
	return p.lifecycle(id, func(domain *libvirtgo.Domain) error {
		return domain.Reboot(libvirtgo.DOMAIN_REBOOT_DEFAULT)
	})
}

func (p *Provider) Delete(_ context.Context, id string) error {
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { _ = domain.Free() }()
	persistent, persistentErr := domain.IsPersistent()
	if persistentErr != nil {
		return errs.Wrap(errs.ErrDelete, id, persistentErr)
	}
	if !persistent {
		destroyErr := domain.Destroy()
		if destroyErr != nil {
			return errs.Wrap(errs.ErrDelete, id, destroyErr)
		}
		return nil
	}
	state, _, stateErr := domain.GetState()
	if stateErr != nil {
		return errs.Wrap(errs.ErrInspect, id, stateErr)
	}
	if nodeState(state) != virt.NodeStopped {
		destroyErr := domain.Destroy()
		if destroyErr != nil {
			return errs.Wrap(errs.ErrDelete, id, destroyErr)
		}
	}
	undefineErr := domain.Undefine()
	if undefineErr != nil {
		return errs.Wrap(errs.ErrDelete, id, undefineErr)
	}
	return nil
}

func waitStopped(ctx context.Context, domain *libvirtgo.Domain, id string) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		state, _, stateErr := domain.GetState()
		if stateErr != nil {
			return errs.Wrap(errs.ErrInspect, id, stateErr)
		}
		if nodeState(state) == virt.NodeStopped {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errs.New(errs.ErrLifecycle, id+": shutdown timed out")
		case <-ticker.C:
		}
	}
}

func (p *Provider) State(_ context.Context, id string) (virt.NodeState, error) {
	domain, err := p.lookup(id)
	if err != nil {
		return "", err
	}
	defer func() { _ = domain.Free() }()
	state, _, stateErr := domain.GetState()
	if stateErr != nil {
		return "", errs.Wrap(errs.ErrInspect, id, stateErr)
	}
	return nodeState(state), nil
}
