package bridge

import (
	"net"
	"time"

	"github.com/tethux/tethux/internal/assert"
)

// UDPPort transports Ethernet frames in UDP datagrams.
type UDPPort struct {
	id         string
	mtu        int
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
}

// ID returns the stable port identifier.
func (u *UDPPort) ID() string {
	if assert.Enabled {
		u.assertValid()
	}
	return u.id
}

// MTU returns the configured frame MTU.
func (u *UDPPort) MTU() int {
	if assert.Enabled {
		u.assertValid()
	}
	return u.mtu
}

// ReadFrame reads one Ethernet frame from a UDP datagram.
func (u *UDPPort) ReadFrame() (Frame, error) {
	if assert.Enabled {
		u.assertValid()
	}
	buf := make([]byte, 65536)

	if err := u.conn.SetReadDeadline(time.Now().Add(readPollInterval)); err != nil {
		return nil, err
	}

	n, _, err := u.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// WriteFrame writes one Ethernet frame as a UDP datagram.
func (u *UDPPort) WriteFrame(frame Frame) error {
	if assert.Enabled {
		u.assertValid()
	}
	assert.That(frame != nil, "UDPPort %q writing nil frame", u.id)
	_, err := u.conn.WriteToUDP(frame, u.remoteAddr)
	return err
}

// Close closes the UDP socket.
func (u *UDPPort) Close() error {
	if assert.Enabled {
		u.assertValid()
	}
	return u.conn.Close()
}

func (u *UDPPort) assertValid() {
	if !assert.Enabled {
		return
	}
	assert.That(u != nil, "nil UDPPort")
	assert.That(u.id != "", "UDPPort has empty ID")
	assert.That(u.mtu > 0, "UDPPort %q has invalid MTU %d", u.id, u.mtu)
	assert.That(u.conn != nil, "UDPPort %q has nil connection", u.id)
	assert.That(u.remoteAddr != nil, "UDPPort %q has nil remote address", u.id)
}
