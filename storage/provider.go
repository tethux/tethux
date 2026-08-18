package storage

import (
	"context"
	"io"
	"io/fs"
	"time"
)

// ObjectType classifies an object returned by a Provider.
type ObjectType string

const (
	ObjectUnknown ObjectType = "unknown"
	ObjectFile    ObjectType = "file"
	ObjectDir     ObjectType = "directory"
)

// Generation identifies one version of a durable stored object.
type Generation string

// ObjectInfo describes a durable stored object.
type ObjectInfo struct {
	Ref Ref

	Type ObjectType

	Size    int64
	ModTime time.Time

	Generation Generation

	ContentType string

	// ETag contains a provider-native object identifier when available.
	ETag string

	Checksum *Checksum

	Metadata Metadata

	Kind ArtifactKind
}

// PutOptions controls how a provider stores an object.
type PutOptions struct {
	// Mode is the permission mode for newly created filesystem objects.
	// Providers without filesystem permissions may ignore it.
	Mode fs.FileMode

	ContentType string
	Metadata    Metadata
	Kind        ArtifactKind

	// ExpectedGeneration performs an optimistic concurrency check.
	//
	// An empty value means no generation check.
	ExpectedGeneration Generation

	// IfNotExists requires that the target does not already exist.
	IfNotExists bool
}

// ListOptions controls object listing.
type ListOptions struct {
	// Recursive returns all descendants beneath prefix.
	//
	// When false, List returns only immediate children.
	Recursive bool
}

// CopyOptions controls how a provider copies an object.
type CopyOptions struct {
	Overwrite bool
}

// MoveOptions controls how a provider moves an object.
type MoveOptions struct {
	Overwrite bool
}

// Capabilities describes guarantees and features provided by a durable storage
// backend.
type Capabilities struct {
	AtomicReplace bool
	AtomicMove    bool

	ConditionalWrite bool
}

// ProviderInfo describes a storage provider and its capabilities.
type ProviderInfo struct {
	Name ProviderName

	Capabilities Capabilities
}

// Provider supplies durable object-oriented storage operations.
//
// Provider does not decide workload placement, caching, staging, writeback
// timing, or runtime cleanup. Those responsibilities belong to Manager.
type Provider interface {
	Name() ProviderName

	Info() ProviderInfo

	Stat(
		ctx context.Context,
		ref Ref,
	) (*ObjectInfo, error)

	Open(
		ctx context.Context,
		ref Ref,
	) (io.ReadCloser, error)

	Put(
		ctx context.Context,
		ref Ref,
		r io.Reader,
		opts PutOptions,
	) error

	Delete(
		ctx context.Context,
		ref Ref,
	) error

	List(
		ctx context.Context,
		prefix Ref,
		opts ListOptions,
	) ([]ObjectInfo, error)

	Copy(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts CopyOptions,
	) error

	Move(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts MoveOptions,
	) error
}

// AsyncProvider exposes asynchronous variants of potentially expensive durable
// storage operations.
//
// Providers may implement this independently of Provider users requiring only
// synchronous access.
type AsyncProvider interface {
	Provider

	CopyAsync(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts CopyOptions,
	) (OperationHandle, error)

	MoveAsync(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts MoveOptions,
	) (OperationHandle, error)

	DeleteAsync(
		ctx context.Context,
		ref Ref,
	) (OperationHandle, error)
}
