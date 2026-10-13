package cleanup

import (
	"context"
	"testing"
	"time"
)

func TestNewContextSurvivesCanceledParent(t *testing.T) {
	parent, stop := context.WithCancel(t.Context())
	stop()
	ctx, cancel := NewContext(parent)
	defer cancel()
	if ctx.Err() != nil {
		t.Fatal("rollback inherited cancellation")
	}
	deadline, bounded := ctx.Deadline()
	if !bounded || time.Until(deadline) > 30*time.Second {
		t.Fatal("rollback is unbounded")
	}
	cancel()
	if ctx.Err() == nil {
		t.Fatal("rollback cancellation did not release context")
	}
}
