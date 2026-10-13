// Package errs defines categorized endpoint errors.
package errs

import (
	"errors"
	"strings"
)

var (
	// ErrConfig identifies invalid endpoint configuration.
	ErrConfig = errors.New("invalid endpoint configuration")
	// ErrNotFound identifies an unknown endpoint.
	ErrNotFound = errors.New("endpoint not found")
	// ErrDuplicate identifies an existing endpoint identity.
	ErrDuplicate = errors.New("endpoint already exists")
	// ErrState identifies a stopped endpoint.
	ErrState = errors.New("endpoint is not running")
	// ErrNetwork identifies a frame transport or native packet failure.
	ErrNetwork = errors.New("endpoint network failure")
	// ErrUnsupported identifies an unavailable endpoint operation.
	ErrUnsupported = errors.New("unsupported endpoint operation")
)

// Operation preserves the category, endpoint, and underlying cause.
type Operation struct {
	Op    string
	Kind  error
	ID    string
	Cause error
}

// Error formats the operation diagnostic.
func (e *Operation) Error() string {
	parts := []string{e.Op, e.Kind.Error(), e.ID}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
	}
	return strings.Join(parts, ": ")
}

// Unwrap exposes the category and cause.
func (e *Operation) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Cause}
}

// New constructs a categorized endpoint error.
func New(op string, kind error, id string) error { return Wrap(op, kind, id, nil) }

// Wrap preserves an underlying endpoint error.
func Wrap(op string, kind error, id string, cause error) error {
	return &Operation{Op: op, Kind: kind, ID: id, Cause: cause}
}
