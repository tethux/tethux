# tethux

[![Go Reference](https://pkg.go.dev/badge/github.com/tethux/tethux.svg)](https://pkg.go.dev/github.com/tethux/tethux)

tethux is not ready for general use yet. It is an early-stage network emulation
toolkit for building programmable Ethernet topologies across containers, virtual
machines, and physical hosts.

The project includes an Ethernet switch, UDP/TAP/raw/pcap transports, container
and VM providers, a declarative topology runner, and integration tooling. This
repository is a Go and Nix monorepo; detailed commands and examples live in the README nearest each
subsystem.

## Monorepo map

| Path | Purpose | Documentation |
| --- | --- | --- |
| `cmd/` | Public CLI packages and executable entrypoints | [`cmd/README.md`](cmd/README.md) |
| `cmd/bridge/` | Ethernet switch and namespace/container bridge commands | [`cmd/bridge/README.md`](cmd/bridge/README.md) |
| `cmd/topology/` | Declarative TOML topology commands | [`cmd/topology/README.md`](cmd/topology/README.md) |
| `cmd/virt/` | Container and libvirt domain management CLI | [`cmd/virt/README.md`](cmd/virt/README.md) |
| `bridge/` | Public Ethernet switch, transports, and network primitives | [README](bridge/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/bridge) |
| `topology/` | Topology model, TOML decoding, and local container execution | [README](topology/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/topology) |
| `storage/` | Public storage abstractions and local provider | [README](storage/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/storage) |
| `virt/` | Public workload, container, and virtual-machine APIs | [README](virt/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/virt) |
| `virt/hypervisor/libvirt/` | Public libvirt domain provider | [README](virt/hypervisor/libvirt/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/virt/hypervisor/libvirt) |
| `tools/` | Repository CI, archive, and host tooling | [`tools/README.md`](tools/README.md) |
| `tools/ci/` | Unified repository test, archive, and host CLI | [`tools/ci/README.md`](tools/ci/README.md) |
| `dagger/` | Portable CI execution graph exported to OpenTelemetry | [`tools/ci/README.md`](tools/ci/README.md#dagger-and-signoz) |
| `nix/` | Development shells, NixOS test hosts, fixture registry, and CI operations | [`nix/README.md`](nix/README.md) |
| `.woodpecker/` | Ordered NAS and two-laptop CI workflows | [`nix/README.md`](nix/README.md) |

## Current capabilities

- learning Ethernet switch with UDP, TAP, raw-socket, and pcap ports;
- deterministic veth attachment to Linux namespaces and containers;
- TOML topologies with automatic Docker/Podman selection and owned cleanup;
- a common lifecycle API over Docker, Podman, and containerd;
- a libvirt domain provider with lifecycle events, storage preparation, serial
  consoles, SPICE displays, and bridged networking;
- JSON Lines provider tests covering two images and every provider operation;
- provider-managed container links between physical hosts over UDP;
- reproducible NixOS test hosts with a local OCI fixture registry;
- commit-addressed CI reports archived on the NAS;
- Dagger traces and command logs exported to the repository's OpenTelemetry backend;
- byte-exact libpcap-observed tests for every bridge transport backend.

## Quick start

Install the command at the repository's current module version:

```bash
go install github.com/tethux/tethux/cmd/tethux@latest
```

Libraries share the repository's single module version. Add the module, then
import only the packages required by your application:

```bash
go get github.com/tethux/tethux@latest
```

```go
import (
	"github.com/tethux/tethux/bridge"
	"github.com/tethux/tethux/storage"
	"github.com/tethux/tethux/virt"
)
```

For a reproducible install, replace `latest` with a version from the
[repository tags](https://github.com/tethux/tethux/tags). Browse the complete
module on [pkg.go.dev](https://pkg.go.dev/github.com/tethux/tethux).

The multicall command includes the libvirt provider and therefore needs the
libvirt development library (`libvirt-dev` on Debian/Ubuntu,
`libvirt-devel` on Fedora) and a working C toolchain at build time. Applications
that import only `bridge`, `storage`, or the container providers do not need
libvirt.

Releases use semantic `vX.Y.Z` Git tags. Go tooling and pkg.go.dev discover the
public packages from those tags, so documentation and API changes are tagged
together rather than published from an arbitrary branch commit.

Build and run the local container chain from this checkout:

```bash
RUNTIME=docker mise run fixture-registry:start
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/container-chain.toml"
```

The CLI logs progress and prints shell, interface, and ping commands for the
running nodes. Press Ctrl+C to clean up. See the [topology README](topology/README.md)
for bidirectional ping examples, TOML fields, and library usage.

## Architecture

The public `bridge`, `topology`, `storage`, and `virt` packages form the reusable
API. Commands under `cmd` compose those packages, while repository automation and
CI implementation remain private under `internal`. All packages are released
together from the root `github.com/tethux/tethux` Go module.

Enter the development shell and run the normal checks:

```bash
nix develop
mise run check
go run ./cmd/tethux --help
```

The matching `tools/ci task check` workflow is a typed Go declaration of every
format, lint, test, and build step. Mise only selects the pinned tools and calls
that workflow.

Build the primary multicall binary:

```bash
nix build .#tethux
./result/bin/tethux --help
```

For bridge examples, provider testing, cross-host links, test host installation,
recovery, and CI archives, follow the subsystem README from the map above.

## Privileged tests

Bridge and provider integration tests create real containers, veth devices,
namespaces, and UDP listeners. Use the NixOS test hosts or another disposable
lab host. Local privileged integration is never automatic; opt in with
`TETHUX_RUN_INTEGRATION=1` and the Mise tasks documented in
[`nix/README.md`](nix/README.md).
