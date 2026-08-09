package bridge

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/tethux/tethux/internal/assert"
	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

const ethernetHeaderLen = 14

// Frame contains one Ethernet frame.
type Frame = []byte

// Port is a bidirectional Ethernet frame transport attached to a Switch.
type Port interface {
	ID() string
	MTU() int
	ReadFrame() (Frame, error)
	WriteFrame(Frame) error
	Close() error
}

// PortMiddleware wraps a Port with additional behavior.
type PortMiddleware func(Port) Port

// CaptureSink receives copies of frames observed by a Switch.
type CaptureSink interface {
	Capture(CapturedFrame)
}

// CapturedFrame describes a frame observed on a switch port.
type CapturedFrame struct {
	PortID    string
	Direction FrameDirection
	Frame     Frame
	Time      time.Time
}

// FrameDirection identifies whether a captured frame entered or left the switch.
type FrameDirection string

const (
	// FrameIngress identifies a frame received from a port.
	FrameIngress FrameDirection = "ingress"
	// FrameEgress identifies a frame sent to a port.
	FrameEgress FrameDirection = "egress"
)

// EventType identifies a switch lifecycle or forwarding event.
type EventType string

const (
	// EventPortAttached reports that a port was attached.
	EventPortAttached EventType = "port_attached"
	// EventPortRemoved reports that a port was detached.
	EventPortRemoved EventType = "port_removed"
	// EventPortClosed reports that a detached port was closed.
	EventPortClosed EventType = "port_closed"
	// EventFrameIngress reports a frame received by the switch.
	EventFrameIngress EventType = "frame_ingress"
	// EventFrameEgress reports a frame forwarded by the switch.
	EventFrameEgress EventType = "frame_egress"
	// EventFrameDropped reports a frame that could not be processed or forwarded.
	EventFrameDropped EventType = "frame_dropped"
	// EventFDBLearned reports a forwarding-database update.
	EventFDBLearned EventType = "fdb_learned"
	// EventSwitchStart reports that the switch started.
	EventSwitchStart EventType = "switch_start"
	// EventSwitchStop reports that the switch stopped.
	EventSwitchStop EventType = "switch_stop"
)

// Event describes a switch lifecycle or forwarding operation.
type Event struct {
	Type       EventType
	PortID     string
	TargetPort string
	Frame      Frame
	MAC        net.HardwareAddr
	Time       time.Time
	Err        error
}

// EventHandler receives an immutable copy of a switch event.
type EventHandler func(Event)

// SwitchOptions configures switch forwarding behavior.
type SwitchOptions struct {
	DisableUnknownUnicastFlood bool
}

// Switch is a concurrent Ethernet learning switch.
type Switch struct {
	mu       sync.RWMutex
	ports    map[string]Port
	fdb      map[string]string
	captures []CaptureSink
	handlers map[uint64]EventHandler
	order    []string

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
	nextHook uint64
	opts     SwitchOptions
}

// NewSwitch creates a stopped switch with no attached ports.
func NewSwitch(opts SwitchOptions) *Switch {
	return &Switch{
		ports:    make(map[string]Port),
		fdb:      make(map[string]string),
		handlers: make(map[uint64]EventHandler),
		order:    make([]string, 0),
		opts:     opts,
	}
}

// AttachPort attaches a port and starts its reader when the switch is running.
func (s *Switch) AttachPort(p Port) error {
	assert.That(s != nil, "AttachPort called on nil switch")
	assert.That(p != nil, "AttachPort called with nil port")
	assert.That(p.ID() != "", "AttachPort called with empty port ID")
	assert.That(p.MTU() > 0, "port %q has invalid MTU %d", p.ID(), p.MTU())

	s.mu.Lock()

	if assert.Enabled {
		s.assertValidLocked()
	}

	id := p.ID()

	if _, exists := s.ports[id]; exists {
		s.mu.Unlock()
		return errs.ErrPortAlrAttached
	}

	s.ports[id] = p
	s.order = append(s.order, id)

	if assert.Enabled {
		s.assertValidLocked()
	}

	if s.started {
		assert.That(s.ctx != nil, "started switch has nil context")
		assert.That(s.cancel != nil, "started switch has nil cancel")

		s.startReaderLocked(p)
	}

	if assert.Enabled {
		s.assertValidLocked()
	}

	s.mu.Unlock()

	s.emit(&Event{
		Type:   EventPortAttached,
		PortID: id,
		Time:   time.Now(),
	})

	return nil
}

