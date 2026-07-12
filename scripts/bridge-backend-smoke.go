//go:build ignore

// bridge-backend-smoke is a privileged integration test for every bridge
// transport. It uses libpcap as an independent packet injector/observer and
// writes both structured JSON Lines and a pcap artifact for CI archives.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	bridge "github.com/0xveya/tethux/internal/libtethux/bridge"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

const (
	testMTU       = 1500
	testEtherType = 0x88b5
	packetTimeout = 3 * time.Second
)

type event struct {
	Schema     string         `json:"schema"`
	Backend    string         `json:"backend"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	DurationMS int64          `json:"duration_ms"`
	Parameters map[string]any `json:"parameters"`
	Metrics    map[string]any `json:"metrics"`
	Message    *string        `json:"message"`
}

type captureWriter struct {
	mu     sync.Mutex
	file   *os.File
	writer *pcapgo.Writer
}

func main() {
	os.Exit(run())
}

func run() int {
	var outputPath string
	var capturePath string
	flag.StringVar(&outputPath, "output", "bridge-backends.jsonl", "structured JSON Lines output")
	flag.StringVar(&capturePath, "pcap", "bridge-backends.pcap", "packet capture artifact")
	flag.Parse()

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "bridge backend smoke tests require root")
		return 2
	}

	output, err := createOutput(outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = output.Close() }()

	captures, err := newCaptureWriter(capturePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = captures.Close() }()

	encoder := json.NewEncoder(output)
	tests := []struct {
		name string
		run  func(*captureWriter) (map[string]any, error)
	}{
		{name: "udp", run: testUDP},
		{name: "raw", run: func(c *captureWriter) (map[string]any, error) { return testVethBackend(c, bridge.RawScheme) }},
		{name: "pcap", run: func(c *captureWriter) (map[string]any, error) { return testVethBackend(c, bridge.PcapScheme) }},
		{name: "tap", run: testTAP},
	}

	failed := false
	for _, test := range tests {
		started := time.Now().UTC()
		metrics, testErr := test.run(captures)
		finished := time.Now().UTC()
		result := event{
			Schema:     "tethux.bridge-backend/v1",
			Backend:    test.name,
			Status:     "passed",
			StartedAt:  started,
			FinishedAt: finished,
			DurationMS: finished.Sub(started).Milliseconds(),
			Parameters: map[string]any{"ether_type": testEtherType, "mtu": testMTU, "observer": "libpcap"},
			Metrics:    metrics,
		}
		if testErr != nil {
			failed = true
			result.Status = "failed"
			message := testErr.Error()
			result.Message = &message
		}
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			fmt.Fprintln(os.Stderr, encodeErr)
			return 1
		}
		fmt.Printf("bridge backend %-4s: %s (%dms)\n", test.name, result.Status, result.DurationMS)
	}

	if failed {
		return 1
	}
	return 0
}

func testUDP(captures *captureWriter) (map[string]any, error) {
	leftObserver, err := listenUDP()
	if err != nil {
		return nil, err
	}
	defer leftObserver.Close()
	rightObserver, err := listenUDP()
	if err != nil {
		return nil, err
	}
	defer rightObserver.Close()

	left, leftAddress, err := udpPort("udp-left", leftObserver.LocalAddr().String())
	if err != nil {
		return nil, err
	}
	right, _, err := udpPort("udp-right", rightObserver.LocalAddr().String())
	if err != nil {
		_ = left.Close()
		return nil, err
	}

	sw := bridge.NewSwitch(bridge.SwitchOptions{})
	if startErr := attachAndStart(sw, left, right); startErr != nil {
		return nil, startErr
	}
	defer func() { _ = sw.Stop() }()

	remote, err := net.ResolveUDPAddr("udp", leftAddress)
	if err != nil {
		return nil, err
	}
	byteCount := 0
	frames := testFrames(1)
	for _, frame := range frames {
		if _, sendErr := leftObserver.WriteToUDP(frame, remote); sendErr != nil {
			return nil, sendErr
		}
		received, readErr := readUDPFrame(rightObserver, frame)
		if readErr != nil {
			return nil, readErr
		}
		if captureErr := captures.Write(received); captureErr != nil {
			return nil, captureErr
		}
		byteCount += len(received)
	}
	return packetMetrics(len(frames), len(frames), byteCount), nil
}

func testVethBackend(captures *captureWriter, scheme bridge.AvailableScheme) (map[string]any, error) {
	prefix := interfacePrefix(string(scheme))
	leftPortIf, leftPeerIf := prefix+"li", prefix+"lp"
	rightPortIf, rightPeerIf := prefix+"ri", prefix+"rp"
	if err := createVeth(leftPortIf, leftPeerIf); err != nil {
		return nil, err
	}
	defer deleteLink(leftPortIf)
	if err := createVeth(rightPortIf, rightPeerIf); err != nil {
		return nil, err
	}
	defer deleteLink(rightPortIf)

	injector, err := openPcap(leftPeerIf)
	if err != nil {
		return nil, err
	}
	defer injector.Close()
	observer, err := openPcap(rightPeerIf)
	if err != nil {
		return nil, err
	}
	defer observer.Close()

	left, err := bridge.NewPort(scheme, portOptions("left", leftPortIf))
	if err != nil {
		return nil, err
	}
	right, err := bridge.NewPort(scheme, portOptions("right", rightPortIf))
	if err != nil {
		_ = left.Close()
		return nil, err
	}
	sw := bridge.NewSwitch(bridge.SwitchOptions{})
	if startErr := attachAndStart(sw, left, right); startErr != nil {
		return nil, startErr
	}
	defer func() { _ = sw.Stop() }()

	marker := byte(13)
	if scheme == bridge.PcapScheme {
		marker = 14
	}
	byteCount := 0
	frames := testFrames(marker)
	for _, frame := range frames {
		if sendErr := injector.WritePacketData(frame); sendErr != nil {
			return nil, sendErr
		}
		received, readErr := awaitPcapFrame(observer, frame)
		if readErr != nil {
			return nil, readErr
		}
		if captureErr := captures.Write(received); captureErr != nil {
			return nil, captureErr
		}
		byteCount += len(received)
	}
	return packetMetrics(len(frames), len(frames), byteCount), nil
}

func testTAP(captures *captureWriter) (map[string]any, error) {
	prefix := interfacePrefix("tap")
	tapIf, linuxBridge := prefix+"t", prefix+"b"
	bridgeIf, peerIf := prefix+"vi", prefix+"vp"

	tapPort, err := bridge.NewPort(bridge.TAPScheme, portOptions("tap", tapIf))
	if err != nil {
		return nil, err
	}
	defer tapPort.Close()
	if linkErr := createVeth(bridgeIf, peerIf); linkErr != nil {
		return nil, linkErr
	}
	defer deleteLink(bridgeIf)
	if bridgeErr := runIP("link", "add", linuxBridge, "type", "bridge"); bridgeErr != nil {
		return nil, bridgeErr
	}
	defer deleteLink(linuxBridge)
	for _, args := range [][]string{
		{"link", "set", tapIf, "master", linuxBridge},
		{"link", "set", bridgeIf, "master", linuxBridge},
		{"link", "set", tapIf, "up"},
		{"link", "set", bridgeIf, "up"},
		{"link", "set", peerIf, "up"},
		{"link", "set", linuxBridge, "up"},
	} {
		if linkErr := runIP(args...); linkErr != nil {
			return nil, linkErr
		}
	}

	packetEndpoint, err := openPcap(peerIf)
	if err != nil {
		return nil, err
	}
	defer packetEndpoint.Close()
	udpObserver, err := listenUDP()
	if err != nil {
		return nil, err
	}
	defer udpObserver.Close()
	udp, udpAddress, err := udpPort("tap-udp", udpObserver.LocalAddr().String())
	if err != nil {
		return nil, err
	}

	sw := bridge.NewSwitch(bridge.SwitchOptions{})
	if startErr := attachAndStart(sw, tapPort, udp); startErr != nil {
		return nil, startErr
	}
	defer func() { _ = sw.Stop() }()

	remote, err := net.ResolveUDPAddr("udp", udpAddress)
	if err != nil {
		return nil, err
	}
	byteCount := 0
	forwardFrames := [][]byte{testFrameSize(31, 60), testFrameSize(32, 512)}
	for _, forward := range forwardFrames {
		if sendErr := packetEndpoint.WritePacketData(forward); sendErr != nil {
			return nil, sendErr
		}
		forwardReceived, readErr := readUDPFrame(udpObserver, forward)
		if readErr != nil {
			return nil, fmt.Errorf("tap ingress: %w", readErr)
		}
		if captureErr := captures.Write(forwardReceived); captureErr != nil {
			return nil, captureErr
		}
		byteCount += len(forwardReceived)
	}
	reverseFrames := [][]byte{reverseTestFrame(33), reverseTestFrameSize(34, 512)}
	for _, reverse := range reverseFrames {
		if _, sendErr := udpObserver.WriteToUDP(reverse, remote); sendErr != nil {
			return nil, sendErr
		}
		reverseReceived, readErr := awaitPcapFrame(packetEndpoint, reverse)
		if readErr != nil {
			return nil, fmt.Errorf("tap egress: %w", readErr)
		}
		if captureErr := captures.Write(reverseReceived); captureErr != nil {
			return nil, captureErr
		}
		byteCount += len(reverseReceived)
	}
	return packetMetrics(4, 4, byteCount), nil
}

func portOptions(id, name string) *bridge.PortOptions {
	return &bridge.PortOptions{ID: id, Interface: name, MTU: testMTU, ImmediateMode: true, SnapLen: 65535}
}

func attachAndStart(sw *bridge.Switch, ports ...bridge.Port) error {
	for _, port := range ports {
		if err := sw.AttachPort(port); err != nil {
			return err
		}
	}
	return sw.Start()
}

func udpPort(id, remote string) (bridge.Port, string, error) {
	local, err := freeUDPAddress()
	if err != nil {
		return nil, "", err
	}
	port, err := bridge.NewPort(bridge.UDPScheme, &bridge.PortOptions{ID: id, LocalAddr: local, Remote: remote, MTU: testMTU})
	return port, local, err
}

func listenUDP() (*net.UDPConn, error) {
	return net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
}

func freeUDPAddress() (string, error) {
	conn, err := listenUDP()
	if err != nil {
		return "", err
	}
	address := conn.LocalAddr().String()
	return address, conn.Close()
}

func readUDPFrame(conn *net.UDPConn, want []byte) ([]byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(packetTimeout)); err != nil {
		return nil, err
	}
	buffer := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(buffer[:n], want) {
			return bytes.Clone(buffer[:n]), nil
		}
	}
}

func openPcap(name string) (*pcap.Handle, error) {
	handle, err := pcap.OpenLive(name, 65535, true, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if err := handle.SetBPFFilter(fmt.Sprintf("ether proto 0x%04x", testEtherType)); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

func awaitPcapFrame(handle *pcap.Handle, want []byte) ([]byte, error) {
	deadline := time.Now().Add(packetTimeout)
	for time.Now().Before(deadline) {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			if errors.Is(err, pcap.NextErrorTimeoutExpired) {
				continue
			}
			return nil, err
		}
		if bytes.Equal(data, want) {
			return bytes.Clone(data), nil
		}
	}
	return nil, errors.New("timed out waiting for exact Ethernet frame through libpcap")
}

func createVeth(left, right string) error {
	if err := runIP("link", "add", left, "type", "veth", "peer", "name", right); err != nil {
		return err
	}
	if err := runIP("link", "set", left, "up"); err != nil {
		deleteLink(left)
		return err
	}
	if err := runIP("link", "set", right, "up"); err != nil {
		deleteLink(left)
		return err
	}
	return nil
}

func deleteLink(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- the binary is fixed and names are generated, bounded interface names.
	_ = exec.CommandContext(ctx, "ip", "link", "delete", name).Run()
}

func runIP(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- the binary is fixed and arguments are internal test topology data.
	command := exec.CommandContext(ctx, "ip", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %v: %w: %s", args, err, bytes.TrimSpace(output))
	}
	return nil
}

func interfacePrefix(backend string) string {
	stamp := time.Now().UnixNano() % 100000
	return fmt.Sprintf("tb%c%05d", backend[0], stamp)
}

func reverseTestFrame(marker byte) []byte {
	return reverseTestFrameSize(marker, 60)
}

func testFrames(marker byte) [][]byte {
	broadcast := testFrameSize(marker+1, 128)
	copy(broadcast[:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	return [][]byte{testFrameSize(marker, 60), broadcast, testFrameSize(marker+2, 1514)}
}

func testFrameSize(marker byte, size int) []byte {
	return ethernetFrame([]byte{0x02, 0, 0, 0, marker, 2}, []byte{0x02, 0, 0, 0, marker, 1}, marker, size)
}

func reverseTestFrameSize(marker byte, size int) []byte {
	return ethernetFrame([]byte{0x02, 0, 0, 0, marker, 1}, []byte{0x02, 0, 0, 0, marker, 2}, marker, size)
}

func ethernetFrame(destination, source []byte, marker byte, size int) []byte {
	frame := make([]byte, size)
	copy(frame[0:6], destination)
	copy(frame[6:12], source)
	binary.BigEndian.PutUint16(frame[12:14], testEtherType)
	value := marker
	for index := 14; index < len(frame); index++ {
		frame[index] = value
		value++
	}
	return frame
}

func packetMetrics(sent, received, byteCount int) map[string]any {
	return map[string]any{
		"packets_sent":        sent,
		"packets_received":    received,
		"packet_loss_percent": float64(sent-received) / float64(sent) * 100,
		"bytes_received":      byteCount,
	}
}

func createOutput(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	// #nosec G304 -- the operator explicitly selects the two artifact paths.
	return os.Create(path)
}

func newCaptureWriter(path string) (*captureWriter, error) {
	file, err := createOutput(path)
	if err != nil {
		return nil, err
	}
	writer := pcapgo.NewWriter(file)
	if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &captureWriter{file: file, writer: writer}, nil
}

func (c *captureWriter) Write(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Now().UTC(), CaptureLength: len(frame), Length: len(frame)}, frame)
}

func (c *captureWriter) Close() error {
	return c.file.Close()
}
