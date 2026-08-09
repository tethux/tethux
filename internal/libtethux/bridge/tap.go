package bridge

import (
	"github.com/songgao/water"
	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

type TAPPort struct {
	id    string
	mtu   int
	iface *water.Interface
}

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

func (t *TAPPort) ID() string {
	return t.id
}

func (t *TAPPort) MTU() int {
	return t.mtu
}

func (t *TAPPort) ReadFrame() (Frame, error) {
	buf := make([]byte, 65536)
	n, err := t.iface.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (t *TAPPort) WriteFrame(frame Frame) error {
	_, err := t.iface.Write(frame)
	return err
}

func (t *TAPPort) Close() error {
	return t.iface.Close()
}
