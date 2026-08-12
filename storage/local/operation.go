package local

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tethux/tethux/storage"
)

// operation is the local provider's in-process implementation of a storage
// operation handle.
type operation struct {
	id     storage.OperationID
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu          sync.RWMutex
	status      storage.Operation
	prepared    *storage.Prepared
	resultErr   error
	finished    bool
	nextSubID   uint64
	subscribers map[uint64]chan storage.Event
}

func newOperation(
	ctx context.Context,
	opType storage.OperationType,
	source *storage.Ref,
	target *storage.Ref,
) *operation {
	opCtx, cancel := context.WithCancel(ctx)
	op := &operation{
		id:     storage.OperationID(uuid.NewString()),
		ctx:    opCtx,
		cancel: cancel,
		done:   make(chan struct{}),
		status: storage.Operation{
			Type:   opType,
			State:  storage.OperationPending,
			Source: source,
			Target: target,
		},
		subscribers: make(map[uint64]chan storage.Event),
	}
	op.status.ID = op.id

	go func() {
		<-opCtx.Done()
		op.mu.RLock()
		finished := op.finished
		op.mu.RUnlock()
		if !finished {
			op.finish(opCtx.Err())
		}
	}()

	return op
}

func (o *operation) ID() storage.OperationID {
	return o.id
}

// context returns the operation-owned context used by its worker.
func (o *operation) context() context.Context {
	return o.ctx
}

func (o *operation) setPrepared(
	prepared *storage.Prepared,
	err error,
) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.prepared = prepared
	o.resultErr = err
}

func (o *operation) Prepared(ctx context.Context) (*storage.Prepared, error) {
	if _, err := o.Wait(ctx); err != nil {
		return nil, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.resultErr != nil {
		return nil, o.resultErr
	}
	return o.prepared, nil
}

func (o *operation) Status(ctx context.Context) (storage.Operation, error) {
	if err := ctx.Err(); err != nil {
		return storage.Operation{}, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.status, nil
}

func (o *operation) Wait(ctx context.Context) (storage.Operation, error) {
	select {
	case <-o.done:
		return o.Status(context.Background())
	case <-ctx.Done():
		return storage.Operation{}, ctx.Err()
	}
}

func (o *operation) Cancel(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.cancel()
	return nil
}

func (o *operation) Events(ctx context.Context) (<-chan storage.Event, error) {
	out := make(chan storage.Event, 16)

	o.mu.Lock()
	subID := o.nextSubID
	o.nextSubID++
	subscriber := make(chan storage.Event, 16)
	if o.finished {
		close(subscriber)
	} else {
		o.subscribers[subID] = subscriber
	}
	o.mu.Unlock()

	go func() {
		defer close(out)
		defer func() {
			o.mu.Lock()
			delete(o.subscribers, subID)
			o.mu.Unlock()
		}()

		for {
			event, ok := <-subscriber
			if !ok {
				return
			}
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (o *operation) start() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished || o.status.State != storage.OperationPending {
		return
	}
	o.status.State = storage.OperationRunning
	o.status.StartedAt = time.Now()
	o.broadcastLocked(&storage.Event{
		Type:        storage.EventOperationStarted,
		Time:        time.Now(),
		OperationID: o.id,
	})
}

func (o *operation) finish(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished {
		return
	}

	o.finished = true
	o.status.CompletedAt = time.Now()
	switch {
	case err == nil:
		o.status.State = storage.OperationCompleted
	case errors.Is(err, context.Canceled):
		o.status.State = storage.OperationCanceled
		o.status.Error = err.Error()
	default:
		o.status.State = storage.OperationFailed
		o.status.Error = err.Error()
	}

	eventType := storage.EventOperationCompleted
	switch o.status.State {
	case storage.OperationCanceled:
		eventType = storage.EventOperationCanceled
	case storage.OperationFailed:
		eventType = storage.EventOperationFailed
	}
	o.broadcastLocked(&storage.Event{
		Type:        eventType,
		Time:        time.Now(),
		OperationID: o.id,
		Error:       o.status.Error,
	})
	close(o.done)
	for id, subscriber := range o.subscribers {
		close(subscriber)
		delete(o.subscribers, id)
	}
}

func (o *operation) broadcastLocked(event *storage.Event) {
	for _, subscriber := range o.subscribers {
		subscriber <- *event
	}
}
