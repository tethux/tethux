package errs

import "errors"

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
)
