package libvirt

import (
	"time"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
)

func (p *Provider) handleLifecycleEvent(
	_ *libvirtgo.Connect,
	domainRef *libvirtgo.Domain,
	event *libvirtgo.DomainEventLifecycle,
) {
	id, err := domainRef.GetUUIDString()
	if err != nil {
		return
	}
	managed, metadataErr := isManaged(domainRef)
	if (!managed || metadataErr != nil) && !p.knowsManaged(id) {
		return
	}
	name, err := domainRef.GetName()
	if err != nil {
		return
	}
	eventType, state, ok := lifecycleEvent(event.Event)
	if !ok {
		return
	}
	p.emit(&virt.Event{
		Type: eventType, Time: time.Now(), NodeID: id, Name: name,
		State: state, Detail: event.String(),
	})
	if event.Event == libvirtgo.DOMAIN_EVENT_UNDEFINED {
		p.forgetManaged(id)
	}
}

func lifecycleEvent(event libvirtgo.DomainEventType) (virt.EventType, virt.NodeState, bool) {
	switch event {
	case libvirtgo.DOMAIN_EVENT_DEFINED:
		return virt.EventDefined, virt.NodeStopped, true
	case libvirtgo.DOMAIN_EVENT_UNDEFINED:
		return virt.EventUndefined, virt.NodeStopped, true
	case libvirtgo.DOMAIN_EVENT_STARTED:
		return virt.EventStarted, virt.NodeRunning, true
	case libvirtgo.DOMAIN_EVENT_STOPPED, libvirtgo.DOMAIN_EVENT_SHUTDOWN:
		return virt.EventStopped, virt.NodeStopped, true
	case libvirtgo.DOMAIN_EVENT_SUSPENDED, libvirtgo.DOMAIN_EVENT_PMSUSPENDED:
		return virt.EventSuspended, virt.NodeSuspended, true
	case libvirtgo.DOMAIN_EVENT_RESUMED:
		return virt.EventResumed, virt.NodeRunning, true
	case libvirtgo.DOMAIN_EVENT_CRASHED:
		return virt.EventCrashed, virt.NodeStopped, true
	default:
		return "", "", false
	}
}
