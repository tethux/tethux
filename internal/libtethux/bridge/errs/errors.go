package errs

import (
	"errors"
	"strings"
)

var (
	// ErrPermissionDenied indicates that an operation requires additional privileges.
	ErrPermissionDenied = errors.New("insufficient privileges: must run as root")
	// ErrPidNotFound indicates that a target process does not exist.
	ErrPidNotFound = errors.New("target process ID not found")
	// ErrLinkExists indicates that a requested network link already exists.
	ErrLinkExists = errors.New("veth interface already exists")
	// ErrLinkNotFound indicates that a requested network link does not exist.
	ErrLinkNotFound = errors.New("network interface not found")
	// ErrNamespaceFailed indicates that a network namespace could not be accessed.
	ErrNamespaceFailed = errors.New("could not access or switch network namespace")
	// ErrNamespaceSwitch indicates that switching network namespaces failed.
	ErrNamespaceSwitch = errors.New("failed to switch network namespace")
	// ErrFailedToCreate indicates that link creation failed.
	ErrFailedToCreate = errors.New("failed to create veth pair")
	// ErrFailedToFindPeer indicates that a veth peer could not be found.
	ErrFailedToFindPeer = errors.New("failed to find peer interface")
	// ErrFailedToSetMTU indicates that an interface MTU could not be configured.
	ErrFailedToSetMTU = errors.New("failed to set MTU")
	// ErrMTUOverflow indicates that an MTU exceeds the supported representation.
	ErrMTUOverflow = errors.New("MTU overflow")
	// ErrSockOverflow indicates that a socket descriptor cannot be represented safely.
	ErrSockOverflow = errors.New("socket descriptor overflows uintptr capacity")
	// ErrPortAlrAttached indicates that a switch already contains the port.
	ErrPortAlrAttached = errors.New("port already attached")
	// ErrPortNotFound indicates that a switch does not contain the requested port.
	ErrPortNotFound = errors.New("port not found")
	// ErrFrameTooShort indicates that input cannot contain an Ethernet header.
	ErrFrameTooShort = errors.New("frame too short")
	// ErrReadTimeout indicates that a port read reached its polling deadline.
	ErrReadTimeout = errors.New("port read timeout")
	// ErrInvalidOptions indicates invalid caller-supplied configuration.
	ErrInvalidOptions = errors.New("invalid options")
	// ErrUnsupportedMode indicates an unsupported interface mode.
	ErrUnsupportedMode = errors.New("unsupported mode")
	// ErrUnknownScheme indicates an unregistered transport scheme.
	ErrUnknownScheme = errors.New("unknown port scheme")
	// ErrPortSetup indicates that a concrete port could not be configured.
	ErrPortSetup = errors.New("failed to set up port")
)

// OpError describes a failed bridge operation and preserves its error category and cause.
type OpError struct {
	Operation string
	Kind      error
	Target    string
	Cause     error
}

// Error returns a human-readable description of the failed operation.
func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	for _, value := range []string{e.Operation, errorString(e.Kind), e.Target, errorString(e.Cause)} {
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

// Wrap creates an operation error with a category and underlying cause.
func Wrap(operation string, kind error, target string, cause error) error {
	return &OpError{Operation: operation, Kind: kind, Target: target, Cause: cause}
}

// New creates an operation error with a category and no underlying cause.
func New(operation string, kind error, target string) error {
	return Wrap(operation, kind, target, nil)
}
