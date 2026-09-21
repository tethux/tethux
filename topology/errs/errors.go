package errs

import (
	"errors"
	"strings"
)

var (
	// ErrStart indicates a topology runtime could not start.
	ErrStart = errors.New("start topology")
	// ErrCleanup indicates a topology runtime could not release its resources.
	ErrCleanup = errors.New("clean up topology")
	// ErrUnsupported indicates a runtime cannot realize a topology resource.
	ErrUnsupported = errors.New("unsupported topology resource")
	// ErrDecode indicates an unreadable or invalid topology document.
	ErrDecode = errors.New("decode topology")
	// ErrInvalidTopology indicates an invalid topology definition.
	ErrInvalidTopology = errors.New("invalid topology")
	// ErrInvalidNode indicates an invalid topology node.
	ErrInvalidNode = errors.New("invalid topology node")
	// ErrInvalidPort indicates an invalid topology port.
	ErrInvalidPort = errors.New("invalid topology port")
	// ErrInvalidLink indicates an invalid topology link.
	ErrInvalidLink = errors.New("invalid topology link")
	// ErrInvalidView indicates an invalid topology view.
	ErrInvalidView = errors.New("invalid topology view")
	// ErrDuplicateID indicates that an identifier is not unique within a topology.
	ErrDuplicateID = errors.New("duplicate topology id")
	// ErrNotFound indicates that a referenced topology resource does not exist.
	ErrNotFound = errors.New("topology resource not found")
)

// OpError describes a failed topology operation and preserves its category and cause.
type OpError struct {
	Operation string
	Kind      error
	Target    string
	Cause     error
}

// Error returns a human-readable description of the failed operation.
func (e *OpError) Error() string {
	parts := make([]string, 0, 4)

	for _, value := range []string{
		e.Operation,
		errorString(e.Kind),
		e.Target,
		errorString(e.Cause),
	} {
		if value != "" {
			parts = append(parts, value)
		}
	}

	return strings.Join(parts, ": ")
}

// Unwrap returns the stable category and optional underlying cause.
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

// Wrap creates a topology operation error with a stable category and cause.
func Wrap(operation string, kind error, target string, cause error) error {
	return &OpError{
		Operation: operation,
		Kind:      kind,
		Target:    target,
		Cause:     cause,
	}
}

// New creates a topology operation error with no underlying cause.
func New(operation string, kind error, target string) error {
	return Wrap(operation, kind, target, nil)
}
