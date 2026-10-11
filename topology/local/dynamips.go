package local

import (
	"context"
	"fmt"
	"math"
	"net/netip"
	"path/filepath"

	"github.com/tethux/tethux/storage"
	localstorage "github.com/tethux/tethux/storage/local"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips"
)

// DynamipsOptions selects the native provider, persistent storage, and local endpoints.
type DynamipsOptions struct {
	Provider        *dynamips.Adapter
	StateRoot       string
	Image           string
	ConsoleBasePort int
}

func routerManager(root string) (*dynamips.Manager, error) {
	store, storageErr := localstorage.New(root)
	if storageErr != nil {
		return nil, storageErr
	}
	return dynamips.NewManager(store), nil
}

func (run *Running) startRouter(ctx context.Context, top *topology.Topology, node *topology.Node, spec *topology.DynamipsSpec, options Options, token string, index int) error {
	if index < 0 || index >= math.MaxInt32 || spec.RAMMB > 2048 {
		return errs.New("start router", errs.ErrUnsupported, "router count or RAM exceeds native ranges")
	}
	image := spec.Image
	if options.Dynamips.Image != "" {
		image = options.Dynamips.Image
	}
	platform := spec.Platform
	if platform == "" {
		platform = "c7200"
	}
	consolePort := spec.ConsolePort
	if consolePort == 0 {
		base := options.Dynamips.ConsoleBasePort
		if base == 0 {
			base = 20000
		}
		port := base + index
		if port < 1 || port > math.MaxUint16 {
			return errs.New("start router", errs.ErrStart, "console port range must fit within 1..65535")
		}
		consolePort = uint16(port)
	}
	cfg := &dynamips.Config{
		Config: emulator.Config{NodeConfig: virt.NodeConfig{
			ID: string(node.ID), Name: fmt.Sprintf("tethux-%s-%d", token, index), Image: image, MemoryMB: int(spec.RAMMB),
		}},
		State:    storage.Ref{Provider: localstorage.DefaultName, Key: storage.Key(filepath.Join(string(top.ID), string(node.ID)))},
		Platform: dynamips.PlatformConfig{Name: platform}, InstanceID: int32(index) + 1,
		ConsolePort: consolePort, EnhancedConsole: spec.EnhancedConsole,
		Cards:      make([]dynamips.Card, 0, len(spec.Cards)),
		Interfaces: make([]dynamips.Interface, 0, len(node.Ports)),
	}
	if platform == "c7200" {
		npe, midplane := spec.NPE, spec.Midplane
		if npe == "" {
			npe = "npe-400"
		}
		if midplane == "" {
			midplane = "vxr"
		}
		cfg.Platform.C7200 = &dynamips.C7200Config{NPE: npe, Midplane: midplane}
	} else if spec.NPE != "" || spec.Midplane != "" {
		return errs.New("start router", errs.ErrUnsupported, "NPE and midplane require c7200")
	}
	if spec.StartupConfig != "" {
		cfg.StartupConfig, cfg.PrivateConfig = []byte(spec.StartupConfig), []byte{}
	}
	for _, card := range spec.Cards {
		cfg.Cards = append(cfg.Cards, dynamips.Card{Slot: card.Slot, Name: card.Name})
	}
	mappings := make(map[topology.PortID]topology.DynamipsInterface, len(spec.Interfaces))
	for _, iface := range spec.Interfaces {
		if _, exists := mappings[iface.ID]; exists {
			return errs.New("start router", errs.ErrInvalidPort, "duplicate native port mapping")
		}
		mappings[iface.ID] = iface
	}
	for linkIndex, link := range top.Links {
		for side, endpoint := range []topology.Endpoint{link.A, link.B} {
			if endpoint.Node != node.ID {
				continue
			}
			mapping, exists := mappings[endpoint.Port]
			if !exists {
				return errs.New("start router", errs.ErrInvalidPort, "missing native mapping for "+endpoint.String())
			}
			port := options.BasePort + 2*linkIndex + side
			remote := options.BasePort + 2*linkIndex + 1 - side
			if port < 1 || port > math.MaxUint16 || remote < 1 || remote > math.MaxUint16 {
				return errs.New("start router", errs.ErrStart, "UDP port range must fit within 1..65535")
			}
			cfg.Interfaces = append(cfg.Interfaces, dynamips.Interface{
				ID: string(endpoint.Port), Slot: mapping.Slot, Port: mapping.Port,
				UDP: dynamips.UDPConfig{LocalPort: uint16(port), Remote: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(remote))},
			})
		}
	}
	run.logger.InfoContext(ctx, "Creating router", "node", node.ID, "image", image)
	created, resources, createErr := run.manager.Create(ctx, run.dynamips, cfg)
	if resources != nil {
		run.nodes[node.ID] = cfg.ID
		run.routers[node.ID] = resources
		run.order = append(run.order, node.ID)
	}
	if createErr != nil {
		return createErr
	}
	if startErr := run.dynamips.Start(ctx, created.ID); startErr != nil {
		return startErr
	}
	run.logger.InfoContext(ctx, "Router running", "node", node.ID, "console", consolePort)
	return nil
}
