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
	// ObjectUnknown represents an object whose type is not known.
	ObjectUnknown ObjectType = "unknown"
	// ObjectFile represents a regular file.
	ObjectFile ObjectType = "file"
	// ObjectDir represents a directory.
	ObjectDir ObjectType = "directory"
)

// Generation identifies one version of a stored object.
type Generation string

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Ref  Ref
	Type ObjectType

	Size       int64
	ModTime    time.Time
	Generation Generation

	ContentType string
	ETag        string
	Checksum    *Checksum
	Metadata    Metadata
	Kind        ArtifactKind
}

// PutOptions controls how a provider stores an object.
type PutOptions struct {
	// Mode is the permission mode for a newly created object.
	Mode fs.FileMode
}

// CopyOptions controls how a provider copies an object.
type CopyOptions struct {
	// Overwrite permits replacing an existing destination.
	Overwrite bool
}

// MoveOptions controls how a provider moves an object.
type MoveOptions struct {
	// Overwrite permits replacing an existing destination.
	Overwrite bool
}

// Capabilities describes guarantees and features provided by a backend.
type Capabilities struct {
	AtomicReplace    bool
	AtomicMove       bool
	ConditionalWrite bool
}

// ProviderInfo describes a storage provider and its capabilities.
type ProviderInfo struct {
	Name         ProviderName
	Capabilities Capabilities
}

// Provider supplies object-oriented storage operations.
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
	) ([]ObjectInfo, error)

	// Copy copies an object from src to dst according to opts.
	Copy(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts CopyOptions,
	) error

	// Move moves an object from src to dst according to opts.
	Move(
		ctx context.Context,
		src Ref,
		dst Ref,
		opts MoveOptions,
	) error
}
