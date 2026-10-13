// Package errs defines categorized endpoint errors.
package errs

import (
	"errors"
	"strings"
)

var (
	ErrConfig = errors.New("invalid endpoint configuration")
	ErrNotFound = errors.New("endpoint not found")
	ErrDuplicate = errors.New("endpoint already exists")
	ErrState = errors.New("endpoint is not running")
	ErrNetwork = errors.New("endpoint network failure")
	ErrProbe = errors.New("ICMP probe failed")
	ErrUnsupported = errors.New("unsupported endpoint operation")
)

// Operation preserves the category, endpoint, and underlying cause.
type Operation struct {
	Op string
	Kind error
	ID string
	Cause error
}

func (e *Operation) Error() string {
	parts := []string{e.Op, e.Kind.Error(), e.ID}
	if e.Cause != nil { parts = append(parts, e.Cause.Error()) }
	return strings.Join(parts, ": ")
}

func (e *Operation) Unwrap() []error { return []error{e.Kind, e.Cause} }

func New(op string, kind error, id string) error { return Wrap(op, kind, id, nil) }
func Wrap(op string, kind error, id string, cause error) error {
	return &Operation{Op:op, Kind:kind, ID:id, Cause:cause}
}
