package storage

import (
	"context"
	"errors"
)

// ReleaseResources releases leases and retains only failures for retry.
// The caller must stop workloads using these resources before releasing them.
func ReleaseResources(ctx context.Context, manager Manager, resources []*Prepared) ([]*Prepared, error) {
	remaining := resources[:0]
	var failures []error
	for _, resource := range resources {
		releaseErr := manager.Release(ctx, resource)
		if releaseErr != nil {
			remaining = append(remaining, resource)
			failures = append(failures, releaseErr)
		}
	}
	clear(resources[len(remaining):])
	return remaining, errors.Join(failures...)
}
