// Package cleanup supplies bounded contexts for rollback operations.
package cleanup

import (
	"context"
	"time"
)

// NewContext preserves values while replacing cancellation with a rollback deadline.
func NewContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
}
