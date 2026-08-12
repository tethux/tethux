package storage

import (
	"context"
	"time"
)

// EventType identifies a storage lifecycle event.
type EventType string

const (
	EventOperationStarted   EventType = "operation.started"
	EventOperationProgress  EventType = "operation.progress"
	EventOperationCompleted EventType = "operation.completed"
	EventOperationFailed    EventType = "operation.failed"
	EventOperationCanceled  EventType = "operation.canceled"

	EventPrepared  EventType = "storage.prepared"
	EventDirty     EventType = "storage.dirty"
	EventCommitted EventType = "storage.committed"
	EventReleased  EventType = "storage.released"
)

// Event describes a change in storage or an asynchronous storage operation.
type Event struct {
	Type EventType
	Time time.Time

	OperationID OperationID

	Ref *Ref

	Progress *Progress

	Error string
}

// EventSource exposes storage events.
//
// Implementations should close the returned channel when ctx is canceled or
// the source can no longer produce events.
type EventSource interface {
	Events(ctx context.Context) (<-chan Event, error)
}
