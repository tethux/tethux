package libvirt

import (
	"testing"

	libvirtgo "libvirt.org/go/libvirt"

	"github.com/tethux/tethux/virt"
)

func TestLifecycleEventNormalization(t *testing.T) {
	tests := []struct {
		input     libvirtgo.DomainEventType
		eventType virt.EventType
		state     virt.NodeState
	}{
		{input: libvirtgo.DOMAIN_EVENT_DEFINED, eventType: virt.EventDefined, state: virt.NodeStopped},
		{input: libvirtgo.DOMAIN_EVENT_STARTED, eventType: virt.EventStarted, state: virt.NodeRunning},
		{input: libvirtgo.DOMAIN_EVENT_SUSPENDED, eventType: virt.EventSuspended, state: virt.NodeSuspended},
		{input: libvirtgo.DOMAIN_EVENT_RESUMED, eventType: virt.EventResumed, state: virt.NodeRunning},
		{input: libvirtgo.DOMAIN_EVENT_SHUTDOWN, eventType: virt.EventStopped, state: virt.NodeStopped},
		{input: libvirtgo.DOMAIN_EVENT_CRASHED, eventType: virt.EventCrashed, state: virt.NodeStopped},
		{input: libvirtgo.DOMAIN_EVENT_UNDEFINED, eventType: virt.EventUndefined, state: virt.NodeStopped},
	}
	for _, test := range tests {
		eventType, state, ok := lifecycleEvent(test.input)
		if !ok || eventType != test.eventType || state != test.state {
			t.Errorf("lifecycleEvent(%v) = %q, %q, %v", test.input, eventType, state, ok)
		}
	}
	if _, _, ok := lifecycleEvent(libvirtgo.DomainEventType(999)); ok {
		t.Fatal("unknown lifecycle event was accepted")
	}
}
