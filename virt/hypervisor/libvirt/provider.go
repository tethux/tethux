// Package libvirt provides a local libvirt-backed domain provider.
package libvirt

import (
	"context"
	"errors"
	"sync"
	"time"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/domain"
	"github.com/tethux/tethux/virt/hypervisor/libvirt/errs"
)

// Provider manages tethux-owned domains through a libvirt connection.
type Provider struct {
	conn       *libvirtgo.Connect
	callbackID int

	mu          sync.Mutex
	nextSubID   uint64
	subscribers map[uint64]chan virt.Event
	managed     map[string]struct{}
}

var (
	_ domain.Provider        = (*Provider)(nil)
	_ domain.ConsoleProvider = (*Provider)(nil)
	_ virt.EventSource       = (*Provider)(nil)
)

var (
	eventLoopOnce sync.Once
	eventLoopErr  error
)

// New connects to uri and starts lifecycle event delivery.
func New(uri string) (*Provider, error) {
	startEventLoop()
	if eventLoopErr != nil {
		return nil, errs.Wrap(errs.ErrEvents, uri, eventLoopErr)
	}

	conn, err := libvirtgo.NewConnect(uri)
	if err != nil {
		return nil, errs.Wrap(errs.ErrConnect, uri, err)
	}
	p := &Provider{
		conn:        conn,
		callbackID:  -1,
		subscribers: make(map[uint64]chan virt.Event),
		managed:     make(map[string]struct{}),
	}
	callbackID, callbackErr := conn.DomainEventLifecycleRegister(nil, p.handleLifecycleEvent)
	if callbackErr != nil {
		_, _ = conn.Close()
		return nil, errs.Wrap(errs.ErrEvents, uri, callbackErr)
	}
	p.callbackID = callbackID
	return p, nil
}

// Info reports the libvirt provider's supported operations.
func (p *Provider) Info() virt.ProviderInfo {
	return virt.ProviderInfo{
		Name: "libvirt", DisplayName: "libvirt", Kind: virt.ProviderKindDomain,
		Capabilities: virt.Capabilities{Console: true, AuxConsole: true, Pause: true, Events: true},
	}
}

// Close deregisters lifecycle events and closes the libvirt connection.
func (p *Provider) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	var deregisterErr error
	if p.callbackID >= 0 {
		if err := p.conn.DomainEventDeregister(p.callbackID); err != nil {
			deregisterErr = errs.Wrap(errs.ErrEvents, "deregister", err)
		}
		p.callbackID = -1
	}
	_, closeErr := p.conn.Close()
	return errors.Join(deregisterErr, closeErr)
}

func startEventLoop() {
	eventLoopOnce.Do(func() {
		eventLoopErr = libvirtgo.EventRegisterDefaultImpl()
		if eventLoopErr != nil {
			return
		}
		go func() {
			for {
				if err := libvirtgo.EventRunDefaultImpl(); err != nil {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}()
	})
}

// Events subscribes to lifecycle changes for domains managed by this provider.
// The channel closes when ctx is canceled.
func (p *Provider) Events(ctx context.Context) (<-chan virt.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := make(chan virt.Event, 16)
	sub := make(chan virt.Event, 16)
	p.mu.Lock()
	id := p.nextSubID
	p.nextSubID++
	p.subscribers[id] = sub
	p.mu.Unlock()

	go func() {
		defer close(out)
		defer func() {
			p.mu.Lock()
			delete(p.subscribers, id)
			p.mu.Unlock()
		}()
		for {
			select {
			case event := <-sub:
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (p *Provider) emit(event *virt.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, subscriber := range p.subscribers {
		select {
		case subscriber <- *event:
		default:
		}
	}
}

func (p *Provider) rememberManaged(id string) {
	p.mu.Lock()
	p.managed[id] = struct{}{}
	p.mu.Unlock()
}

func (p *Provider) knowsManaged(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.managed[id]
	return ok
}

func (p *Provider) forgetManaged(id string) {
	p.mu.Lock()
	delete(p.managed, id)
	p.mu.Unlock()
}
