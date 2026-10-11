package dynamips

import (
	"context"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator"
)

// Provider extends the common workload lifecycle with typed router creation.
// A concrete provider must also release its owned native resources through Close.
type Provider interface {
	virt.Provider

	CreateRouter(ctx context.Context, cfg *RuntimeConfig) (*emulator.Node, error)
	Close() error
}
