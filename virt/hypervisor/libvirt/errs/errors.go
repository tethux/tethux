package errs

import (
	"errors"
	"strings"
)

var (
	// Operations.
	ErrConnect   = errors.New("failed to connect to libvirt")
	ErrCreate    = errors.New("failed to create libvirt domain")
	ErrInspect   = errors.New("failed to inspect libvirt domain")
	ErrLifecycle = errors.New("failed to change libvirt domain lifecycle")
	ErrDelete    = errors.New("failed to delete libvirt domain")
	ErrList      = errors.New("failed to list libvirt domains")
	ErrXML       = errors.New("failed to build libvirt domain XML")
	ErrMetadata  = errors.New("failed to access libvirt domain metadata")
	ErrEvents    = errors.New("failed to monitor libvirt domain events")
	ErrConsole   = errors.New("failed to open libvirt domain console")

	// Configuration.
	ErrConfig        = errors.New("invalid libvirt domain configuration")
	ErrNilConfig     = errors.New("nil domain configuration")
	ErrEmptyName     = errors.New("domain name is empty")
	ErrInvalidUUID   = errors.New("invalid domain UUID")
	ErrInvalidCPU    = errors.New("invalid CPU count")
	ErrInvalidMemory = errors.New("invalid memory size")
	ErrFirmware      = errors.New("unsupported firmware")
	ErrBootDevice    = errors.New("unsupported boot device")

	// Disk configuration.
	ErrDisk       = errors.New("invalid disk configuration")
	ErrDiskDevice = errors.New("unsupported disk device")
	ErrDiskSource = errors.New("disk source is empty")
	ErrDiskFormat = errors.New("unsupported disk format")
	ErrDiskBus    = errors.New("unsupported disk bus")
	ErrDiskTarget = errors.New("invalid disk target")

	// Network configuration.
	ErrInterface       = errors.New("invalid network interface configuration")
	ErrInterfaceBridge = errors.New("interface bridge is empty")
	ErrInterfaceModel  = errors.New("unsupported interface model")
	ErrInterfaceMAC    = errors.New("invalid interface MAC address")

	ErrNotManaged = errors.New("libvirt domain is not managed by tethux")
)

// OpError records a stable category, target, and optional libvirt cause.
type OpError struct {
	Provider string
	Kind     error
	Target   string
	Cause    error
}

func (e *OpError) Error() string {
	parts := make([]string, 0, 4)

	for _, value := range []string{
		e.Provider,
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

func errorString(value error) string {
	if value == nil {
		return ""
	}

	return value.Error()
}

// New constructs a categorized libvirt error without an underlying cause.
func New(kind error, target string) error {
	return &OpError{
		Provider: "libvirt",
		Kind:     kind,
		Target:   target,
	}
}

// Wrap constructs a categorized libvirt error that preserves cause.
func Wrap(kind error, target string, cause error) error {
	return &OpError{
		Provider: "libvirt",
		Kind:     kind,
		Target:   target,
		Cause:    cause,
	}
}
