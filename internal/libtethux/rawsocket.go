package libtethux

import (
	"errors"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const readPollInterval = 100 * time.Millisecond
const maxInt32 = 1<<31 - 1

var errReadTimeout = errors.New("port read timeout")

type RawSocketPort struct {
	id     string
	mtu    int
	fd     int
	ifName string
}

func (r *RawSocketPort) ID() string {
	return r.id
}

func (r *RawSocketPort) MTU() int {
	return r.mtu
}

func (r *RawSocketPort) ReadFrame() (Frame, error) {
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
		return nil, errReadTimeout
	}

	buf := make([]byte, 65536)
	n, _, err := syscall.Recvfrom(r.fd, buf, 0)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (r *RawSocketPort) WriteFrame(frame Frame) error {
	return syscall.Sendto(r.fd, frame, 0, nil)
}

func (r *RawSocketPort) Close() error {
	return syscall.Close(r.fd)
}
