package bridge

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/tethux/tethux/bridge/errs"
	"github.com/tethux/tethux/internal/assert"
)

type middlewarePort struct {
	base      Port
	readHook  func() error
	writeHook func() error
}

// WrapPort applies middleware in declaration order, with the first middleware outermost.
func WrapPort(port Port, middleware ...PortMiddleware) Port {
	assert.That(port != nil, "WrapPort called with nil port")

	var id string
	var mtu int
	if assert.Enabled {
		id = port.ID()
		mtu = port.MTU()
		assert.That(id != "", "WrapPort called with empty port ID")
		assert.That(mtu > 0, "WrapPort called with invalid MTU %d", mtu)
	}

	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			port = middleware[i](port)
			if assert.Enabled {
				assert.That(port != nil, "middleware %d returned nil port", i)
				assert.That(port.ID() == id,
					"middleware %d changed port ID %q -> %q", i, id, port.ID())
				assert.That(port.MTU() == mtu,
					"middleware %d changed MTU %d -> %d", i, mtu, port.MTU())
			}
		}
	}
	return port
}

func (m *middlewarePort) ID() string {
	if assert.Enabled {
		m.assertValid()
	}
	return m.base.ID()
}

func (m *middlewarePort) MTU() int {
	if assert.Enabled {
		m.assertValid()
	}
	return m.base.MTU()
}

func (m *middlewarePort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		m.assertValid()
	}
	if m.readHook != nil {
		if err := m.readHook(); err != nil {
			return nil, err
		}
	}
	return m.base.ReadFrame()
}

func (m *middlewarePort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		m.assertValid()
	}
	assert.That(frame != nil, "middlewarePort writing nil frame")

	if m.writeHook != nil {
		if err := m.writeHook(); err != nil {
			return err
		}
	}
	return m.base.WriteFrame(frame)
}

func (m *middlewarePort) Close() error {
	if assert.Enabled {
		m.assertValid()
	}
	return m.base.Close()
}

func (m *middlewarePort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(m != nil, "nil middlewarePort")
	assert.That(m.base != nil, "middlewarePort has nil base")
	assert.That(m.base.ID() != "", "middleware base has empty ID")
	assert.That(m.base.MTU() > 0,
		"middleware base has invalid MTU %d", m.base.MTU())
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

// WithLatency wraps a port with fixed ingress and egress latency.
func WithLatency(port Port, delay time.Duration) Port {
	return WrapPort(port, Latency(delay))
}

// PacketLossOptions configures independent ingress and egress frame loss.
// Random is optional and is primarily useful for deterministic tests.
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

// NewPacketLossMiddleware builds middleware that treats dropped frames as successful operations.
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

// WithPacketLoss wraps a port with configurable packet loss.
func WithPacketLoss(port Port, opts PacketLossOptions) (Port, error) {
	middleware, err := NewPacketLossMiddleware(opts)
	if err != nil {
		return nil, err
	}
	return WrapPort(port, middleware), nil
}

func (p *packetLossPort) ID() string {
	if assert.Enabled {
		p.assertValid()
	}
	return p.base.ID()
}

func (p *packetLossPort) MTU() int {
	if assert.Enabled {
		p.assertValid()
	}
	return p.base.MTU()
}

func (p *packetLossPort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		p.assertValid()
	}
	for {
		frame, err := p.base.ReadFrame()
		if err != nil || !p.drop() {
			return frame, err
		}
	}
}

func (p *packetLossPort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		p.assertValid()
	}
	assert.That(frame != nil, "packetLossPort writing nil frame")

	if p.drop() {
		return nil
	}
	return p.base.WriteFrame(frame)
}

func (p *packetLossPort) Close() error {
	if assert.Enabled {
		p.assertValid()
	}
	return p.base.Close()
}

func (p *packetLossPort) drop() bool {
	if p.probability == 1 {
		return true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	value := p.random()
	assert.That(value >= 0 && value < 1,
		"packet loss RNG returned %f; expected [0,1)", value)
	return value < p.probability
}

func (p *packetLossPort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(p != nil, "nil packetLossPort")
	assert.That(p.base != nil, "packetLossPort has nil base")
	assert.That(p.random != nil, "packetLossPort has nil RNG")
	assert.That(p.probability >= 0 && p.probability <= 1,
		"invalid probability %f", p.probability)
}
