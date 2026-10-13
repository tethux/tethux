package shitnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	native "github.com/tethux/shitnet"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/shitnet/errs"
)

func TestNativeEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a := &Adapter{}
	t.Cleanup(func() {
		if closeErr := a.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	left, right := freeUDP(t), freeUDP(t)
	peer, listenErr := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	t.Cleanup(func() {
		if closeErr := peer.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	peerAddr, ok := peer.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("unexpected UDP address")
	}
	stack, createErr := native.New(native.Config{IP: netip.MustParseAddr("10.77.0.3"), MAC: native.MAC{2, 0, 0, 0, 0, 3}})
	if createErr != nil {
		t.Fatal(createErr)
	}
	t.Cleanup(stack.Close)
	configs := []Config{
		{ID: "a", IP: netip.MustParseAddr("10.77.0.1"), MAC: native.MAC{2, 0, 0, 0, 0, 1}, Local: left, Remote: peerAddr.AddrPort()},
		{ID: "b", IP: netip.MustParseAddr("10.77.0.2"), MAC: native.MAC{2, 0, 0, 0, 0, 2}, Local: right, Remote: peerAddr.AddrPort()},
	}
	for _, cfg := range configs {
		if _, createErr := a.CreateEndpoint(ctx, &cfg); createErr != nil {
			t.Fatal(createErr)
		}
		if startErr := a.Start(ctx, cfg.ID); startErr != nil {
			t.Fatal(startErr)
		}
	}
	for _, cfg := range configs {
		checkReply(t, peer, stack, &cfg)
	}
	if restartErr := a.Restart(ctx, "a"); restartErr != nil {
		t.Fatal(restartErr)
	}
	checkReply(t, peer, stack, &configs[0])
	if suspendErr := a.Suspend(ctx, "a"); !errors.Is(suspendErr, errs.ErrUnsupported) {
		t.Fatal(suspendErr)
	}
	nodes, listErr := a.List(ctx)
	if listErr != nil || len(nodes) != 2 {
		t.Fatalf("list: %v %v", nodes, listErr)
	}
	if stopErr := a.Stop(ctx, "a"); stopErr != nil {
		t.Fatal(stopErr)
	}
	if stopErr := a.Stop(ctx, "a"); stopErr != nil {
		t.Fatal(stopErr)
	}
	state, stateErr := a.State(ctx, "a")
	if stateErr != nil || state != virt.NodeStopped {
		t.Fatalf("stop: %v %v", state, stateErr)
	}
	if deleteErr := a.Delete(ctx, "a"); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	conn, bindErr := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(left))
	if bindErr != nil {
		t.Fatalf("UDP lease survived deletion: %v", bindErr)
	}
	if closeErr := conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
}

// checkReply drives real ARP and ICMP traffic from an independent native peer.
func checkReply(t *testing.T, peer *net.UDPConn, stack *native.Shitnet, cfg *Config) {
	t.Helper()
	if requestErr := stack.ARPRequest(cfg.IP); requestErr != nil {
		t.Fatal(requestErr)
	}
	exchange(t, peer, stack, cfg.Local, native.EventARPLearned)
	queued, queueErr := stack.ICMPEchoRequest(cfg.IP, 1, 1, []byte("peer"))
	if queueErr != nil || queued != native.QueueQueued {
		t.Fatalf("queue echo: %v %v", queued, queueErr)
	}
	event := exchange(t, peer, stack, cfg.Local, native.EventICMPEchoReply)
	if event.ICMP == nil || event.ICMP.Source != cfg.IP || event.ICMP.Identifier != 1 ||
		event.ICMP.Sequence != 1 || string(event.ICMP.Payload) != "peer" {
		t.Fatalf("unexpected echo reply: %+v", event)
	}
}

func exchange(t *testing.T, peer *net.UDPConn, stack *native.Shitnet, remote netip.AddrPort, expected native.EventType) native.Event {
	t.Helper()
	if deadlineErr := peer.SetReadDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
		t.Fatal(deadlineErr)
	}
	buffer := make([]byte, 65536)
	for {
		for {
			poll, pollErr := stack.PollTX()
			if pollErr != nil {
				t.Fatal(pollErr)
			}
			if !poll.Available {
				break
			}
			if _, writeErr := peer.WriteToUDPAddrPort(poll.Frame, remote); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		read, source, readErr := peer.ReadFromUDPAddrPort(buffer)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if source != remote {
			t.Fatalf("unexpected frame source: %s", source)
		}
		if receiveErr := stack.Receive(buffer[:read]); receiveErr != nil {
			t.Fatal(receiveErr)
		}
		for {
			poll, pollErr := stack.PollEvent()
			if pollErr != nil {
				t.Fatal(pollErr)
			}
			if !poll.Available {
				break
			}
			if poll.Event.Type == expected {
				return poll.Event
			}
		}
	}
}

func freeUDP(t *testing.T) netip.AddrPort {
	t.Helper()
	conn, listenErr := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("UDP listener returned an unexpected address")
	}
	result := addr.AddrPort()
	if closeErr := conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	return result
}
