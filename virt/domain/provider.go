package domain

import (
	"context"

	"github.com/tethux/tethux/virt"
)

// Provider extends virt.Provider with virtual-machine domain operations.
type Provider interface {
	virt.Provider

	CreateDomain(
		ctx context.Context,
		cfg *RuntimeConfig,
	) (*Node, error)

	InspectDomain(
		ctx context.Context,
		id string,
	) (*Node, error)

	PowerOff(
		ctx context.Context,
		id string,
	) error
}