// RemovePort detaches and closes the port identified by id.
func (s *Switch) RemovePort(id string) error {
	assert.That(s != nil, "RemovePort called on nil switch")
	assert.That(id != "", "RemovePort called with empty id")

	s.mu.Lock()
	if assert.Enabled {
		s.assertValidLocked()
	}

	p, ok := s.ports[id]
	if !ok {
		s.mu.Unlock()
		return errs.ErrPortNotFound
	}

	assert.That(p != nil, "stored port %q is nil", id)
	assert.That(p.ID() == id,
		"stored port key %q != Port.ID() %q", id, p.ID())

	delete(s.ports, id)
	deleteFDBEntriesForPort(s.fdb, id)
	s.order = removePortOrder(s.order, id)

	if assert.Enabled {
		s.assertValidLocked()
	}
	s.mu.Unlock()

	s.emit(&Event{
		Type:   EventPortRemoved,
		PortID: id,
		Time:   time.Now(),
	})

	if err := p.Close(); err != nil {
		return err
	}

	s.emit(&Event{
		Type:   EventPortClosed,
		PortID: id,
		Time:   time.Now(),
	})

	return nil
}

// Start starts readers for all attached ports.
func (s *Switch) Start() error {
	assert.That(s != nil, "Start called on nil switch")

	s.mu.Lock()
	if assert.Enabled {
		s.assertValidLocked()
	}

	if s.started {
		assert.That(s.ctx != nil, "started switch has nil ctx")
		assert.That(s.cancel != nil, "started switch has nil cancel")

		s.mu.Unlock()
		return nil
	}

	assert.That(s.ctx == nil, "stopped switch has existing ctx")
	assert.That(s.cancel == nil, "stopped switch has existing cancel")

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	assert.That(s.ctx != nil, "context.WithCancel returned nil ctx")
	assert.That(s.cancel != nil, "context.WithCancel returned nil cancel")
	if assert.Enabled {
		s.assertValidLocked()
	}

	for _, id := range s.sortedPortIDsLocked() {
		p, ok := s.ports[id]
		assert.That(ok, "sorted port %q missing from ports map", id)
		assert.That(p != nil, "sorted port %q is nil", id)
		s.startReaderLocked(p)
	}

	if assert.Enabled {
		s.assertValidLocked()
	}
	s.mu.Unlock()
	s.emit(&Event{Type: EventSwitchStart, Time: time.Now()})
	return nil
}

// Stop stops all readers and closes all attached ports.
func (s *Switch) Stop() error {
	assert.That(s != nil, "Stop called on nil switch")

	s.mu.Lock()
	if assert.Enabled {
		s.assertValidLocked()
	}

	if !s.started {
		assert.That(s.ctx == nil, "stopped switch has non-nil ctx")
		assert.That(s.cancel == nil, "stopped switch has non-nil cancel")

		s.mu.Unlock()
		return nil
	}

	assert.That(s.ctx != nil, "started switch has nil ctx")
	assert.That(s.cancel != nil, "started switch has nil cancel")

	cancel := s.cancel

	ports := make([]Port, 0, len(s.ports))
	for _, id := range s.sortedPortIDsLocked() {
		p, ok := s.ports[id]

		assert.That(ok, "ordered port %q missing from ports map", id)
		assert.That(p != nil, "port %q is nil", id)

		ports = append(ports, p)
	}

	s.started = false
	s.cancel = nil
	s.ctx = nil

	if assert.Enabled {
		s.assertValidLocked()
	}
	s.mu.Unlock()

	assert.That(cancel != nil, "Stop copied nil cancel")

	cancel()

	for _, p := range ports {
		assert.That(p != nil, "Stop collected nil port")
		_ = p.Close()
	}

	s.emit(&Event{
		Type: EventSwitchStop,
		Time: time.Now(),
	})

	s.wg.Wait()
	return nil
}

