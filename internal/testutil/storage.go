// Package testutil supplies resource accounting for lifecycle failure tests.
package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/storage/local"
)

var (
	ErrPrepare = errors.New("injected preparation failure")
	ErrRelease = errors.New("injected release failure")
	ErrCreate  = errors.New("injected creation failure")
)

// Storage delegates to real local storage while injecting selected failures.
type Storage struct {
	storage.Manager
	FailPrepare     int
	InvalidLocation int
	FailRelease     int
	RequireRollback bool
	Cancel          context.CancelFunc
	Acquired        []*storage.Prepared
	Released        map[storage.PreparedID]bool
	Calls           int
}

// NewStorage creates a local store with two durable files.
func NewStorage(t *testing.T) *Storage {
	t.Helper()
	provider, newErr := local.New(t.TempDir())
	if newErr != nil {
		t.Fatal(newErr)
	}
	for _, key := range []storage.Key{"one", "two"} {
		putErr := provider.Put(t.Context(), storage.Ref{Provider: provider.Name(), Key: key}, strings.NewReader("durable fixture"), storage.PutOptions{})
		if putErr != nil {
			t.Fatal(putErr)
		}
	}
	return &Storage{Manager: provider, Released: make(map[storage.PreparedID]bool)}
}

//nolint:gocritic // The storage.Manager contract passes PrepareRequest by value.
func (s *Storage) Prepare(ctx context.Context, req storage.PrepareRequest) (*storage.Prepared, error) {
	s.Calls++
	if s.Calls == s.FailPrepare {
		if s.Cancel != nil {
			s.Cancel()
		}
		return nil, ErrPrepare
	}
	prepared, prepareErr := s.Manager.Prepare(ctx, req)
	if prepareErr != nil {
		return nil, prepareErr
	}
	s.Acquired = append(s.Acquired, prepared)
	if s.Calls == s.InvalidLocation {
		prepared.Location.Kind = storage.LocationURI
	}
	return prepared, nil
}

func (s *Storage) Release(ctx context.Context, prepared *storage.Prepared) error {
	if s.RequireRollback {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, bounded := ctx.Deadline(); !bounded {
			return errors.New("unbounded rollback")
		}
	}
	if s.FailRelease > 0 && len(s.Acquired) >= s.FailRelease && prepared.ID == s.Acquired[s.FailRelease-1].ID {
		return ErrRelease
	}
	if s.Released[prepared.ID] {
		return errors.New("lease released twice")
	}
	releaseErr := s.Manager.Release(ctx, prepared)
	if releaseErr == nil {
		s.Released[prepared.ID] = true
	}
	return releaseErr
}

// CheckOwnership requires each acquisition to be either released or retained once.
func (s *Storage) CheckOwnership(t *testing.T, retained []*storage.Prepared) {
	t.Helper()
	tracked := make(map[storage.PreparedID]int)
	for _, resource := range retained {
		tracked[resource.ID]++
	}
	for _, resource := range s.Acquired {
		count := tracked[resource.ID]
		if s.Released[resource.ID] {
			count++
		}
		if count != 1 {
			t.Errorf("lease %s has %d owners", resource.ID, count)
		}
	}
	for _, resource := range retained {
		found := false
		for _, acquired := range s.Acquired {
			if acquired.ID == resource.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("unknown retained lease %s", resource.ID)
		}
	}
}
