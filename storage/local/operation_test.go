package local

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tethux/tethux/storage"
)

func TestOperationWaitAndEvents(t *testing.T) {
	op := newOperation(context.Background(), storage.OperationCopy, nil, nil)
	events, err := op.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	op.start()
	op.finish(nil)

	status, err := op.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != storage.OperationCompleted {
		t.Fatalf("state = %q, want completed", status.State)
	}
	if status.StartedAt.IsZero() || status.CompletedAt.IsZero() {
		t.Fatal("expected operation timestamps")
	}

	got := make([]storage.EventType, 0, 2)
	for event := range events {
		got = append(got, event.Type)
	}
	if len(got) != 2 || got[0] != storage.EventOperationStarted || got[1] != storage.EventOperationCompleted {
		t.Fatalf("events = %v, want started and completed", got)
	}
}

func TestOperationFailure(t *testing.T) {
	op := newOperation(context.Background(), storage.OperationCopy, nil, nil)
	op.start()
	op.finish(errors.New("copy failed"))

	status, err := op.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != storage.OperationFailed {
		t.Fatalf("state = %q, want failed", status.State)
	}
	if status.Error != "copy failed" {
		t.Fatalf("error = %q", status.Error)
	}
}

func TestOperationCancel(t *testing.T) {
	op := newOperation(context.Background(), storage.OperationCopy, nil, nil)
	if err := op.Cancel(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	status, err := op.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != storage.OperationCanceled {
		t.Fatalf("state = %q, want canceled", status.State)
	}
}

func TestOperationWaitHonorsContext(t *testing.T) {
	op := newOperation(context.Background(), storage.OperationCopy, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := op.Wait(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestCopyAsyncCompletes(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := storage.Ref{Provider: DefaultName, Key: "source"}
	dst := storage.Ref{Provider: DefaultName, Key: "destination"}
	putErr := provider.Put(context.Background(), src, strings.NewReader("payload"), storage.PutOptions{})
	if putErr != nil {
		t.Fatal(putErr)
	}

	op, err := provider.CopyAsync(context.Background(), src, dst, storage.CopyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := op.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != storage.OperationCompleted {
		t.Fatalf("state = %q, want completed", status.State)
	}
	if _, err := provider.Stat(context.Background(), dst); err != nil {
		t.Fatal(err)
	}
}

func TestCopyAsyncReportsFailure(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := storage.Ref{Provider: DefaultName, Key: "missing"}
	dst := storage.Ref{Provider: DefaultName, Key: "destination"}

	op, err := provider.CopyAsync(context.Background(), src, dst, storage.CopyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := op.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != storage.OperationFailed {
		t.Fatalf("state = %q, want failed", status.State)
	}
	if status.Error == "" {
		t.Fatal("expected operation error")
	}
}

func TestPrepareAsyncReturnsPreparedResult(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	req := storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/data"},
		NodeID:       "router-1",
		ResourceType: storage.ResourceTypeDirectory,
		Create:       true,
	}

	handle, err := provider.PrepareAsync(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	prepareOp, ok := handle.(storage.PrepareOperation)
	if !ok {
		t.Fatal("expected prepare operation capability")
	}

	prepared, err := prepareOp.Prepared(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prepared == nil || prepared.ID == "" {
		t.Fatal("expected prepared result with ID")
	}
	if prepared.Ref != req.Ref || prepared.NodeID != req.NodeID {
		t.Fatalf("unexpected prepared result: %#v", prepared)
	}
}

func TestPrepareAsyncReturnsPreparationError(t *testing.T) {
	provider, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	handle, err := provider.PrepareAsync(context.Background(), storage.PrepareRequest{
		Ref: storage.Ref{Provider: DefaultName, Key: "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepareOp, ok := handle.(storage.PrepareOperation)
	if !ok {
		t.Fatal("expected prepare operation capability")
	}
	_, preparedErr := prepareOp.Prepared(context.Background())
	if preparedErr == nil {
		t.Fatal("expected preparation error")
	}
}

func TestCommitAsyncCompletes(t *testing.T) {
	provider, newErr := New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	prepared, prepareErr := provider.Prepare(context.Background(), storage.PrepareRequest{
		Ref:          storage.Ref{Provider: DefaultName, Key: "nodes/router/disk"},
		ResourceType: storage.ResourceTypeFile,
		Create:       true,
	})
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}

	op, asyncErr := provider.CommitAsync(context.Background(), prepared, storage.CommitOptions{
		Sync: storage.SyncPolicyFull,
	})
	if asyncErr != nil {
		t.Fatal(asyncErr)
	}
	status, waitErr := op.Wait(context.Background())
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if status.State != storage.OperationCompleted {
		t.Fatalf("state = %q, want completed", status.State)
	}
}

func TestCommitAsyncReportsFailure(t *testing.T) {
	provider, newErr := New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	prepared := &storage.Prepared{
		Ref: storage.Ref{Provider: DefaultName, Key: "missing"},
	}

	op, asyncErr := provider.CommitAsync(context.Background(), prepared, storage.CommitOptions{})
	if asyncErr != nil {
		t.Fatal(asyncErr)
	}
	status, waitErr := op.Wait(context.Background())
	if waitErr != nil {
		t.Fatal(waitErr)
	}
	if status.State != storage.OperationFailed {
		t.Fatalf("state = %q, want failed", status.State)
	}
	if status.Error == "" {
		t.Fatal("expected operation error")
	}
}
