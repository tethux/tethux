package dynamips

import (
	native "github.com/tethux/dynamips-downgrade"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
)

// Adapter creates routers using a caller-owned native runtime.
// Calls must be externally serialized.
type Adapter struct {
	Runtime *native.Runtime
	Routers map[string]*Router
}

var _ Provider = (*Adapter)(nil)

// Info identifies the provider. Optional capabilities remain disabled.
func (a *Adapter) Info() virt.ProviderInfo {
	return virt.ProviderInfo{
		Name: "dynamips", DisplayName: "Dynamips", Kind: virt.ProviderKindEmulator,
		Capabilities: virt.Capabilities{Console: true, Pause: true},
	}
}

// Router groups the native handles owned by one router.
type Router struct {
	Node            emulator.Node
	InstanceID      int32
	EnhancedConsole bool
	StateDirectory  string
	VM              *native.VM
	NIOs            map[string]*native.NIO
}
