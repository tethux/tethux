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

// PrepareMode describes how a workload will prepare storage.
type PrepareMode string

const (
	// PrepareDirect uses the existing durable object directly.
	PrepareDirect PrepareMode = "direct"
	// PrepareCopy creates an independent runtime copy.
	PrepareCopy PrepareMode = "copy"
	// PrepareOverlay creates writable storage backed by another object.
	PrepareOverlay PrepareMode = "overlay"
)

// Ownership describes who owns a prepared runtime resource.
type Ownership string

const (
	// OwnershipShared identifies a resource shared by multiple workloads.
	OwnershipShared Ownership = "shared"
	// OwnershipNode identifies a resource created for one workload node.
	OwnershipNode Ownership = "node"
	// OwnershipProject identifies a resource owned by a project.
	OwnershipProject Ownership = "project"
	// OwnershipExternal identifies a resource owned outside the runtime.
	OwnershipExternal Ownership = "external"
)

// ResourceType describes the kind of resource requested during preparation.
type ResourceType string

const (
	// ResourceTypeFile requests a regular file.
	ResourceTypeFile ResourceType = "file"
	// ResourceTypeDirectory requests a directory.
	ResourceTypeDirectory ResourceType = "directory"
)

// PreparedID uniquely identifies one runtime preparation.
type PreparedID string

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

	// Mode defaults to PrepareDirect when empty.
	Mode PrepareMode

	// ResourceType is required when Create is true. When empty, an existing
	// resource's type is inferred from the provider.
	ResourceType ResourceType

	// Create allows the provider to create a missing resource.
	Create bool
}

// Prepared records one runtime preparation of a durable storage reference.
//
// A prepared resource remains owned or pinned according to Ownership until
// Release is called.
type Prepared struct {
	ID         PreparedID
	Ref        Ref
	NodeID     string
	AccessMode AccessMode
	Ownership  Ownership
	Location   RuntimeLocation
}

// Manager prepares storage references for workload runtimes, commits changes,
// and releases prepared resources.
type Manager interface {
	Prepare(
		ctx context.Context,
		req PrepareRequest,
	) (*Prepared, error)

	PrepareAsync(
		ctx context.Context,
		req PrepareRequest,
	) (OperationHandle, error)

	Commit(
		ctx context.Context,
		prepared *Prepared,
		opts CommitOptions,
	) error

	CommitAsync(
		ctx context.Context,
		prepared *Prepared,
		opts CommitOptions,
	) (OperationHandle, error)

	Release(
		ctx context.Context,
		prepared *Prepared,
	) error
}
