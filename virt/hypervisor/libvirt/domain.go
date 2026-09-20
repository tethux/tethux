package libvirt

import (
	"context"
	"errors"
	"io"

	libvirtgo "libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

func (p *Provider) CreateDomain(ctx context.Context, cfg *domain.RuntimeConfig) (*domain.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	xmlConfig, xmlErr := domainXML(cfg)
	if xmlErr != nil {
		return nil, xmlErr
	}

	domainRef, defineErr := p.conn.DomainDefineXML(xmlConfig)
	if defineErr != nil {
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, defineErr)
	}
	defer func() { _ = domainRef.Free() }()

	createErr := domainRef.Create()
	if createErr != nil {
		_ = domainRef.Undefine()
		return nil, errs.Wrap(errs.ErrCreate, cfg.Name, createErr)
	}
	if err := ctx.Err(); err != nil {
		_ = domainRef.Destroy()
		_ = domainRef.Undefine()
		return nil, err
	}

	node, inspectErr := p.inspect(domainRef)
	if inspectErr == nil {
		p.rememberManaged(node.ID)
	}
	return node, inspectErr
}

func (p *Provider) InspectDomain(ctx context.Context, id string) (*domain.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

	node, inspectErr := p.inspect(domainRef)
	if inspectErr == nil {
		p.rememberManaged(node.ID)
	}
	return node, inspectErr
}

func (p *Provider) Reload(ctx context.Context, id string) (*virt.Node, error) {
	node, err := p.InspectDomain(ctx, id)
	if err != nil {
		return nil, err
	}
	return &node.Node, nil
}

func (p *Provider) List(ctx context.Context) ([]*virt.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	domains, listErr := p.conn.ListAllDomains(
		libvirtgo.CONNECT_LIST_DOMAINS_ACTIVE |
			libvirtgo.CONNECT_LIST_DOMAINS_INACTIVE,
	)
	if listErr != nil {
		return nil, errs.Wrap(errs.ErrList, "", listErr)
	}
	defer func() {
		for index := range domains {
			_ = domains[index].Free()
		}
	}()

	result := make([]*virt.Node, 0, len(domains))

	for index := range domains {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		domainRef := &domains[index]

		managed, metadataErr := isManaged(domainRef)
		if metadataErr != nil {
			return nil, metadataErr
		}

		if !managed {
			continue
		}

		node, inspectErr := p.inspect(domainRef)

		if inspectErr != nil {
			return nil, inspectErr
		}
		p.rememberManaged(node.ID)

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
	xmlDescription, err := domainRef.GetXMLDesc(0)
	if err != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, err)
	}
	var description libvirtxml.Domain
	if err := description.Unmarshal(xmlDescription); err != nil {
		return nil, errs.Wrap(errs.ErrInspect, id, err)
	}

	node := &domain.Node{
		Node: virt.Node{
			ID: id, Name: name, State: nodeState(state),
			Console: virt.Console{Type: virt.ConsoleSerial},
		},
		UUID: id, Persistent: true,
	}
	if description.Devices == nil {
		return node, nil
	}

	for index := range description.Devices.Disks {
		disk := &description.Devices.Disks[index]
		if disk.Target == nil || disk.Source == nil {
			continue
		}
		source := ""
		if disk.Source.File != nil {
			source = disk.Source.File.File
		} else if disk.Source.Block != nil {
			source = disk.Source.Block.Dev
		}
		node.Disks = append(node.Disks, domain.DiskInfo{Target: disk.Target.Dev, Source: source})
	}
	for index := range description.Devices.Interfaces {
		iface := &description.Devices.Interfaces[index]
		info := domain.InterfaceInfo{}
		if iface.MAC != nil {
			info.MAC = iface.MAC.Address
		}
		if iface.Target != nil {
			info.Target = iface.Target.Dev
		}
		node.Interfaces = append(node.Interfaces, info)
	}
	for _, graphics := range description.Devices.Graphics {
		if graphics.Spice == nil || graphics.Spice.Port <= 0 {
			continue
		}
		host := graphics.Spice.Listen
		if host == "" {
			host = "127.0.0.1"
		}
		if graphics.Spice.Port <= 65535 {
			port := uint16(graphics.Spice.Port) // #nosec G115 -- guarded by the uint16 upper bound.
			node.Aux = &virt.Console{Type: virt.ConsoleSpice, Host: host, Port: port}
		}
		break
	}

	return node, nil
}

func (p *Provider) OpenConsole(
	ctx context.Context,
	id string,
	input io.Reader,
	output io.Writer,
) error {
	if output == nil {
		return errs.New(errs.ErrConsole, id+": output is nil")
	}
	domainRef, err := p.lookup(id)
	if err != nil {
		return err
	}
	defer func() { _ = domainRef.Free() }()

	stream, err := p.conn.NewStream(0)
	if err != nil {
		return errs.Wrap(errs.ErrConsole, id, err)
	}
	freeStream := true
	defer func() {
		_ = stream.Abort()
		if freeStream {
			_ = stream.Free()
		}
	}()
	if err := domainRef.OpenConsole("", stream, libvirtgo.DOMAIN_CONSOLE_FORCE); err != nil {
		return errs.Wrap(errs.ErrConsole, id, err)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Abort()
		case <-done:
		}
	}()
	var inputDone chan struct{}
	if input != nil {
		inputDone = make(chan struct{})
		go func() {
			defer close(inputDone)
			copyConsoleInput(stream, input)
		}()
	}
	defer func() {
		if inputDone == nil {
			return
		}
		if closer, ok := input.(io.Closer); ok {
			_ = closer.Close()
			<-inputDone
			return
		}
		select {
		case <-inputDone:
		default:
			// The caller supplied a blocking reader that cannot be canceled.
			// Keep the stream allocated so that the input goroutine cannot use
			// a freed libvirt handle.
			freeStream = false
		}
	}()

	buffer := make([]byte, 32*1024)
	for {
		count, recvErr := stream.Recv(buffer)
		if count > 0 {
			if _, writeErr := output.Write(buffer[:count]); writeErr != nil {
				_ = stream.Abort()
				return errs.Wrap(errs.ErrConsole, id, writeErr)
			}
		}
		if recvErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(recvErr, io.EOF) {
				return nil
			}
			return errs.Wrap(errs.ErrConsole, id, recvErr)
		}
	}
}

func copyConsoleInput(stream *libvirtgo.Stream, input io.Reader) {
	buffer := make([]byte, 32*1024)
	for {
		count, err := input.Read(buffer)
		if count > 0 {
			remaining := buffer[:count]
			for len(remaining) > 0 {
				written, sendErr := stream.Send(remaining)
				if sendErr != nil || written <= 0 {
					return
				}
				remaining = remaining[written:]
			}
		}
		if err != nil {
			return
		}
	}
}
