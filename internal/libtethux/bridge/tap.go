package bridge

import (
	"github.com/songgao/water"
	"github.com/tethux/tethux/internal/assert"
	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

// TAPPort transports Ethernet frames through a TAP interface.
type TAPPort struct {
	id    string
	mtu   int
	iface *water.Interface
}

// NewTAPPort opens a TAP-backed port using opts.
func NewTAPPort(opts *PortOptions) (Port, error) {
	if opts.Interface == "" {
		return nil, errs.New("open tap", errs.ErrInvalidOptions, "interface name is required")
	}

	iface, err := water.New(water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: opts.Interface,
		},
	})
	if err != nil {
		return nil, errs.Wrap("open tap", errs.ErrPortSetup, opts.Interface, err)
	}

	id := opts.ID
	if id == "" {
		id = opts.Interface
	}

	return &TAPPort{
		id:    id,
		mtu:   opts.MTU,
		iface: iface,
	}, nil
}

// ID returns the stable port identifier.
func (t *TAPPort) ID() string {
	if assert.Enabled {
		t.assertValid()
	}
	return t.id
}

// MTU returns the configured frame MTU.
func (t *TAPPort) MTU() int {
	if assert.Enabled {
		t.assertValid()
	}
	return t.mtu
}

// ReadFrame reads one Ethernet frame from the TAP interface.
func (t *TAPPort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		t.assertValid()
	}
	buf := make([]byte, 65536)
	n, err := t.iface.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// WriteFrame writes one Ethernet frame through the TAP interface.
func (t *TAPPort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		t.assertValid()
	}
	assert.That(frame != nil, "TAPPort %q writing nil frame", t.id)
	_, err := t.iface.Write(frame)
	return err
}

// Close closes the TAP interface.
func (t *TAPPort) Close() error {
	if assert.Enabled {
		t.assertValid()
	}
	return t.iface.Close()
}

func (t *TAPPort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(t != nil, "nil TAPPort")
	assert.That(t.id != "", "TAPPort has empty ID")
	assert.That(t.mtu > 0, "TAPPort %q has invalid MTU %d", t.id, t.mtu)
	assert.That(t.iface != nil, "TAPPort %q has nil interface", t.id)
}
