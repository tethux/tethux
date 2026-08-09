package bridge

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

type middlewarePort struct {
	base      Port
	readHook  func() error
	writeHook func() error
}

// applies middleware in declaration order; the first is outermost.
func WrapPort(port Port, middleware ...PortMiddleware) Port {
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			port = middleware[i](port)
		}
	}
	return port
}

func (m *middlewarePort) ID() string {
	return m.base.ID()
}

func (m *middlewarePort) MTU() int {
	return m.base.MTU()
}

func (m *middlewarePort) ReadFrame() (Frame, error) {
	if m.readHook != nil {
		if err := m.readHook(); err != nil {
			return nil, err
		}
	}
	return m.base.ReadFrame()
}

func (m *middlewarePort) WriteFrame(frame Frame) error {
	if m.writeHook != nil {
		if err := m.writeHook(); err != nil {
			return err
		}
	}
	return m.base.WriteFrame(frame)
}

func (m *middlewarePort) Close() error {
	return m.base.Close()
}

// delays each ingress and egress frame.
func Latency(delay time.Duration) PortMiddleware {
	if delay <= 0 {
		return func(port Port) Port { return port }
	}

	return func(port Port) Port {
		return &middlewarePort{
			base: port,
			readHook: func() error {
				time.Sleep(delay)
				return nil
			},
			writeHook: func() error {
				time.Sleep(delay)
				return nil
			},
		}
	}
}

// wraps a port with latency.
func WithLatency(port Port, delay time.Duration) Port {
	return WrapPort(port, Latency(delay))
}

// configures independent ingress and egress loss from 0 to 1.
// random is optional and mainly useful for tests.
type PacketLossOptions struct {
	Probability float64
	Random      func() float64
}

type packetLossPort struct {
	base        Port
	probability float64
	random      func() float64
	mu          sync.Mutex
}

// builds packet loss middleware; dropped frames are successful operations.
func NewPacketLossMiddleware(opts PacketLossOptions) (PortMiddleware, error) {
	if opts.Probability < 0 || opts.Probability > 1 {
		return nil, errs.New("packet loss", errs.ErrInvalidOptions, "probability must be between 0 and 1")
	}
	if opts.Random == nil {
		opts.Random = rand.Float64
	}

	return func(port Port) Port {
		if opts.Probability == 0 {
			return port
		}
		return &packetLossPort{
			base:        port,
			probability: opts.Probability,
			random:      opts.Random,
		}
	}, nil
}

// wraps a port with packet loss.
func WithPacketLoss(port Port, opts PacketLossOptions) (Port, error) {
	middleware, err := NewPacketLossMiddleware(opts)
	if err != nil {
		return nil, err
	}
	return WrapPort(port, middleware), nil
}

func (p *packetLossPort) ID() string { return p.base.ID() }

func (p *packetLossPort) MTU() int { return p.base.MTU() }

func (p *packetLossPort) ReadFrame() (Frame, error) {
	for {
		frame, err := p.base.ReadFrame()
		if err != nil || !p.drop() {
			return frame, err
		}
	}
}

func (p *packetLossPort) WriteFrame(frame Frame) error {
	if p.drop() {
		return nil
	}
	return p.base.WriteFrame(frame)
}

func (p *packetLossPort) Close() error { return p.base.Close() }

func (p *packetLossPort) drop() bool {
	if p.probability == 1 {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.random() < p.probability
}
