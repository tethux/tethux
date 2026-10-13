package shitnet

import (
	"context"

	"github.com/tethux/tethux/virt"
)

// Provider extends the common workload lifecycle with typed endpoint creation.
type Provider interface {
	virt.Provider

	CreateEndpoint(ctx context.Context, cfg *Config) (*virt.Node, error)
	Close() error
}
