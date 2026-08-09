package domain

import (
	"context"

	"github.com/tethux/tethux/internal/libtethux/virt"
)

type Provider interface {
	virt.Provider

	CreateDomain(
		ctx context.Context,
		cfg *Config,
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
