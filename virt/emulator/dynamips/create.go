package dynamips

import (
	"context"
	"errors"
	"math"
	"path/filepath"

	native "github.com/tethux/dynamips-downgrade"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

// CreateRouter configures a stopped router and its UDP attachments.
// Unsupported platform overrides are rejected before native allocation.
// Failed rollback resources remain in Routers for explicit cleanup.
func (a *Adapter) CreateRouter(ctx context.Context, cfg *RuntimeConfig) (*emulator.Node, error) {
	contextErr := ctx.Err()
	if contextErr != nil {
		return nil, errs.Wrap("create router", errs.ErrCreate, "", contextErr)
	}
	if a == nil || a.Runtime == nil {
		return nil, errs.New("create router", errs.ErrRuntime, "")
	}
	configErr := validateConfig(cfg)
	if configErr != nil {
		return nil, configErr
	}
	stateDirectory := cfg.StateDirectory
	if stateDirectory != "" {
		resolved, resolveErr := filepath.EvalSymlinks(stateDirectory)
		if resolveErr != nil {
			return nil, errs.Wrap("resolve router state", errs.ErrConfig, cfg.ID, resolveErr)
		}
		stateDirectory = resolved
	}
	if _, exists := a.Routers[cfg.ID]; exists {
		return nil, errs.New("create router", errs.ErrDuplicate, cfg.ID)
	}
	for _, existing := range a.Routers {
		if existing.Node.Name == cfg.Name || existing.InstanceID == cfg.InstanceID ||
			(stateDirectory != "" && existing.StateDirectory == stateDirectory) {
			return nil, errs.New("create router", errs.ErrDuplicate, cfg.Name)
		}
	}
	vm, createErr := a.Runtime.CreateVM(native.VMConfig{
		Name: cfg.Name, InstanceID: cfg.InstanceID, Platform: cfg.Platform.Name,
		StateDirectory: stateDirectory,
	})
	if createErr != nil {
		return nil, errs.Wrap("create router", errs.ErrCreate, cfg.ID, createErr)
	}
	router := &Router{
		Node: emulator.Node{Node: virt.Node{
			ID: cfg.ID, Name: cfg.Name, State: virt.NodeStopped,
			Console: virt.Console{Type: virt.ConsoleNone},
		}},
		InstanceID: cfg.InstanceID, VM: vm, NIOs: make(map[string]*native.NIO),
		EnhancedConsole: cfg.EnhancedConsole,
		StateDirectory:  stateDirectory,
	}
	if a.Routers == nil {
		a.Routers = make(map[string]*Router)
	}
	configureErr := a.configureRouter(ctx, cfg, router)
	if configureErr != nil {
		cleanupErr := cleanupRouter(router)
		if cleanupErr != nil {
			a.Routers[cfg.ID] = router
		}
		return nil, errors.Join(errs.Wrap("create router", errs.ErrCreate, cfg.ID, configureErr), cleanupErr)
	}
	a.Routers[cfg.ID] = router
	node := router.Node
	return &node, nil
}

func (a *Adapter) configureRouter(ctx context.Context, cfg *RuntimeConfig, router *Router) error {
	vm := router.VM
	steps := make([]func() error, 0, 7)
	if specific := cfg.Platform.C7200; specific != nil {
		if specific.NPE != "" {
			steps = append(steps, func() error { return vm.SetC7200NPE(specific.NPE) })
		}
		if specific.Midplane != "" {
			steps = append(steps, func() error { return vm.SetC7200Midplane(specific.Midplane) })
		}
	}
	if cfg.Platform.MAC != nil {
		steps = append(steps, func() error { return vm.SetC7200MACAddr(*cfg.Platform.MAC) })
	}
	steps = append(steps, func() error { return vm.SetIOS(cfg.Image) })
	if memory := cfg.MemoryMB; memory != 0 {
		if memory < 0 || uint64(memory) > math.MaxUint32 {
			return errs.New("configure RAM", errs.ErrConfig, cfg.ID)
		}
		ram := uint32(memory)
		steps = append(steps, func() error { return vm.SetRAM(ram) })
	}
	if cfg.ConsolePort != 0 {
		steps = append(steps, func() error { return vm.SetConsoleTCPPort(cfg.ConsolePort) })
		router.Node.Console = virt.Console{Type: virt.ConsoleTelnet, Host: "127.0.0.1", Port: cfg.ConsolePort}
	}
	if cfg.StartupConfig != nil || cfg.PrivateConfig != nil {
		steps = append(steps, func() error { return vm.PushConfig(cfg.StartupConfig, cfg.PrivateConfig) })
	}
	for _, step := range steps {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if stepErr := step(); stepErr != nil {
			return stepErr
		}
	}
	for _, card := range cfg.Cards {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if cardErr := vm.AddCard(card.Slot, card.Name); cardErr != nil {
			return cardErr
		}
	}
	for _, iface := range cfg.Interfaces {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		nio, createErr := a.Runtime.CreateUDP(native.UDPConfig{
			Name: cfg.Name + "/" + iface.ID, LocalPort: iface.UDP.LocalPort,
			RemoteHost: iface.UDP.Remote.Addr().String(), RemotePort: iface.UDP.Remote.Port(),
		})
		if createErr != nil {
			return createErr
		}
		router.NIOs[iface.ID] = nio
		attachErr := vm.AttachNIO(iface.Slot, iface.Port, nio)
		if attachErr != nil {
			return attachErr
		}
	}
	return ctx.Err()
}

func cleanupRouter(router *Router) error {
	if router.VM != nil {
		deleteErr := router.VM.Delete()
		if deleteErr != nil {
			return errs.Wrap("delete partial router", errs.ErrCleanup, router.Node.ID, deleteErr)
		}
		router.VM = nil
	}
	var failures []error
	for id, nio := range router.NIOs {
		deleteErr := nio.Delete()
		if deleteErr != nil {
			failures = append(failures, errs.Wrap("delete partial NIO", errs.ErrCleanup, id, deleteErr))
			continue
		}
		delete(router.NIOs, id)
	}
	return errors.Join(failures...)
}