// OnEvent registers a handler and returns a function that unregisters it.
func (s *Switch) OnEvent(handler EventHandler) func() {
	assert.That(s != nil, "OnEvent called on nil switch")
	assert.That(handler != nil, "OnEvent called with nil handler")

	s.mu.Lock()
	defer s.mu.Unlock()

	if assert.Enabled {
		s.assertValidLocked()
	}

	id := s.nextHook
	s.nextHook++

	assert.That(s.nextHook > id,
		"event hook counter did not advance")

	s.handlers[id] = handler

	assert.That(s.handlers[id] != nil,
		"handler %d was not stored", id)

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		delete(s.handlers, id)
		if assert.Enabled {
			s.assertValidLocked()
		}
	}
}

// AddCaptureSink registers a sink that receives copies of captured frames.
func (s *Switch) AddCaptureSink(sink CaptureSink) {
	assert.That(s != nil, "AddCaptureSink called on nil switch")
	assert.That(sink != nil, "AddCaptureSink called with nil sink")

	s.mu.Lock()
	defer s.mu.Unlock()

	if assert.Enabled {
		s.assertValidLocked()
	}

	s.captures = append(s.captures, sink)

	assert.That(len(s.captures) > 0,
		"capture sink append failed")
	if assert.Enabled {
		s.assertValidLocked()
	}
}

func (s *Switch) startReaderLocked(p Port) {
	assert.That(s != nil, "startReaderLocked on nil switch")
	assert.That(p != nil, "startReaderLocked with nil port")
	assert.That(s.started, "starting reader while switch is stopped")
	assert.That(s.ctx != nil, "starting reader with nil ctx")
	assert.That(s.cancel != nil, "starting reader with nil cancel")
	assert.That(p.ID() != "", "starting reader for empty port ID")
	assert.That(p.MTU() > 0,
		"port %q has invalid MTU %d", p.ID(), p.MTU())

	stored, ok := s.ports[p.ID()]
	assert.That(ok,
		"starting reader for unattached port %q", p.ID())
	assert.That(stored == p,
		"stored port %q differs from reader port", p.ID())

	ctx := s.ctx
	assert.That(ctx != nil, "captured nil reader context")

	s.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			frame, err := p.ReadFrame()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					if isReadTimeout(err) {
						continue
					}
					s.emit(&Event{
						Type:   EventFrameDropped,
						PortID: p.ID(),
						Time:   time.Now(),
						Err:    err,
					})
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			assert.That(frame != nil,
				"port %q returned nil frame without error", p.ID())

			s.processFrame(p.ID(), frame)
		}
	})
}

