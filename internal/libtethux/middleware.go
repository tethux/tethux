package libtethux

import "time"

type middlewarePort struct {
	base      Port
	readHook  func() error
	writeHook func() error
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

func WithLatency(port Port, delay time.Duration) Port {
	if delay <= 0 {
		return port
	}

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
