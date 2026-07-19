package bridge

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

type middlewarePort struct {
	base      Port
	readHook  func() error
	writeHook func() error
}

// WrapPort applies middleware in declaration order. The first middleware is
// the outermost wrapper, so it observes a frame before later middleware.
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

// Latency returns middleware that delays each ingress and egress frame.
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

// WithLatency wraps port with latency middleware.
func WithLatency(port Port, delay time.Duration) Port {
	return WrapPort(port, Latency(delay))
}

// PacketLossOptions configures a middleware which independently drops ingress
// and egress frames. Probability must be in the inclusive range [0, 1].
//
// Random is primarily useful for deterministic tests. It must return a value
// in [0, 1); a nil function uses math/rand's package-level source.
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

// NewPacketLossMiddleware constructs packet-loss middleware. A dropped frame
// is intentionally reported as a successful read or write: loss is part of
// the simulated link, not a failure of the underlying port.
func NewPacketLossMiddleware(opts PacketLossOptions) (PortMiddleware, error) {
	if opts.Probability < 0 || opts.Probability > 1 {
		return nil, fmt.Errorf("packet loss probability must be between 0 and 1, got %g", opts.Probability)
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

// WithPacketLoss wraps port with packet-loss middleware.
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
