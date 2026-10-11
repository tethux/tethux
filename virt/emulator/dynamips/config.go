package dynamips

import (
	"net/netip"

	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/virt/emulator"
)

// Config describes a Dynamips router. Image is a local IOS image path and
// MemoryMB is the router RAM size in MiB.
type Config struct {
	emulator.Config
	// State identifies the durable directory containing this router's native files.
	State storage.Ref

	Platform   PlatformConfig
	InstanceID int32

	Cards       []Card
	Interfaces  []Interface
	ConsolePort uint16
	// EnhancedConsole selects local console editing and history when attaching.
	EnhancedConsole bool

	// Nil configs reopen saved NVRAM unchanged; non-nil configs explicitly provision it.
	StartupConfig []byte
	PrivateConfig []byte
}

// Card describes a card installed in a router slot.
type Card struct {
	Slot uint32
	Name string
}

// Interface maps a logical Ethernet port to a card port and UDP NIO.
type Interface struct {
	ID   string
	Slot uint32
	Port uint32
	UDP  UDPConfig
}

// UDPConfig describes an Ethernet-over-UDP endpoint.
type UDPConfig struct {
	LocalPort uint16
	Remote    netip.AddrPort
}
