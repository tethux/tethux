package libtethux

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type testPort struct {
	id     string
	mtu    int
	readCh chan Frame

	mu     sync.Mutex
	writes []Frame
	closed bool
}

func newTestPort(id string) *testPort {
	return &testPort{
		id:     id,
		mtu:    1500,
		readCh: make(chan Frame, 8),
	}
}

func (p *testPort) ID() string {
	return p.id
}

func (p *testPort) MTU() int {
	return p.mtu
}

func (p *testPort) ReadFrame() (Frame, error) {
	frame, ok := <-p.readCh
	if !ok {
		return nil, errors.New("closed")
	}
	return frame, nil
}

func (p *testPort) WriteFrame(frame Frame) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, append(Frame(nil), frame...))
	return nil
}

func (p *testPort) Close() error {
	p.mu.Lock()
	if !p.closed {
		close(p.readCh)
		p.closed = true
	}
	p.mu.Unlock()
	return nil
}

func (p *testPort) inject(frame Frame) {
	p.readCh <- append(Frame(nil), frame...)
}

func (p *testPort) writeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.writes)
}

func (p *testPort) lastWrite() Frame {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.writes) == 0 {
		return nil
	}
	return append(Frame(nil), p.writes[len(p.writes)-1]...)
}

func TestSwitchFloodsBroadcastToOtherPorts(t *testing.T) {
	sw := NewSwitch(SwitchOptions{})
	left := newTestPort("left")
	right := newTestPort("right")
	third := newTestPort("third")

	mustAttach(t, sw, left, right, third)
	mustStart(t, sw)
	defer mustStop(t, sw)

	frame := ethernetFrame(
		"ff:ff:ff:ff:ff:ff",
		"02:00:00:00:00:01",
		0x0800,
		[]byte("hello"),
	)
	left.inject(frame)

	waitForWrites(t, right, 1)
	waitForWrites(t, third, 1)

	if got := right.lastWrite(); string(got) != string(frame) {
		t.Fatalf("right frame mismatch: got %x want %x", got, frame)
	}
	if got := third.lastWrite(); string(got) != string(frame) {
		t.Fatalf("third frame mismatch: got %x want %x", got, frame)
	}
}

func TestSwitchLearnsAndForwardsKnownUnicast(t *testing.T) {
	sw := NewSwitch(SwitchOptions{})
	left := newTestPort("left")
	right := newTestPort("right")
	third := newTestPort("third")

	mustAttach(t, sw, left, right, third)
	mustStart(t, sw)
	defer mustStop(t, sw)

	right.inject(ethernetFrame(
		"ff:ff:ff:ff:ff:ff",
		"02:00:00:00:00:02",
		0x0800,
		[]byte("learn-right"),
	))
	waitForWrites(t, left, 1)
	waitForWrites(t, third, 1)

	left.inject(ethernetFrame(
		"02:00:00:00:00:02",
		"02:00:00:00:00:01",
		0x0800,
		[]byte("to-right"),
	))
	waitForWrites(t, right, 1)

	if got := third.writeCount(); got != 1 {
		t.Fatalf("third should only receive the initial flood, got %d writes", got)
	}

	got := right.lastWrite()
	want := ethernetFrame(
		"02:00:00:00:00:02",
		"02:00:00:00:00:01",
		0x0800,
		[]byte("to-right"),
	)
	if string(got) != string(want) {
		t.Fatalf("right frame mismatch: got %x want %x", got, want)
	}
}

func TestSwitchCanDropUnknownUnicastFlood(t *testing.T) {
	sw := NewSwitch(SwitchOptions{DisableUnknownUnicastFlood: true})
	left := newTestPort("left")
	right := newTestPort("right")

	mustAttach(t, sw, left, right)
	mustStart(t, sw)
	defer mustStop(t, sw)

	left.inject(ethernetFrame(
		"02:00:00:00:00:09",
		"02:00:00:00:00:01",
		0x0800,
		[]byte("drop-me"),
	))

	time.Sleep(100 * time.Millisecond)

	if got := right.writeCount(); got != 0 {
		t.Fatalf("expected no forwarded writes, got %d", got)
	}
}

func TestRemovePortClosesAndRemovesFDBEntries(t *testing.T) {
	sw := NewSwitch(SwitchOptions{})
	left := newTestPort("left")
	right := newTestPort("right")

	mustAttach(t, sw, left, right)
	sw.learn(mustMAC(t, "02:00:00:00:00:02"), "right")

	if err := sw.RemovePort("right"); err != nil {
		t.Fatalf("remove port: %v", err)
	}

	if _, ok := sw.fdb["02:00:00:00:00:02"]; ok {
		t.Fatalf("fdb entry still present after port removal")
	}
	if !right.closed {
		t.Fatalf("removed port was not closed")
	}
}

func TestSwitchStopsIdleUDPReadersQuickly(t *testing.T) {
	leftAddr := freeUDPAddr(t)
	rightAddr := freeUDPAddr(t)

	left, err := NewPort(UDPScheme, &PortOptions{
		ID:        "left",
		LocalAddr: leftAddr,
		Remote:    rightAddr,
		MTU:       1500,
	})
	if err != nil {
		t.Fatalf("create left udp port: %v", err)
	}

	right, err := NewPort(UDPScheme, &PortOptions{
		ID:        "right",
		LocalAddr: rightAddr,
		Remote:    leftAddr,
		MTU:       1500,
	})
	if err != nil {
		t.Fatalf("create right udp port: %v", err)
	}

	sw := NewSwitch(SwitchOptions{})
	mustAttach(t, sw, left, right)
	mustStart(t, sw)

	start := time.Now()
	mustStop(t, sw)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("idle UDP switch stop took %s", elapsed)
	}
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	var listenConfig net.ListenConfig
	conn, err := listenConfig.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().String()
}

func mustAttach(t *testing.T, sw *Switch, ports ...Port) {
	t.Helper()
	for _, port := range ports {
		if err := sw.AttachPort(port); err != nil {
			t.Fatalf("attach %s: %v", port.ID(), err)
		}
	}
}

func mustStart(t *testing.T, sw *Switch) {
	t.Helper()
	if err := sw.Start(); err != nil {
		t.Fatalf("start switch: %v", err)
	}
}

func mustStop(t *testing.T, sw *Switch) {
	t.Helper()
	if err := sw.Stop(); err != nil {
		t.Fatalf("stop switch: %v", err)
	}
}

func waitForWrites(t *testing.T, port *testPort, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := port.writeCount(); got >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d writes on %s, got %d", want, port.ID(), port.writeCount())
}

func ethernetFrame(dst, src string, etherType uint16, payload []byte) Frame {
	dstMAC, _ := net.ParseMAC(dst)
	srcMAC, _ := net.ParseMAC(src)

	frame := make([]byte, 0, 14+len(payload))
	frame = append(frame, dstMAC...)
	frame = append(frame, srcMAC...)
	frame = append(frame, byte(etherType>>8), byte(etherType)) // #nosec G115
	frame = append(frame, payload...)
	return frame
}

func mustMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		t.Fatalf("parse mac %s: %v", value, err)
	}
	return mac
}
