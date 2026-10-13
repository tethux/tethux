package shitnet

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"sort"
	"sync"

	native "github.com/tethux/shitnet"
	nativeerrs "github.com/tethux/shitnet/errs"
	"github.com/tethux/tethux/bridge"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/shitnet/errs"
)

// Adapter owns endpoints. Calls must be externally serialized.
// Packet handling and probes serialize native calls within each endpoint.
type Adapter struct{ nodes map[string]*endpoint }

type endpoint struct {
	mu      sync.Mutex
	cfg     Config
	stack   *native.Shitnet
	state   virt.NodeState
	port    bridge.Port
	cancel  context.CancelFunc
	done    chan struct{}
	failure error
	next    uint32
	probes  map[uint32]*probe
}

type probe struct {
	target       netip.Addr
	id, sequence uint16
	sent         bool
	done         chan error
}

var probePayload = []byte("tethux")

// Info reports endpoint capabilities.
func (a *Adapter) Info() virt.ProviderInfo {
	return virt.ProviderInfo{Name: "shitnet", DisplayName: "Shitnet", Kind: virt.ProviderKindEmulator}
}

// CreateEndpoint allocates a stopped native stack.
func (a *Adapter) CreateEndpoint(ctx context.Context, cfg *Config) (*virt.Node, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if a == nil || cfg == nil || cfg.ID == "" || !cfg.IP.Is4() || cfg.IP.IsUnspecified() ||
		cfg.MAC == (native.MAC{}) || cfg.MAC[0]&1 != 0 || !cfg.Local.IsValid() || !cfg.Remote.IsValid() ||
		!cfg.Local.Addr().Is4() || !cfg.Remote.Addr().Is4() || cfg.Local.Port() == 0 || cfg.Remote.Port() == 0 {
		return nil, errs.New("create endpoint", errs.ErrConfig, "ID, unicast MAC, IPv4 address, and UDP endpoints are required")
	}
	if _, exists := a.nodes[cfg.ID]; exists {
		return nil, errs.New("create endpoint", errs.ErrDuplicate, cfg.ID)
	}
	stack, createErr := native.New(native.Config{IP: cfg.IP, MAC: cfg.MAC})
	if createErr != nil {
		return nil, errs.Wrap("create endpoint", errs.ErrConfig, cfg.ID, createErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		stack.Close()
		return nil, contextErr
	}
	if a.nodes == nil {
		a.nodes = make(map[string]*endpoint)
	}
	a.nodes[cfg.ID] = &endpoint{cfg: *cfg, stack: stack, state: virt.NodeStopped, probes: make(map[uint32]*probe)}
	return &virt.Node{ID: cfg.ID, Name: cfg.Name, State: virt.NodeStopped}, nil
}

func (a *Adapter) lookup(ctx context.Context, id string) (*endpoint, error) {
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if a == nil {
		return nil, errs.New("find endpoint", errs.ErrNotFound, id)
	}
	node, exists := a.nodes[id]
	if !exists {
		return nil, errs.New("find endpoint", errs.ErrNotFound, id)
	}
	return node, nil
}

// Start binds the UDP transport and begins packet processing.
func (a *Adapter) Start(ctx context.Context, id string) error {
	node, lookupErr := a.lookup(ctx, id)
	if lookupErr != nil {
		return lookupErr
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	if node.state == virt.NodeRunning {
		return nil
	}
	if node.done != nil {
		select {
		case <-node.done:
		default:
			return errs.New("start endpoint", errs.ErrState, id)
		}
	}
	port, portErr := bridge.NewUDPPort(&bridge.PortOptions{ID: id, LocalAddr: node.cfg.Local.String(), Remote: node.cfg.Remote.String(), MTU: 1500})
	if portErr != nil {
		return errs.Wrap("start endpoint", errs.ErrNetwork, id, portErr)
	}
	runCtx, cancel := context.WithCancel(ctx)
	node.port, node.cancel, node.done, node.state, node.failure = port, cancel, make(chan struct{}), virt.NodeRunning, nil
	go node.run(runCtx, port, node.done)
	return nil
}

func (node *endpoint) run(ctx context.Context, port bridge.Port, done chan struct{}) {
	var failure error
	defer func() {
		closeErr := port.Close()
		if !errors.Is(closeErr, net.ErrClosed) {
			failure = errors.Join(failure, closeErr)
		}
		node.mu.Lock()
		if ctx.Err() != nil {
			failure = nil
		}
		node.state, node.failure = virt.NodeStopped, failure
		for key, pending := range node.probes {
			pending.done <- errs.Wrap("stop probe", errs.ErrState, node.cfg.ID, failure)
			delete(node.probes, key)
		}
		node.mu.Unlock()
		close(done)
	}()
	for ctx.Err() == nil {
		frame, readErr := port.ReadFrame()
		if readErr != nil {
			var timeout net.Error
			if errors.As(readErr, &timeout) && timeout.Timeout() {
				continue
			}
			failure = readErr
			return
		}
		if len(frame) < 14 {
			continue
		}
		node.mu.Lock()
		receiveErr := node.stack.Receive(frame)
		// Malformed Ethernet input is discarded at the native packet boundary.
		if errors.Is(receiveErr, nativeerrs.ErrInvalidPacket) {
			receiveErr = nil
		}
		frames, drainErr := node.drain()
		node.mu.Unlock()
		if receiveErr != nil || drainErr != nil {
			failure = errors.Join(receiveErr, drainErr)
			return
		}
		if writeErr := writeFrames(port, frames); writeErr != nil {
			failure = writeErr
			return
		}
	}
}

func (node *endpoint) drain() ([][]byte, error) {
	for {
		poll, pollErr := node.stack.PollEvent()
		if pollErr != nil {
			return nil, pollErr
		}
		if !poll.Available {
			break
		}
		for key, pending := range node.probes {
			if poll.Event.Type == native.EventARPLearned && poll.Event.ARP != nil && poll.Event.ARP.IP == pending.target && !pending.sent {
				queued, queueErr := node.stack.ICMPEchoRequest(pending.target, pending.id, pending.sequence, probePayload)
				if queueErr != nil {
					return nil, queueErr
				}
				pending.sent = queued == native.QueueQueued
			}
			icmp := poll.Event.ICMP
			if poll.Event.Type == native.EventICMPEchoReply && icmp != nil && icmp.Source == pending.target && icmp.Identifier == pending.id && icmp.Sequence == pending.sequence && bytes.Equal(icmp.Payload, probePayload) {
				pending.done <- nil
				delete(node.probes, key)
			}
		}
	}
	var frames [][]byte
	for {
		poll, pollErr := node.stack.PollTX()
		if pollErr != nil {
			return nil, pollErr
		}
		if !poll.Available {
			return frames, nil
		}
		frames = append(frames, poll.Frame)
	}
}

func writeFrames(port bridge.Port, frames [][]byte) error {
	for _, frame := range frames {
		if writeErr := port.WriteFrame(frame); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// Ping performs sequential native ARP resolution and ICMP echo exchanges.
func (a *Adapter) Ping(ctx context.Context, id string, target netip.Addr, count uint16) (PingResult, error) {
	var result PingResult
	if !target.Is4() || count == 0 {
		return result, errs.New("ping endpoint", errs.ErrConfig, id)
	}
	node, lookupErr := a.lookup(ctx, id)
	if lookupErr != nil {
		return result, lookupErr
	}
	for index := uint16(0); index < count; index++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return result, errs.Wrap("ping endpoint", errs.ErrProbe, id, contextErr)
		}
		sent, probeErr := node.ping(ctx, target)
		if sent {
			result.Sent++
		}
		if probeErr != nil {
			return result, errs.Wrap("ping endpoint", errs.ErrProbe, id, probeErr)
		}
		result.Received++
	}
	return result, nil
}

func (node *endpoint) ping(ctx context.Context, target netip.Addr) (sent bool, resultErr error) {
	node.mu.Lock()
	if node.state != virt.NodeRunning {
		node.mu.Unlock()
		return false, errs.New("send echo", errs.ErrState, node.cfg.ID)
	}
	node.next++
	key := node.next
	pending := &probe{target: target, id: uint16(key >> 16), sequence: uint16(key & 0xffff), done: make(chan error, 1)}
	node.probes[key] = pending
	queued, queueErr := node.stack.ICMPEchoRequest(target, pending.id, pending.sequence, probePayload)
	pending.sent = queued == native.QueueQueued
	if queueErr == nil && !pending.sent {
		queueErr = node.stack.ARPRequest(target)
	}
	frames, drainErr := node.drain()
	port := node.port
	node.mu.Unlock()
	defer func() { node.mu.Lock(); sent = pending.sent; delete(node.probes, key); node.mu.Unlock() }()
	if queueErr != nil || drainErr != nil {
		return false, errors.Join(queueErr, drainErr)
	}
	if writeErr := writeFrames(port, frames); writeErr != nil {
		return false, writeErr
	}
	select {
	case probeErr := <-pending.done:
		return false, probeErr
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Stop closes the transport and joins packet processing, retaining the native stack.
func (a *Adapter) Stop(ctx context.Context, id string) error {
	node, lookupErr := a.lookup(ctx, id)
	if lookupErr != nil {
		return lookupErr
	}
	node.mu.Lock()
	cancel, port, done := node.cancel, node.port, node.done
	node.mu.Unlock()
	if done == nil {
		return nil
	}
	cancel()
	closeErr := port.Close()
	if errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}
	select {
	case <-done:
		return closeErr
	case <-ctx.Done():
		return errors.Join(closeErr, ctx.Err())
	}
}

// Restart rebinds the transport with the current addresses and ARP table.
func (a *Adapter) Restart(ctx context.Context, id string) error {
	if stopErr := a.Stop(ctx, id); stopErr != nil {
		return stopErr
	}
	return a.Start(ctx, id)
}

// Suspend reports that pausing native endpoints is unsupported.
func (a *Adapter) Suspend(ctx context.Context, id string) error {
	if _, lookupErr := a.lookup(ctx, id); lookupErr != nil {
		return lookupErr
	}
	return errs.New("suspend endpoint", errs.ErrUnsupported, id)
}

// Resume reports that pausing native endpoints is unsupported.
func (a *Adapter) Resume(ctx context.Context, id string) error {
	if _, lookupErr := a.lookup(ctx, id); lookupErr != nil {
		return lookupErr
	}
	return errs.New("resume endpoint", errs.ErrUnsupported, id)
}

// State returns packet processing state and any background failure.
func (a *Adapter) State(ctx context.Context, id string) (virt.NodeState, error) {
	node, lookupErr := a.lookup(ctx, id)
	if lookupErr != nil {
		return "", lookupErr
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	if node.failure != nil {
		return node.state, errs.Wrap("read endpoint state", errs.ErrNetwork, id, node.failure)
	}
	return node.state, nil
}

// Reload returns an endpoint node snapshot.
func (a *Adapter) Reload(ctx context.Context, id string) (*virt.Node, error) {
	state, stateErr := a.State(ctx, id)
	if stateErr != nil {
		return nil, stateErr
	}
	return &virt.Node{ID: id, Name: a.nodes[id].cfg.Name, State: state}, nil
}

// List returns endpoint snapshots sorted by ID.
func (a *Adapter) List(ctx context.Context) ([]*virt.Node, error) {
	if a == nil {
		return nil, errs.New("list endpoints", errs.ErrConfig, "nil adapter")
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	ids := make([]string, 0, len(a.nodes))
	for id := range a.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	nodes := make([]*virt.Node, 0, len(ids))
	for _, id := range ids {
		node, reloadErr := a.Reload(ctx, id)
		if reloadErr != nil {
			return nil, reloadErr
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// Delete stops packet processing and releases the native stack.
func (a *Adapter) Delete(ctx context.Context, id string) error {
	if a == nil {
		return nil
	}
	if _, exists := a.nodes[id]; !exists {
		return ctx.Err()
	}
	if stopErr := a.Stop(ctx, id); stopErr != nil {
		return stopErr
	}
	node := a.nodes[id]
	node.mu.Lock()
	node.stack.Close()
	node.mu.Unlock()
	delete(a.nodes, id)
	return nil
}

// Close deletes owned endpoints and joins their workers.
func (a *Adapter) Close() error {
	if a == nil {
		return nil
	}
	var failures []error
	for id := range a.nodes {
		if deleteErr := a.Delete(context.Background(), id); deleteErr != nil {
			failures = append(failures, deleteErr)
		}
	}
	return errors.Join(failures...)
}
