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
	configs := []Config{
		{ID: "a", IP: netip.MustParseAddr("10.77.0.1"), MAC: native.MAC{2, 0, 0, 0, 0, 1}, Local: left, Remote: right},
		{ID: "b", IP: netip.MustParseAddr("10.77.0.2"), MAC: native.MAC{2, 0, 0, 0, 0, 2}, Local: right, Remote: left},
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
		target := configs[0].IP
		if cfg.ID == "a" {
			target = configs[1].IP
		}
		result, pingErr := a.Ping(ctx, cfg.ID, target, 3)
		if pingErr != nil || result.Sent != 3 || result.Received != 3 {
			t.Fatalf("real ARP/ICMP exchange: %+v %v", result, pingErr)
		}
	}
	if restartErr := a.Restart(ctx, "a"); restartErr != nil {
		t.Fatal(restartErr)
	}
	if _, pingErr := a.Ping(ctx, "a", configs[1].IP, 1); pingErr != nil {
		t.Fatal(pingErr)
	}
	if stopErr := a.Stop(ctx, "b"); stopErr != nil {
		t.Fatal(stopErr)
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	result, timeoutErr := a.Ping(probeCtx, "a", configs[1].IP, 1)
	probeCancel()
	if !errors.Is(timeoutErr, context.DeadlineExceeded) || result.Sent != 1 || result.Received != 0 {
		t.Fatalf("lost echo and cancellation: %+v %v", result, timeoutErr)
	}
	if startErr := a.Start(ctx, "b"); startErr != nil {
		t.Fatal(startErr)
	}
	if _, pingErr := a.Ping(ctx, "a", configs[1].IP, 1); pingErr != nil {
		t.Fatalf("canceled probe blocked later traffic: %v", pingErr)
	}
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
