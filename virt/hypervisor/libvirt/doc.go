// Package libvirt implements Tethux's virtual-machine domain interfaces using
// an externally managed libvirt daemon.
//
// New opens a libvirt connection URI such as qemu:///system or
// qemu:///session. Provider implements domain.Provider, domain.ConsoleProvider,
// and virt.EventSource. It can define, inspect, list, start, stop, power off,
// suspend, resume, restart, and delete domains, and it exposes serial consoles
// and normalized lifecycle events.
//
// Domain disks must be resolved before CreateDomain is called. Most callers
// should use domain.Manager with a storage.Manager to convert portable
// domain.Config disk references into a domain.RuntimeConfig. The caller owns
// the prepared storage resources and must release them after deleting the
// domain.
//
// The provider operates only on domains carrying Tethux ownership metadata.
// It connects to libvirt but does not install, start, or configure the daemon,
// storage pools, host bridges, or guest images.
//
// This package uses libvirt.org/go/libvirt and therefore requires cgo, a C
// compiler, pkg-config, and the libvirt development headers at build time.
package libvirt
