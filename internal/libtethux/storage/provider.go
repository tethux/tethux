package storage

import (
	"context"
	"io"
	"time"
)

type ObjectType string

const (
	ObjectUnknown ObjectType = "unknown"
	ObjectFile    ObjectType = "file"
	ObjectDir     ObjectType = "directory"
)

type ObjectInfo struct {
	Ref     Ref
	Type    ObjectType
	Size    int64
	ModTime time.Time
}

type PutOptions struct {
	Mode uint32
}

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
