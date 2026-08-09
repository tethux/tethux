package storage

import "context"

type AccessMode string

const (
	AccessReadOnly  AccessMode = "read-only"
	AccessReadWrite AccessMode = "read-write"
)

type LocationKind string

const (
	LocationPath LocationKind = "path"
	LocationURI  LocationKind = "uri"
)

type RuntimeLocation struct {
	Kind  LocationKind
	Value string
}

type PrepareRequest struct {
	Ref        Ref
	NodeID     string
	AccessMode AccessMode
}

type Prepared struct {
	Ref      Ref
	Location RuntimeLocation
}

type Manager interface {
	Prepare(
		ctx context.Context,
		req PrepareRequest,
	) (*Prepared, error)

	Release(
		ctx context.Context,
		prepared *Prepared,
	) error
}
