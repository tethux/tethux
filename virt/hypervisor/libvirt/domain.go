package libvirt

import (
	"context"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func (p *Provider) CreateDomain(_ context.Context, cfg *domain.RuntimeConfig) (*domain.Node, error) {
	xmlConfig, err := domainXML(cfg)
	if err != nil {
		return nil, err
	}
	domainRef, err := p.conn.DomainDefineXML(xmlConfig)
	if err != nil {
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, err)
	}
	defer func() { _ = domainRef.Free() }()
	createErr := domainRef.Create()
	if createErr != nil {
		_ = domainRef.Undefine()
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, createErr)
	}
	return p.inspect(domainRef)
}

func (p *Provider) InspectDomain(_ context.Context, id string) (*domain.Node, error) {
	domainRef, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = domainRef.Free() }()
	return p.inspect(domainRef)
}

func (p *Provider) Reload(ctx context.Context, id string) (*virt.Node, error) {
	node, err := p.InspectDomain(ctx, id)
	if err != nil {
		return nil, err
	}
	return &node.Node, nil
}

func (p *Provider) List(ctx context.Context) ([]*virt.Node, error) {
	domains, err := p.conn.ListAllDomains(libvirtgo.CONNECT_LIST_DOMAINS_ACTIVE | libvirtgo.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, errs.Wrap(errs.ErrList, "", err)
	}
	result := make([]*virt.Node, 0, len(domains))
	for index := range domains {
		node, inspectErr := p.inspect(&domains[index])
		_ = domains[index].Free()
		if inspectErr != nil {
			return nil, inspectErr
		}
		result = append(result, &node.Node)
	}
	return result, nil
}

func (p *Provider) inspect(domainRef *libvirtgo.Domain) (*domain.Node, error) {
	id, err := domainRef.GetUUIDString()
	if err != nil {
		return nil, errs.Wrap(errs.ErrInspect, "uuid", err)
	}
	name, err := domainRef.GetName()
	if err != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, err)
	}
	state, _, err := domainRef.GetState()
	if err != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, err)
	}
	return &domain.Node{
		Node: virt.Node{ID: id, Name: name, State: nodeState(state)},
		UUID: id, Persistent: true,
	}, nil
}
