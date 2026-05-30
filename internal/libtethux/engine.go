package libtethux

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/0xveya/tethux/internal/libtethux/errs"
)

const ethernetHeaderLen = 14

type Frame = []byte

type Port interface {
	ID() string
	MTU() int
	ReadFrame() (Frame, error)
	WriteFrame(Frame) error
	Close() error
}

type PortMiddleware func(Port) Port

type CaptureSink interface {
	Capture(CapturedFrame)
}

type CapturedFrame struct {
	PortID    string
	Direction FrameDirection
	Frame     Frame
	Time      time.Time
}

type FrameDirection string

const (
	FrameIngress FrameDirection = "ingress"
	FrameEgress  FrameDirection = "egress"
)

type EventType string

const (
	EventPortAttached EventType = "port_attached"
	EventPortRemoved  EventType = "port_removed"
	EventPortClosed   EventType = "port_closed"
	EventFrameIngress EventType = "frame_ingress"
	EventFrameEgress  EventType = "frame_egress"
	EventFrameDropped EventType = "frame_dropped"
	EventFDBLearned   EventType = "fdb_learned"
	EventSwitchStart  EventType = "switch_start"
	EventSwitchStop   EventType = "switch_stop"
)

type Event struct {
	Type       EventType
	PortID     string
	TargetPort string
	Frame      Frame
	MAC        net.HardwareAddr
	Time       time.Time
	Err        error
}

type EventHandler func(Event)

type SwitchOptions struct {
	DisableUnknownUnicastFlood bool
}

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

func NewSwitch(opts SwitchOptions) *Switch {
	return &Switch{
		ports:    make(map[string]Port),
		fdb:      make(map[string]string),
		handlers: make(map[uint64]EventHandler),
		order:    make([]string, 0),
		opts:     opts,
	}
}

func (s *Switch) AttachPort(p Port) error {
	s.mu.Lock()
	if _, exists := s.ports[p.ID()]; exists {
		s.mu.Unlock()
		return errs.ErrPortAlrAttached
	}

	s.ports[p.ID()] = p
	s.order = append(s.order, p.ID())

	if s.started {
		s.startReaderLocked(p)
	}

	s.mu.Unlock()
	s.emit(&Event{Type: EventPortAttached, PortID: p.ID(), Time: time.Now()})

	return nil
}

func (s *Switch) RemovePort(id string) error {
	s.mu.Lock()
	p, ok := s.ports[id]
	if !ok {
		s.mu.Unlock()
		return errs.ErrPortNotFound
	}

	delete(s.ports, id)
	deleteFDBEntriesForPort(s.fdb, id)
	s.order = removePortOrder(s.order, id)
	s.mu.Unlock()

	s.emit(&Event{Type: EventPortRemoved, PortID: id, Time: time.Now()})

	if err := p.Close(); err != nil {
		return err
	}

	s.emit(&Event{Type: EventPortClosed, PortID: id, Time: time.Now()})
	return nil
}

func (s *Switch) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	for _, id := range s.sortedPortIDsLocked() {
		s.startReaderLocked(s.ports[id])
	}

	s.mu.Unlock()
	s.emit(&Event{Type: EventSwitchStart, Time: time.Now()})
	return nil
}

func (s *Switch) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}

	cancel := s.cancel
	ports := make([]Port, 0, len(s.ports))
	for _, id := range s.sortedPortIDsLocked() {
		ports = append(ports, s.ports[id])
	}
	s.started = false
	s.cancel = nil
	s.ctx = nil
	s.mu.Unlock()

	cancel()
	for _, p := range ports {
		_ = p.Close()
	}
	s.emit(&Event{Type: EventSwitchStop, Time: time.Now()})
	s.wg.Wait()
	return nil
}

func (s *Switch) OnEvent(handler EventHandler) func() {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextHook
	s.nextHook++
	s.handlers[id] = handler

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.handlers, id)
	}
}

func (s *Switch) AddCaptureSink(sink CaptureSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captures = append(s.captures, sink)
}

func (s *Switch) startReaderLocked(p Port) {
	ctx := s.ctx

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

			s.processFrame(p.ID(), frame)
		}
	})
}

func isReadTimeout(err error) bool {
	if errors.Is(err, errReadTimeout) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (s *Switch) processFrame(srcID string, frame Frame) {
	now := time.Now()
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
		targetFrame := cloneFrame(frame)
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
	if len(mac) == 0 || isBroadcastMAC(mac) {
		return
	}

	s.mu.Lock()
	key := mac.String()
	if current, ok := s.fdb[key]; ok && current == portID {
		s.mu.Unlock()
		return
	}

	s.fdb[key] = portID
	s.mu.Unlock()
	s.emit(&Event{
		Type:   EventFDBLearned,
		PortID: portID,
		MAC:    append(net.HardwareAddr(nil), mac...),
		Time:   time.Now(),
	})
}

func (s *Switch) egressPorts(srcID string, dstMAC net.HardwareAddr) []Port {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if isBroadcastMAC(dstMAC) || isMulticastMAC(dstMAC) {
		return s.allPortsExceptLocked(srcID)
	}

	if portID, ok := s.fdb[dstMAC.String()]; ok {
		if port, exists := s.ports[portID]; exists && portID != srcID {
			return []Port{port}
		}
	}

	if s.opts.DisableUnknownUnicastFlood {
		return nil
	}

	return s.allPortsExceptLocked(srcID)
}

func (s *Switch) allPortsExceptLocked(srcID string) []Port {
	ids := append([]string(nil), s.order...)
	sort.Strings(ids)

	ports := make([]Port, 0, len(ids))
	for _, id := range ids {
		if id == srcID {
			continue
		}
		if port, ok := s.ports[id]; ok {
			ports = append(ports, port)
		}
	}

	return ports
}

func (s *Switch) sortedPortIDsLocked() []string {
	ids := append([]string(nil), s.order...)
	sort.Strings(ids)
	return ids
}

func (s *Switch) capture(frame *CapturedFrame) {
	s.mu.RLock()
	sinks := append([]CaptureSink(nil), s.captures...)
	s.mu.RUnlock()

	for _, sink := range sinks {
		sink.Capture(CapturedFrame{
			PortID:    frame.PortID,
			Direction: frame.Direction,
			Frame:     cloneFrame(frame.Frame),
			Time:      frame.Time,
		})
	}
}

func (s *Switch) emit(event *Event) {
	s.mu.RLock()
	handlers := make([]EventHandler, 0, len(s.handlers))
	for _, handler := range s.handlers {
		handlers = append(handlers, handler)
	}
	s.mu.RUnlock()

	for _, handler := range handlers {
		handler(cloneEvent(event))
	}
}

func cloneEvent(event *Event) Event {
	event.Frame = cloneFrame(event.Frame)
	if len(event.MAC) > 0 {
		event.MAC = append(net.HardwareAddr(nil), event.MAC...)
	}
	return *event
}

func cloneFrame(frame Frame) Frame {
	return bytes.Clone(frame)
}

func removePortOrder(order []string, target string) []string {
	result := order[:0]
	for _, id := range order {
		if id != target {
			result = append(result, id)
		}
	}
	return result
}

func deleteFDBEntriesForPort(fdb map[string]string, portID string) {
	for mac, current := range fdb {
		if current == portID {
			delete(fdb, mac)
		}
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
