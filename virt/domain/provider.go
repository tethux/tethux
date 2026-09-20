package domain

import (
	"context"
	"io"

	"github.com/tethux/tethux/virt"
)

// ConsoleProvider exposes a domain's primary serial console.
type ConsoleProvider interface {
	OpenConsole(ctx context.Context, id string, input io.Reader, output io.Writer) error
}

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
