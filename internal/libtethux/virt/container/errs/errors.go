package errs

import (
	"errors"
	"strings"
)

var (
	ErrFailedToCreateClient        = errors.New("failed to create client for socket")
	ErrOverrideSocketNotAccessible = errors.New("override socket not accessible")
	ErrNoSocketFound               = errors.New("no accessible socket found")
	ErrNotASocket                  = errors.New("not a socket")
	ErrInvalidConfig               = errors.New("invalid container configuration")
)

var (
	ErrFailedToStartContainer   = errors.New("failed to start container")
	ErrFailedToDeleteContainer  = errors.New("failed to delete container")
	ErrFailedToInspectContainer = errors.New("failed to inspect container")
	ErrFailedToCreateContainer  = errors.New("failed to create container")
	ErrFailedToPullImage        = errors.New("failed to pull image")
	ErrFailedToCreateExec       = errors.New("failed to create exec")
	ErrFailedToAttachExec       = errors.New("failed to attach exec")
	ErrFailedToStdCopy          = errors.New("failed to copy process output")
	ErrFailedToLogs             = errors.New("failed to read container logs")
	ErrFailedToStopContainer    = errors.New("failed to stop container")
	ErrFailedToResumeContainer  = errors.New("failed to resume container")
	ErrFailedToSuspendContainer = errors.New("failed to suspend container")
	ErrFailedToReadState        = errors.New("failed to read container state")
	ErrFailedToListContainers   = errors.New("failed to list containers")
	ErrFailedToPrepareIO        = errors.New("failed to prepare container IO")
	ErrExecFailed               = errors.New("container exec failed")
)

// Deprecated aliases retained for callers compiled against the original names.
var (
	ErrFailedToCreateClent = ErrFailedToCreateClient
	ErrNoSockerFound       = ErrNoSocketFound
)

// OpError preserves both the stable container error category and the runtime's
// underlying error while adding provider and target context.
type OpError struct {
	Provider string
	Kind     error
	Target   string
	Cause    error
}

func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	if e.Provider != "" {
		parts = append(parts, e.Provider)
	}
	if e.Kind != nil {
		parts = append(parts, e.Kind.Error())
	}
	if e.Target != "" {
		parts = append(parts, e.Target)
	}
	if e.Cause != nil {
		parts = append(parts, e.Cause.Error())
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

func Wrap(provider string, kind error, target string, cause error) error {
	return &OpError{Provider: provider, Kind: kind, Target: target, Cause: cause}
}

func New(provider string, kind error, target string) error {
	return Wrap(provider, kind, target, nil)
}
