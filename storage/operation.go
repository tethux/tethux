package storage

import (
	"context"
	"time"
)

// OperationID uniquely identifies a storage operation.
type OperationID string

// OperationType identifies the work performed by a storage operation.
type OperationType string

const (
	OperationPrepare OperationType = "prepare"
	OperationCommit  OperationType = "commit"
	OperationCopy    OperationType = "copy"
	OperationMove    OperationType = "move"
	OperationDelete  OperationType = "delete"
	OperationImport  OperationType = "import"
	OperationExport  OperationType = "export"
	OperationSync    OperationType = "sync"
)

// OperationState describes the lifecycle of asynchronous storage work.
type OperationState string

const (
	OperationPending   OperationState = "pending"
	OperationRunning   OperationState = "running"
	OperationCompleted OperationState = "completed"
	OperationFailed    OperationState = "failed"
	OperationCanceled  OperationState = "canceled"
)

// Progress describes progress for a storage operation.
//
// TotalBytes may be zero when the total size is not known.
type Progress struct {
	Bytes      int64
	TotalBytes int64
}

// Operation describes asynchronous storage work.
type Operation struct {
	ID   OperationID
	Type OperationType

	State OperationState

	Source *Ref
	Target *Ref

	Progress Progress

	StartedAt   time.Time
	CompletedAt time.Time

	Error string
}

// OperationHandle provides control and observation for asynchronous storage
// work.
type OperationHandle interface {
	ID() OperationID

	Status(ctx context.Context) (Operation, error)

	Wait(ctx context.Context) (Operation, error)

	Cancel(ctx context.Context) error

	Events(ctx context.Context) (<-chan Event, error)
}

// PrepareOperation exposes the result of an asynchronous preparation.
type PrepareOperation interface {
	OperationHandle

	Prepared(ctx context.Context) (*Prepared, error)
}
