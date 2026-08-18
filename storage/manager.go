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

// PrepareMode describes how durable storage is materialized for a workload.
type PrepareMode string

const (
	// PrepareDirect exposes the durable object directly.
	PrepareDirect PrepareMode = "direct"

	// PrepareCopy creates an independent runtime copy.
	PrepareCopy PrepareMode = "copy"

	// PrepareOverlay creates writable storage backed by another object.
	PrepareOverlay PrepareMode = "overlay"
)

// Ownership describes ownership of the prepared runtime resource.
//
// Ownership applies to runtime material represented by Location. It does not
// imply ownership of the durable object identified by Ref.
type Ownership string

const (
	// OwnershipShared identifies prepared runtime material shared by multiple
	// workloads.
	OwnershipShared Ownership = "shared"

	// OwnershipNode identifies runtime material created for one workload node.
	// Release may destroy this runtime material.
	OwnershipNode Ownership = "node"

	// OwnershipProject identifies runtime material retained for a project.
	OwnershipProject Ownership = "project"

	// OwnershipExternal identifies runtime material whose lifetime is managed
	// outside the storage manager.
	OwnershipExternal Ownership = "external"
)

// ResourceType describes the kind of runtime resource requested.
type ResourceType string

const (
	ResourceTypeFile      ResourceType = "file"
	ResourceTypeDirectory ResourceType = "directory"
)

// PreparedID uniquely identifies one runtime preparation.
type PreparedID string

// LocationKind describes the form of a prepared runtime location.
type LocationKind string

const (
	// LocationPath identifies a filesystem path.
	LocationPath LocationKind = "path"

	// LocationURI identifies a provider/runtime-specific URI.
	LocationURI LocationKind = "uri"
)

// RuntimeLocation identifies storage that can be consumed by a workload
// runtime.
type RuntimeLocation struct {
	Kind  LocationKind
	Value string
}

// PrepareRequest describes storage required by a workload node.
type PrepareRequest struct {
	Ref Ref

	NodeID string

	AccessMode AccessMode

	// Mode defaults to PrepareDirect when empty.
	Mode PrepareMode

	// ResourceType is required when Create is true.
	//
	// When Create is false and ResourceType is empty, the provider may infer
	// the type from the existing object.
	ResourceType ResourceType

	// Create permits creation of the durable object when it does not exist.
	Create bool
}

// Prepared records one runtime preparation of a durable storage object.
//
// Ref always identifies the durable object.
//
// Location identifies the runtime resource consumed by the workload. For
// PrepareDirect these may represent the same underlying storage. For copy or
// overlay preparation, Location identifies separate runtime material.
//
// BaseGeneration records the durable generation from which the runtime
// resource was prepared. It is used for optimistic commit checks.
type Prepared struct {
	ID PreparedID

	Ref Ref

	BaseGeneration Generation

	NodeID string

	AccessMode AccessMode
	Mode       PrepareMode

	ResourceType ResourceType

	Ownership Ownership

	Location RuntimeLocation
}

// Manager owns runtime storage preparation, writeback, dirty state, and
// cleanup.
//
// Implementations may stage durable objects through caches or other providers.
// Workload providers consume only Prepared.Location and must not implement
// storage placement or synchronization policy themselves.
type Manager interface {
	EventSource

	Prepare(
		ctx context.Context,
		req PrepareRequest,
	) (*Prepared, error)

	PrepareAsync(
		ctx context.Context,
		req PrepareRequest,
	) (PrepareOperation, error)

	// MarkDirty records that writable prepared storage may have diverged from
	// its durable Ref.
	//
	// MarkDirty should be idempotent.
	MarkDirty(
		ctx context.Context,
		prepared *Prepared,
	) error

	Commit(
		ctx context.Context,
		prepared *Prepared,
		opts CommitOptions,
	) (*CommitResult, error)

	CommitAsync(
		ctx context.Context,
		prepared *Prepared,
		opts CommitOptions,
	) (CommitOperation, error)

	// Release releases runtime material associated with prepared.
	//
	// Release must not delete the durable object identified by Prepared.Ref
	// merely because runtime material is node-owned.
	Release(
		ctx context.Context,
		prepared *Prepared,
	) error

	ReleaseAsync(
		ctx context.Context,
		prepared *Prepared,
	) (OperationHandle, error)
}
