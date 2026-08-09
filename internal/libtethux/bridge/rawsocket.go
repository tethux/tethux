package bridge

import (
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/tethux/tethux/internal/assert"
	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

const (
	readPollInterval = 100 * time.Millisecond
	maxInt32         = 1<<31 - 1
)

// RawSocketPort transports Ethernet frames through a Linux packet socket.
type RawSocketPort struct {
	id     string
	mtu    int
	fd     int
	ifName string
}

// ID returns the stable port identifier.
func (r *RawSocketPort) ID() string {
	if assert.Enabled {
		r.assertValid()
	}
	return r.id
}

// MTU returns the configured frame MTU.
func (r *RawSocketPort) MTU() int {
	if assert.Enabled {
		r.assertValid()
	}
	return r.mtu
}

// ReadFrame reads one Ethernet frame from the packet socket.
func (r *RawSocketPort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		r.assertValid()
	}
	if r.fd > maxInt32 {
		return nil, syscall.EINVAL
	}

	fd := int32(r.fd) // #nosec G115 guarded above for unix.PollFd.
	pollFDs := []unix.PollFd{{
		Fd:     fd,
		Events: unix.POLLIN,
	}}
	ready, err := unix.Poll(pollFDs, int(readPollInterval/time.Millisecond))
	if err != nil {
		return nil, err
	}
	if ready == 0 {
		return nil, errs.ErrReadTimeout
	}

	buf := make([]byte, 65536)
	n, _, err := syscall.Recvfrom(r.fd, buf, 0)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// WriteFrame writes one Ethernet frame through the packet socket.
func (r *RawSocketPort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		r.assertValid()
	}
	assert.That(frame != nil, "RawSocketPort %q writing nil frame", r.id)
	return syscall.Sendto(r.fd, frame, 0, nil)
}

// Close closes the packet socket.
func (r *RawSocketPort) Close() error {
	if assert.Enabled {
		r.assertValid()
	}
	return syscall.Close(r.fd)
}

func (r *RawSocketPort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(r != nil, "nil RawSocketPort")
	assert.That(r.id != "", "RawSocketPort has empty ID")
	assert.That(r.mtu > 0, "RawSocketPort %q has invalid MTU %d", r.id, r.mtu)
	assert.That(r.fd >= 0, "RawSocketPort %q has invalid fd %d", r.id, r.fd)
	assert.That(r.ifName != "", "RawSocketPort %q has empty interface name", r.id)
}
