package errs

import (
	"errors"
	"strings"
)

var (
	ErrPermissionDenied = errors.New("insufficient privileges: must run as root")
	ErrPidNotFound      = errors.New("target process ID not found")
	ErrLinkExists       = errors.New("veth interface already exists")
	ErrLinkNotFound     = errors.New("network interface not found")
	ErrNamespaceFailed  = errors.New("could not access or switch network namespace")
	ErrNamespaceSwitch  = errors.New("failed to switch network namespace")
	ErrFailedToCreate   = errors.New("failed to create veth pair")
	ErrFailedToFindPeer = errors.New("failed to find peer interface")
	ErrFailedToSetMTU   = errors.New("failed to set MTU")
	ErrMTUOverflow      = errors.New("MTU overflow")
	ErrSockOverflow     = errors.New("socket descriptor overflows uintptr capacity")
	ErrPortAlrAttached  = errors.New("port already attached")
	ErrPortNotFound     = errors.New("port not found")
	ErrFrameTooShort    = errors.New("frame too short")
	ErrReadTimeout      = errors.New("port read timeout")
	ErrInvalidOptions   = errors.New("invalid options")
	ErrUnsupportedMode  = errors.New("unsupported mode")
	ErrUnknownScheme    = errors.New("unknown port scheme")
	ErrPortSetup        = errors.New("failed to set up port")
)

type OpError struct {
	Operation string
	Kind      error
	Target    string
	Cause     error
}

func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	for _, value := range []string{e.Operation, errorString(e.Kind), e.Target, errorString(e.Cause)} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, ": ")
}

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
func Wrap(operation string, kind error, target string, cause error) error {
	return &OpError{Operation: operation, Kind: kind, Target: target, Cause: cause}
}
func New(operation string, kind error, target string) error {
	return Wrap(operation, kind, target, nil)
}
