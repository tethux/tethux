package errs

import (
	"errors"
	"strings"
)

var (
	ErrConnect   = errors.New("failed to connect to libvirt")
	ErrCreate    = errors.New("failed to create libvirt domain")
	ErrInspect   = errors.New("failed to inspect libvirt domain")
	ErrLifecycle = errors.New("failed to change libvirt domain lifecycle")
	ErrDelete    = errors.New("failed to delete libvirt domain")
	ErrList      = errors.New("failed to list libvirt domains")
	ErrConfig    = errors.New("invalid libvirt domain configuration")
	ErrXML       = errors.New("failed to build libvirt domain XML")
)

type OpError struct {
	Provider string
	Kind     error
	Target   string
	Cause    error
}

func (e *OpError) Error() string {
	parts := make([]string, 0, 4)
	for _, value := range []string{e.Provider, errorString(e.Kind), e.Target, errorString(e.Cause)} {
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

func New(kind error, target string) error {
	return &OpError{Provider: "libvirt", Kind: kind, Target: target}
}

func Wrap(kind error, target string, cause error) error {
	return &OpError{Provider: "libvirt", Kind: kind, Target: target, Cause: cause}
}
