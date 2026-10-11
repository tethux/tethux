package errs

import (
	"errors"
	"strings"
)

var (
	// ErrNotImplemented identifies an unfinished provider operation.
	ErrNotImplemented = errors.New("provider operation is not implemented")
	// ErrConfig identifies invalid router configuration.
	ErrConfig = errors.New("invalid router configuration")
	// ErrUnsupported identifies configuration unavailable through the bindings.
	ErrUnsupported = errors.New("unsupported router configuration")
	// ErrDuplicate identifies an existing router identity.
	ErrDuplicate = errors.New("router identity is already in use")
	// ErrRuntime identifies a missing native runtime.
	ErrRuntime = errors.New("native runtime is required")
	// ErrCreate identifies router creation failure.
	ErrCreate = errors.New("create router")
	// ErrCleanup identifies resource cleanup failure.
	ErrCleanup = errors.New("clean up router")
	// ErrStorage identifies state preparation or release failure.
	ErrStorage = errors.New("router state storage")
	// ErrNotFound identifies an unknown managed router.
	ErrNotFound = errors.New("router not found")
	// ErrLifecycle identifies a failed native lifecycle operation.
	ErrLifecycle = errors.New("router lifecycle")
)

// OpError describes a provider failure and preserves its category and cause.
type OpError struct {
	Operation string
	Kind      error
	Target    string
	Cause     error
}

// Error returns a human-readable diagnostic.
func (e *OpError) Error() string {
	parts := []string{e.Operation, e.Kind.Error()}
	if e.Target != "" {
		parts = append(parts, e.Target)
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	return strings.Join(parts, ": ")
}

// Unwrap exposes the category and optional underlying cause.
func (e *OpError) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Cause}
}

// New constructs a categorized provider error.
func New(operation string, kind error, target string) error {
	return Wrap(operation, kind, target, nil)
}

// Wrap constructs a categorized provider error preserving cause.
func Wrap(operation string, kind error, target string, cause error) error {
	return &OpError{Operation: operation, Kind: kind, Target: target, Cause: cause}
}
