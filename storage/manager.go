package storage

import "context"

// AccessMode describes how a workload will access prepared storage.
type AccessMode string

const (
	// AccessReadOnly requests immutable workload access.
	AccessReadOnly AccessMode = "read-only"
	// AccessReadWrite requests mutable workload access.
	AccessReadWrite AccessMode = "read-write"
)

// LocationKind describes the form of a prepared runtime location.
type LocationKind string

const (
	// LocationPath identifies a local filesystem path.
	LocationPath LocationKind = "path"
	// LocationURI identifies a provider-specific URI.
	LocationURI LocationKind = "uri"
)

// RuntimeLocation is a storage location consumable by a workload runtime.
type RuntimeLocation struct {
	Kind  LocationKind
	Value string
}

// PrepareRequest describes storage required by a workload node.
type PrepareRequest struct {
	Ref        Ref
	NodeID     string
	AccessMode AccessMode
}

// Prepared records a reference and its resolved runtime location.
type Prepared struct {
	Ref      Ref
	Location RuntimeLocation
}

// Manager prepares storage references for workload runtimes and releases them.
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
