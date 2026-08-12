// Package errs defines stable error categories for storage operations.
package errs

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidOptions indicates invalid caller-supplied storage options.
	ErrInvalidOptions = errors.New("invalid storage options")
	// ErrInvalidRef indicates an invalid storage reference or path.
	ErrInvalidRef = errors.New("invalid storage reference")
	// ErrProviderMismatch indicates a reference for another provider.
	ErrProviderMismatch = errors.New("storage provider mismatch")
	// ErrNotFound indicates that a requested storage object does not exist.
	ErrNotFound = errors.New("storage object not found")
	// ErrUnsupportedMode indicates an unsupported preparation mode.
	ErrUnsupportedMode = errors.New("unsupported storage preparation mode")
	// ErrInvalidResourceType indicates an invalid resource type or shape.
	ErrInvalidResourceType = errors.New("invalid storage resource type")
	// ErrStat indicates that inspecting a storage object failed.
	ErrStat = errors.New("failed to stat storage object")
	// ErrOpen indicates that opening a storage object failed.
	ErrOpen = errors.New("failed to open storage object")
	// ErrPut indicates that storing an object failed.
	ErrPut = errors.New("failed to put storage object")
	// ErrDelete indicates that deleting an object failed.
	ErrDelete = errors.New("failed to delete storage object")
	// ErrList indicates that listing objects failed.
	ErrList = errors.New("failed to list storage objects")
	// ErrPrepare indicates that preparing storage failed.
	ErrPrepare = errors.New("failed to prepare storage")
	// ErrCreate indicates that creating a storage object failed.
	ErrCreate = errors.New("failed to create storage object")
	// ErrRelease indicates that releasing prepared storage failed.
	ErrRelease = errors.New("failed to release storage")
	// ErrCopy indicates that copying storage failed.
	ErrCopy = errors.New("failed to copy storage")
	// ErrMove indicates that moving storage failed.
	ErrMove = errors.New("failed to move storage")
	// ErrAlreadyExists indicates that a storage object already exists.
	ErrAlreadyExists error = errors.New("storage object already exists")
	// ErrSourceEqualsDestination indicates that a source and destination are the same.
	ErrSourceEqualsDestination = errors.New("source and destination are the same")
	// ErrChecksum indicates that calculating an object checksum failed.
	ErrChecksum = errors.New("failed to calculate storage checksum")
)

// OpError describes a failed storage operation while preserving its category
// and underlying cause.
type OpError struct {
	Provider string
	Kind     error
	Target   string
	Cause    error
}

// Error returns a human-readable description of the failed storage operation.
func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	for _, value := range []string{e.Provider, errorString(e.Kind), e.Target, errorString(e.Cause)} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ": ")
}

// Unwrap returns the error category and optional underlying cause.
func (e *OpError) Unwrap() []error {
	result := make([]error, 0, 2)
	if e.Kind != nil {
		result = append(result, e.Kind)
	}
	if e.Cause != nil {
		result = append(result, e.Cause)
	}
	return result
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Wrap creates a storage operation error with a category and optional cause.
func Wrap(provider string, kind error, target string, cause error) error {
	return &OpError{Provider: provider, Kind: kind, Target: target, Cause: cause}
}

// New creates a storage operation error with no underlying cause.
func New(provider string, kind error, target string) error {
	return Wrap(provider, kind, target, nil)
}
