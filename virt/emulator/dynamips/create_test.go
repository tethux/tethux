package dynamips

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	native "github.com/tethux/dynamips-downgrade"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func TestCreateRouter(t *testing.T) {
	t.Chdir(t.TempDir())
	runtime, err := native.New()
	if err != nil {
		t.Fatal(err)
	}
	a := &Adapter{Runtime: runtime}
	t.Cleanup(func() {
		for _, router := range a.Routers {
			if cleanupErr := cleanupRouter(router); cleanupErr != nil {
				t.Error(cleanupErr)
			}
		}
		if closeErr := runtime.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	ctx := context.Background()
	localPort := availableUDPPort(t)
	cfg := &RuntimeConfig{Config: Config{
		Config: emulator.Config{NodeConfig: virt.NodeConfig{
			ID: "router-1", Name: "creation-test", Image: "unused-ios.bin", MemoryMB: 256,
		}},
		Platform: PlatformConfig{
			Name: "c7200", MAC: &[6]byte{0x02, 0, 0, 0, 0, 1},
			C7200: &C7200Config{NPE: "npe-400", Midplane: "vxr"},
		},
		InstanceID: 1, ConsolePort: 2000,
		EnhancedConsole: true,
		StartupConfig:   []byte("hostname CreationTest\nend\n"), PrivateConfig: []byte{},
		Cards: []Card{{Slot: 1, Name: "PA-FE-TX"}},
		Interfaces: []Interface{{
			ID: "eth0", Slot: 1, Port: 0,
			UDP: UDPConfig{LocalPort: localPort, Remote: netip.MustParseAddrPort("127.0.0.1:24001")},
		}},
	}}
	node, err := a.CreateRouter(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != cfg.ID || node.State != virt.NodeStopped || node.Console.Port != cfg.ConsolePort {
		t.Fatalf("unexpected node: %+v", node)
	}
	router := a.Routers[cfg.ID]
	if !router.EnhancedConsole {
		t.Fatal("enhanced console preference was not retained")
	}
	status, err := router.VM.Status()
	if err != nil || (status != native.VMHalted && status != native.VMShutdown) {
		t.Fatalf("router must remain stopped: status=%v error=%v", status, err)
	}
	data, err := router.VM.ExtractConfig()
	if err != nil || !bytes.Equal(data.Startup, cfg.StartupConfig) {
		t.Fatalf("startup configuration was not applied: data=%q error=%v", data.Startup, err)
	}
	if _, err = router.NIOs["eth0"].Stats(); err != nil {
		t.Fatal(err)
	}
	if _, err = a.CreateRouter(ctx, cfg); !errors.Is(err, errs.ErrDuplicate) {
		t.Fatalf("duplicate ID: %v", err)
	}
	err = cleanupRouter(router)
	if err != nil {
		t.Fatal(err)
	}
	delete(a.Routers, cfg.ID)

	// Attaching to an invalid slot fails after both NIOs have been created.
	cfg.Interfaces = append(cfg.Interfaces, Interface{
		ID: "eth1", Slot: 99, Port: 0,
		UDP: UDPConfig{LocalPort: availableUDPPort(t), Remote: cfg.Interfaces[0].UDP.Remote},
	})
	if _, err = a.CreateRouter(ctx, cfg); !errors.Is(err, errs.ErrCreate) {
		t.Fatalf("expected native allocation failure: %v", err)
	}
	if len(a.Routers) != 0 {
		t.Fatal("rollback left tracked resources")
	}
	cfg.Interfaces = cfg.Interfaces[:1]
	if _, err = a.CreateRouter(ctx, cfg); err != nil {
		t.Fatalf("retry must reuse the VM name and UDP port after rollback: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = a.CreateRouter(canceled, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause was lost: %v", err)
	}
	cfg.Platform.Chassis = "3660"
	if _, err = a.CreateRouter(ctx, cfg); !errors.Is(err, errs.ErrUnsupported) {
		t.Fatalf("unavailable chassis setter: %v", err)
	}
	for _, platform := range []string{"c1700", "c2600", "c2691", "c3600", "c3725", "c3745"} {
		t.Run(platform, func(t *testing.T) {
			other := &RuntimeConfig{Config: Config{
				Config: emulator.Config{NodeConfig: virt.NodeConfig{
					ID: platform, Name: platform, Image: "unused-ios.bin", MemoryMB: 256,
				}},
				Platform: PlatformConfig{Name: platform}, InstanceID: 10,
			}}
			if _, createErr := a.CreateRouter(ctx, other); createErr != nil {
				t.Fatal(createErr)
			}
			if cleanupErr := cleanupRouter(a.Routers[platform]); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
			delete(a.Routers, platform)
		})
	}
	t.Run("stored-state", func(t *testing.T) { testStoredState(t, a) })
}

func availableUDPPort(t *testing.T) uint16 {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	address, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("UDP listener returned an unexpected address type")
	}
	port := address.AddrPort().Port()
	if closeErr := conn.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	return port
}
