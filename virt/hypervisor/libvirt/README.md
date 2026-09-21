# Libvirt domain provider

Package `libvirt` implements Tethux's virtual-machine lifecycle interfaces on
top of an existing libvirt daemon. It supports domain creation and inspection,
start/stop/poweroff, suspend/resume, restart, deletion, serial consoles, SPICE
display discovery, bridged interfaces, and normalized lifecycle events.

- [Go reference](https://pkg.go.dev/github.com/tethux/tethux/virt/hypervisor/libvirt)
- [Domain model](https://pkg.go.dev/github.com/tethux/tethux/virt/domain)
- [CLI guide](../../../cmd/virt/README.md#libvirt-domains)

## Requirements

The package uses the cgo-based `libvirt.org/go/libvirt` binding. Builds need a
C toolchain, `pkg-config`, and libvirt development headers. On Debian/Ubuntu,
install `build-essential pkg-config libvirt-dev`; on Fedora, install
`gcc pkgconf-pkg-config libvirt-devel`.

At runtime, the process needs access to a libvirt daemon and to every host
bridge and disk path referenced by the domain. `qemu:///system` is appropriate
for system-managed VMs and host bridges; `qemu:///session` is useful for
unprivileged desktop VMs. Tethux does not start or configure libvirt.

## Using the provider

Connect to the daemon and always close the provider:

```go
provider, err := libvirt.New("qemu:///system")
if err != nil {
	return err
}
defer provider.Close()
```

Portable `domain.Config` values refer to disks through `storage.Ref`. Use
`domain.Manager` with a storage provider to prepare those references before
calling the libvirt provider. `domain.Manager.Create` performs that preparation
and returns resources that the caller must release after deleting the domain.
The command implementation in
[`cmd/virt/libvirt.go`](../../../cmd/virt/libvirt.go) is a complete example.

The provider lists and mutates only domains marked with Tethux ownership
metadata. Domain IDs are libvirt UUIDs and are the stable identifiers accepted
by lifecycle operations. Cancel the context passed to `Events` or
`OpenConsole` to end the subscription or console stream.

## CLI

The multicall binary exposes the provider as `tethux virt libvirt`:

```bash
tethux virt libvirt --action=create --disk alpine.qcow2 --bridge lab0 --keep
tethux virt libvirt --action=list
tethux virt libvirt --action=inspect --id DOMAIN_UUID
tethux virt libvirt --action=console --id DOMAIN_UUID
tethux virt libvirt --action=view --id DOMAIN_UUID
```

See the [CLI guide](../../../cmd/virt/README.md#libvirt-domains) for all
lifecycle actions and the privileged integration-test workflow.