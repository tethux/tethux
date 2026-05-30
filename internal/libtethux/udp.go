package libtethux

import (
	"net"
	"time"
)

type UDPPort struct {
	id         string
	mtu        int
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
}

func (u *UDPPort) ID() string {
	return u.id
}

func (u *UDPPort) MTU() int {
	return u.mtu
}

func (u *UDPPort) ReadFrame() (Frame, error) {
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

func (u *UDPPort) WriteFrame(frame Frame) error {
	_, err := u.conn.WriteToUDP(frame, u.remoteAddr)
	return err
}

func (u *UDPPort) Close() error {
	return u.conn.Close()
}
