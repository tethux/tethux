package storage

import (
	"context"
	"io"
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

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Ref     Ref
	Type    ObjectType
	Size    int64
	ModTime time.Time
}

// PutOptions controls how a provider stores an object.
type PutOptions struct {
	Mode uint32
}

// Provider supplies object-oriented storage operations.
type Provider interface {
	Name() ProviderName

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
}