func isReadTimeout(err error) bool {
	if errors.Is(err, errs.ErrReadTimeout) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (s *Switch) processFrame(srcID string, frame Frame) {
	assert.That(s != nil, "processFrame on nil switch")
	assert.That(srcID != "", "processFrame with empty srcID")
	assert.That(frame != nil, "processFrame(%q) with nil frame", srcID)

	s.mu.RLock()
	srcPort, exists := s.ports[srcID]
	s.mu.RUnlock()

	assert.That(exists,
		"processFrame received frame from unattached port %q", srcID)
	assert.That(srcPort != nil,
		"source port %q is nil", srcID)

	now := time.Now()
	assert.That(!now.IsZero(), "time.Now returned zero time")
	frameCopy := cloneFrame(frame)
	s.capture(&CapturedFrame{PortID: srcID, Direction: FrameIngress, Frame: frameCopy, Time: now})
	s.emit(&Event{Type: EventFrameIngress, PortID: srcID, Frame: frameCopy, Time: now})

	if len(frame) < ethernetHeaderLen {
		s.emit(&Event{
			Type:   EventFrameDropped,
			PortID: srcID,
			Frame:  frameCopy,
			Time:   time.Now(),
			Err:    errs.ErrFrameTooShort,
		})
		return
	}

	srcMAC := net.HardwareAddr(frame[6:12])
	dstMAC := net.HardwareAddr(frame[0:6])

	s.learn(srcMAC, srcID)
	targets := s.egressPorts(srcID, dstMAC)
	if len(targets) == 0 {
		return
	}

	for _, target := range targets {
		assert.That(target != nil, "nil target returned by egressPorts")
		assert.That(target.ID() != "", "egress target has empty ID")
		assert.That(target.ID() != srcID,
			"egressPorts returned source port %q", srcID)

		targetFrame := cloneFrame(frame)
		assert.That(targetFrame != nil, "cloneFrame returned nil")
		assert.That(len(targetFrame) == len(frame),
			"clone changed length %d -> %d", len(frame), len(targetFrame))

		if err := target.WriteFrame(targetFrame); err != nil {
			s.emit(&Event{
				Type:       EventFrameDropped,
				PortID:     srcID,
				TargetPort: target.ID(),
				Frame:      targetFrame,
				Time:       time.Now(),
				Err:        err,
			})
			continue
		}

		copied := cloneFrame(targetFrame)
		s.capture(&CapturedFrame{PortID: target.ID(), Direction: FrameEgress, Frame: copied, Time: time.Now()})
		s.emit(&Event{
			Type:       EventFrameEgress,
			PortID:     srcID,
			TargetPort: target.ID(),
			Frame:      copied,
			Time:       time.Now(),
		})
	}
}

func (s *Switch) learn(mac net.HardwareAddr, portID string) {
	assert.That(s != nil, "learn called on nil switch")
	assert.That(portID != "", "learn called with empty port ID")

	if len(mac) == 0 || isBroadcastMAC(mac) {
		return
	}

	assert.That(len(mac) == 6,
		"learn received non-Ethernet MAC length %d", len(mac))

	s.mu.Lock()
	if assert.Enabled {
		s.assertValidLocked()
	}

	_, exists := s.ports[portID]
	assert.That(exists,
		"learning MAC %s on missing port %q", mac, portID)

	key := mac.String()
	assert.That(key != "", "MAC %v converted to empty string", mac)

	if current, ok := s.fdb[key]; ok && current == portID {
		s.mu.Unlock()
		return
	}

	s.fdb[key] = portID

	assert.That(s.fdb[key] == portID,
		"failed to store FDB %s -> %q", mac, portID)

	if assert.Enabled {
		s.assertValidLocked()
	}
	s.mu.Unlock()
	s.emit(&Event{
		Type:   EventFDBLearned,
		PortID: portID,
		MAC:    append(net.HardwareAddr(nil), mac...),
		Time:   time.Now(),
	})
}

func (s *Switch) egressPorts(srcID string, dstMAC net.HardwareAddr) []Port {
	assert.That(s != nil, "egressPorts on nil switch")
	assert.That(srcID != "", "egressPorts with empty source ID")
	assert.That(len(dstMAC) == 6,
		"egressPorts destination MAC length=%d", len(dstMAC))

	s.mu.RLock()
	defer s.mu.RUnlock()

	if isBroadcastMAC(dstMAC) || isMulticastMAC(dstMAC) {
		ports := s.allPortsExceptLocked(srcID)

		for _, p := range ports {
			assert.That(p != nil, "nil broadcast egress port")
			assert.That(p.ID() != srcID,
				"broadcast egress contains source %q", srcID)
		}

		return ports
	}

	if portID, ok := s.fdb[dstMAC.String()]; ok {
		port, exists := s.ports[portID]

		assert.That(exists,
			"FDB %s points to missing port %q",
			dstMAC, portID)

		if exists && portID != srcID {
			assert.That(port != nil,
				"FDB port %q is nil", portID)

			return []Port{port}
		}
	}

	if s.opts.DisableUnknownUnicastFlood {
		return nil
	}

	return s.allPortsExceptLocked(srcID)
}

func (s *Switch) allPortsExceptLocked(srcID string) []Port {
	assert.That(s != nil, "allPortsExceptLocked on nil switch")
	assert.That(srcID != "", "allPortsExceptLocked with empty srcID")

	ids := append([]string(nil), s.order...)
	sort.Strings(ids)

	ports := make([]Port, 0, len(ids))

	for _, id := range ids {
		assert.That(id != "", "empty ID in port order")

		port, ok := s.ports[id]

		assert.That(ok,
			"port order contains %q but ports map does not", id)
		assert.That(port != nil,
			"port order contains nil port %q", id)

		if id == srcID {
			continue
		}

		assert.That(port.ID() == id,
			"port map key %q != Port.ID() %q",
			id, port.ID())

		ports = append(ports, port)
	}

	for _, p := range ports {
		assert.That(p.ID() != srcID,
			"allPortsExceptLocked returned source %q", srcID)
	}

	return ports
}

func (s *Switch) sortedPortIDsLocked() []string {
	assert.That(s != nil, "sortedPortIDsLocked on nil switch")

	ids := append([]string(nil), s.order...)

	for _, id := range ids {
		assert.That(id != "", "empty ID in order")
		_, ok := s.ports[id]
		assert.That(ok,
			"order contains missing port %q", id)
	}

	sort.Strings(ids)

	for i := 1; i < len(ids); i++ {
		assert.That(ids[i-1] <= ids[i],
			"sorted port IDs are not sorted")
		assert.That(ids[i-1] != ids[i],
			"duplicate port ID %q", ids[i])
	}

	return ids
}

func (s *Switch) capture(frame *CapturedFrame) {
	assert.That(s != nil, "capture on nil switch")
	assert.That(frame != nil, "capture called with nil frame")
	assert.That(frame.PortID != "", "capture has empty port ID")
	assert.That(!frame.Time.IsZero(), "capture has zero timestamp")
	assert.That(
		frame.Direction == FrameIngress ||
			frame.Direction == FrameEgress,
		"invalid capture direction %q",
		frame.Direction,
	)

	s.mu.RLock()
	sinks := append([]CaptureSink(nil), s.captures...)
	s.mu.RUnlock()

	for i, sink := range sinks {
		assert.That(sink != nil,
			"capture sink %d is nil", i)

		sink.Capture(CapturedFrame{
			PortID:    frame.PortID,
			Direction: frame.Direction,
			Frame:     cloneFrame(frame.Frame),
			Time:      frame.Time,
		})
	}
}

func (s *Switch) emit(event *Event) {
	assert.That(s != nil, "emit on nil switch")
	assert.That(event != nil, "emit called with nil event")
	assert.That(event.Type != "", "event has empty type")
	assert.That(!event.Time.IsZero(), "event %q has zero timestamp", event.Type)

	switch event.Type {
	case EventPortAttached,
		EventPortRemoved,
		EventPortClosed,
		EventFrameIngress,
		EventFrameDropped,
		EventFDBLearned:
		assert.That(event.PortID != "", "event %q has empty PortID", event.Type)
	case EventFrameEgress:
		assert.That(event.PortID != "", "egress event has empty source")
		assert.That(event.TargetPort != "", "egress event has empty target")
		assert.That(event.PortID != event.TargetPort,
			"egress source and target are both %q", event.PortID)
	case EventSwitchStart, EventSwitchStop:
	default:
		assert.That(false, "unknown event type %q", event.Type)
	}

	if event.Type == EventFDBLearned {
		assert.That(len(event.MAC) == 6,
			"FDB event MAC has length %d", len(event.MAC))
	}
	if event.Type == EventFrameDropped {
		assert.That(event.Err != nil, "frame dropped event has nil error")
	}

	s.mu.RLock()
	handlers := make([]EventHandler, 0, len(s.handlers))
	for id, handler := range s.handlers {
		assert.That(handler != nil, "registered handler %d is nil", id)
		handlers = append(handlers, handler)
	}
	s.mu.RUnlock()

	for _, handler := range handlers {
		handler(cloneEvent(event))
	}
}

func cloneEvent(event *Event) Event {
	assert.That(event != nil, "cloneEvent called with nil event")

	originalFrameLen := len(event.Frame)
	originalMACLen := len(event.MAC)
	event.Frame = cloneFrame(event.Frame)
	assert.That(len(event.Frame) == originalFrameLen,
		"event frame clone changed length")

	if len(event.MAC) > 0 {
		event.MAC = append(net.HardwareAddr(nil), event.MAC...)
		assert.That(len(event.MAC) == originalMACLen,
			"event MAC clone changed length")
	}
	return *event
}

func cloneFrame(frame Frame) Frame {
	return bytes.Clone(frame)
}

func removePortOrder(order []string, target string) []string {
	assert.That(target != "", "removePortOrder with empty target")

	result := order[:0]
	for _, id := range order {
		assert.That(id != "", "order contains empty ID")
		if id != target {
			result = append(result, id)
		}
	}
	for _, id := range result {
		assert.That(id != target, "removed port %q remains in order", target)
	}
	return result
}

func deleteFDBEntriesForPort(fdb map[string]string, portID string) {
	assert.That(fdb != nil, "deleteFDBEntriesForPort with nil FDB")
	assert.That(portID != "", "deleteFDBEntriesForPort with empty port")

	for mac, current := range fdb {
		assert.That(mac != "", "FDB contains empty MAC")
		assert.That(current != "", "FDB %q contains empty port ID", mac)
		if current == portID {
			delete(fdb, mac)
		}
	}
	for mac, current := range fdb {
		assert.That(current != portID,
			"FDB %q still references removed port %q", mac, portID)
	}
}

func isBroadcastMAC(mac net.HardwareAddr) bool {
	return len(mac) == 6 && bytes.Equal(mac, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
}

func isMulticastMAC(mac net.HardwareAddr) bool {
	return len(mac) > 0 && mac[0]&0x01 == 0x01
}

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func (s *Switch) assertValidLocked() {
	if !assert.Enabled {
		return
	}

	assert.That(s.ports != nil, "ports map is nil")
	assert.That(s.fdb != nil, "fdb map is nil")
	assert.That(s.handlers != nil, "handlers map is nil")

	if s.started {
		assert.That(s.ctx != nil, "started switch has nil ctx")
		assert.That(s.cancel != nil, "started switch has nil cancel")
	} else {
		assert.That(s.ctx == nil, "stopped switch has non-nil ctx")
		assert.That(s.cancel == nil, "stopped switch has non-nil cancel")
	}

	seen := make(map[string]bool)

	for _, id := range s.order {
		assert.That(id != "", "empty port id in order")
		assert.That(!seen[id], "duplicate port %q in order", id)
		seen[id] = true

		p, ok := s.ports[id]
		assert.That(ok, "order references missing port %q", id)
		assert.That(p != nil, "port %q is nil", id)
		assert.That(p.ID() == id,
			"port key %q != Port.ID() %q", id, p.ID())
	}

	assert.That(
		len(s.order) == len(s.ports),
		"order/ports mismatch: order=%d ports=%d",
		len(s.order), len(s.ports),
	)

	for id, p := range s.ports {
		assert.That(id != "", "ports map contains empty ID")
		assert.That(p != nil, "ports[%q] is nil", id)
		assert.That(p.ID() == id,
			"ports[%q] reports ID %q", id, p.ID())
		assert.That(seen[id], "port %q exists in map but not in order", id)
		assert.That(p.MTU() > 0,
			"port %q has invalid MTU %d", id, p.MTU())
	}

	for mac, portID := range s.fdb {
		assert.That(mac != "", "empty MAC in FDB")
		assert.That(portID != "", "FDB %q contains empty port ID", mac)
		_, ok := s.ports[portID]
		assert.That(ok,
			"FDB %q points to nonexistent port %q",
			mac, portID)
	}

	for id, handler := range s.handlers {
		assert.That(handler != nil, "handler %d is nil", id)
	}
	for i, sink := range s.captures {
		assert.That(sink != nil, "capture sink %d is nil", i)
	}
}
