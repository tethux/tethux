package libvirt

import (
	"context"
	"errors"
	"time"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func (p *Provider) lookup(id string) (*libvirtgo.Domain, error) {
	domainRef, lookupErr := p.conn.LookupDomainByUUIDString(id)
	if lookupErr != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, lookupErr)
	}

	managed, metadataErr := isManaged(domainRef)
	if metadataErr != nil {
		return nil, errors.Join(metadataErr, releaseDomainRef(id, domainRef))
	}

	if !managed {
		return nil, errors.Join(errs.New(errs.ErrNotManaged, id), releaseDomainRef(id, domainRef))
	}

	return domainRef, nil
}

func (p *Provider) lifecycle(ctx context.Context, id string, operation func(*libvirtgo.Domain) error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, releaseDomainRef(id, domain)) }()
	if operationErr := operation(domain); operationErr != nil {
		return errs.Wrap(errs.ErrLifecycle, id, operationErr)
	}
	return nil
}

// Start boots a stopped managed domain.
func (p *Provider) Start(ctx context.Context, id string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, releaseDomainRef(id, domain)) }()
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

// Stop requests an ACPI shutdown and waits for the domain to stop.
func (p *Provider) Stop(ctx context.Context, id string) (resultErr error) {
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, releaseDomainRef(id, domain)) }()
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

// PowerOff immediately stops a managed domain.
func (p *Provider) PowerOff(ctx context.Context, id string) error {
	return p.lifecycle(ctx, id, (*libvirtgo.Domain).Destroy)
}

// Suspend pauses a running managed domain.
func (p *Provider) Suspend(ctx context.Context, id string) error {
	return p.lifecycle(ctx, id, (*libvirtgo.Domain).Suspend)
}

// Resume continues a suspended managed domain.
func (p *Provider) Resume(ctx context.Context, id string) error {
	return p.lifecycle(ctx, id, (*libvirtgo.Domain).Resume)
}

// Restart requests a guest reboot for a managed domain.
func (p *Provider) Restart(ctx context.Context, id string) error {
	return p.lifecycle(ctx, id, func(domain *libvirtgo.Domain) error {
		return domain.Reboot(libvirtgo.DOMAIN_REBOOT_DEFAULT)
	})
}

// Delete stops and undefines a managed domain.
func (p *Provider) Delete(ctx context.Context, id string) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	domain, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, releaseDomainRef(id, domain)) }()
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

// State returns the normalized lifecycle state of a managed domain.
func (p *Provider) State(ctx context.Context, id string) (stateResult virt.NodeState, resultErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	domain, err := p.lookup(id)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, releaseDomainRef(id, domain)) }()
	state, _, stateErr := domain.GetState()
	if stateErr != nil {
		return "", errs.Wrap(errs.ErrInspect, id, stateErr)
	}
	return nodeState(state), nil
}

func releaseDomainRef(id string, reference *libvirtgo.Domain) error {
	releaseErr := reference.Free()
	if releaseErr != nil {
		return errs.Wrap(errs.ErrLifecycle, id, releaseErr)
	}
	return nil
}
