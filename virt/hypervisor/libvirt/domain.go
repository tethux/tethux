package libvirt

import (
	"context"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func (p *Provider) CreateDomain(_ context.Context, cfg *domain.RuntimeConfig) (*domain.Node, error) {
	xmlConfig, xmlErr := domainXML(cfg)
	if xmlErr != nil {
		return nil, xmlErr
	}

	domainRef, defineErr := p.conn.DomainDefineXML(xmlConfig)
	if defineErr != nil {
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, defineErr)
	}
	defer func() { _ = domainRef.Free() }()

	metadataErr := markManaged(domainRef)
	if metadataErr != nil {
		_ = domainRef.Undefine()
		return nil, metadataErr
	}

	createErr := domainRef.Create()
	if createErr != nil {
		_ = domainRef.Undefine()
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, createErr)
	}

	return p.inspect(domainRef)
}

func (p *Provider) InspectDomain(_ context.Context, id string) (*domain.Node, error) {
	domainRef, lookupErr := p.lookup(id)
	if lookupErr != nil {
		return nil, lookupErr
	}
	defer func() { _ = domainRef.Free() }()

	managed, metadataErr := isManaged(domainRef)
	if metadataErr != nil {
		return nil, metadataErr
	}

	if !managed {
		return nil, errs.New(errs.ErrNotManaged, id)
	}

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
	domains, listErr := p.conn.ListAllDomains(
		libvirtgo.CONNECT_LIST_DOMAINS_ACTIVE |
			libvirtgo.CONNECT_LIST_DOMAINS_INACTIVE,
	)
	if listErr != nil {
		return nil, errs.Wrap(errs.ErrList, "", listErr)
	}

	result := make([]*virt.Node, 0, len(domains))

	for index := range domains {
		domainRef := &domains[index]

		managed, metadataErr := isManaged(domainRef)
		if metadataErr != nil {
			_ = domainRef.Free()
			return nil, metadataErr
		}

		if !managed {
			_ = domainRef.Free()
			continue
		}

		node, inspectErr := p.inspect(domainRef)
		_ = domainRef.Free()

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
