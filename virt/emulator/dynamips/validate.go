package dynamips

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

func validateConfig(cfg *RuntimeConfig) error {
	if cfg == nil {
		return errs.New("validate router", errs.ErrConfig, "nil config")
	}
	if cfg.StateDirectory != "" && (!filepath.IsAbs(cfg.StateDirectory) || strings.ContainsRune(cfg.StateDirectory, '\x00')) {
		return errs.New("validate router", errs.ErrConfig, "state directory must be an absolute path")
	}
	if cfg.ID == "" || cfg.Name == "" || cfg.Image == "" ||
		strings.ContainsAny(cfg.Name, "/\\\x00") || strings.ContainsRune(cfg.ID+cfg.Image, '\x00') {
		return errs.New("validate router", errs.ErrConfig, "ID, name, and image are required; name must be a filename")
	}
	if cfg.InstanceID < 0 || cfg.MemoryMB < 0 || uint64(cfg.MemoryMB) > math.MaxUint32 {
		return errs.New("validate router", errs.ErrConfig, "instance ID and RAM size must fit native ranges")
	}
	switch cfg.Platform.Name {
	case "c1700", "c2600", "c2691", "c3600", "c3725", "c3745", "c7200":
	default:
		return errs.New("validate router", errs.ErrUnsupported, cfg.Platform.Name)
	}
	if cfg.Platform.Chassis != "" || (cfg.Platform.MAC != nil && cfg.Platform.Name != "c7200") {
		return errs.New("validate router", errs.ErrUnsupported, "generic chassis and MAC setters need native bindings")
	}
	if cfg.Platform.C7200 != nil {
		if cfg.Platform.Name != "c7200" {
			return errs.New("validate router", errs.ErrConfig, "C7200 settings require the c7200 platform")
		}
		if strings.ContainsRune(cfg.Platform.C7200.NPE+cfg.Platform.C7200.Midplane, '\x00') {
			return errs.New("validate router", errs.ErrConfig, "C7200 settings contain NUL")
		}
	}
	if cfg.CPUs != 0 && cfg.CPUs != 1 {
		return errs.New("validate router", errs.ErrUnsupported, "CPU count")
	}
	if cfg.AuxConsoleType != "" && cfg.AuxConsoleType != virt.ConsoleNone {
		return errs.New("validate router", errs.ErrUnsupported, "auxiliary console")
	}
	if cfg.ConsoleType != "" && cfg.ConsoleType != virt.ConsoleNone && cfg.ConsoleType != virt.ConsoleTelnet {
		return errs.New("validate router", errs.ErrUnsupported, "console transport")
	}
	if (cfg.ConsoleType == virt.ConsoleNone && cfg.ConsolePort != 0) ||
		(cfg.ConsoleType == virt.ConsoleTelnet && cfg.ConsolePort == 0) {
		return errs.New("validate router", errs.ErrConfig, "console type and port conflict")
	}
	if cfg.EnhancedConsole && cfg.ConsolePort == 0 {
		return errs.New("validate router", errs.ErrConfig, "enhanced console requires a TCP console port")
	}
	slots := make(map[uint32]struct{}, len(cfg.Cards))
	for _, card := range cfg.Cards {
		if card.Name == "" || strings.ContainsRune(card.Name, '\x00') {
			return errs.New("validate router", errs.ErrConfig, "card name")
		}
		if _, exists := slots[card.Slot]; exists {
			return errs.New("validate router", errs.ErrConfig, "duplicate card slot")
		}
		slots[card.Slot] = struct{}{}
	}
	ids := make(map[string]struct{}, len(cfg.Interfaces))
	ports := make(map[[2]uint32]struct{}, len(cfg.Interfaces))
	for _, iface := range cfg.Interfaces {
		if iface.ID == "" || strings.ContainsRune(iface.ID, '\x00') ||
			iface.UDP.LocalPort == 0 || !iface.UDP.Remote.IsValid() ||
			!iface.UDP.Remote.Addr().Is4() || iface.UDP.Remote.Port() == 0 {
			return errs.New("validate router", errs.ErrConfig, "interface requires an ID and IPv4 UDP endpoints")
		}
		port := [2]uint32{iface.Slot, iface.Port}
		_, duplicateID := ids[iface.ID]
		_, duplicatePort := ports[port]
		if duplicateID || duplicatePort {
			return errs.New("validate router", errs.ErrConfig, "duplicate interface ID or card port")
		}
		ids[iface.ID] = struct{}{}
		ports[port] = struct{}{}
	}
	return nil
}
