# tethux

tethux is not ready for general use yet. It is an early-stage network emulation
toolkit for building programmable Ethernet topologies across containers, virtual
machines, and physical hosts.

The project includes an Ethernet switch, UDP/TAP/raw/pcap transports, container
and VM providers, and integration tooling. This repository is a Go and Nix
monorepo; detailed commands and examples live in the README nearest each
subsystem.

## Monorepo map

| Path | Purpose | Documentation |
| --- | --- | --- |
| `cmd/` | Public CLI packages and executable entrypoints | [`cmd/README.md`](cmd/README.md) |
| `cmd/bridge/` | Ethernet switch and namespace/container bridge commands | [`cmd/bridge/README.md`](cmd/bridge/README.md) |
| `cmd/virt/` | Docker, Podman, and containerd providers and integration CLI | [`cmd/virt/README.md`](cmd/virt/README.md) |
| `bridge/` | Public Ethernet switch, transports, and network primitives | [README](bridge/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/bridge) |
| `storage/` | Public storage abstractions and local provider | [README](storage/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/storage) |
| `virt/` | Public virtualization APIs and container providers | [README](virt/README.md) · [Go reference](https://pkg.go.dev/github.com/tethux/tethux/virt) |
| `tools/` | Repository CI, archive, and host tooling | [`tools/README.md`](tools/README.md) |
| `tools/ci/` | Unified repository test, archive, and host CLI | [`tools/ci/README.md`](tools/ci/README.md) |
| `dagger/` | Portable CI execution graph exported to OpenTelemetry | [`tools/ci/README.md`](tools/ci/README.md#dagger-and-signoz) |
| `nix/` | Development shells, NixOS test hosts, fixture registry, and CI operations | [`nix/README.md`](nix/README.md) |
| `.woodpecker/` | Ordered NAS and two-laptop CI workflows | [`nix/README.md`](nix/README.md#woodpecker-topology) |

## Current capabilities

- learning Ethernet switch with UDP, TAP, raw-socket, and pcap ports;
- deterministic veth attachment to Linux namespaces and containers;
- a common lifecycle API over Docker, Podman, and containerd;
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

The current release is `v0.0.4`; replace `latest` with `v0.0.4` for a
reproducible install. Browse the complete module on
[pkg.go.dev](https://pkg.go.dev/github.com/tethux/tethux).

## Architecture

The public `bridge`, `storage`, and `virt` packages form the reusable API.
Commands under `cmd` compose those packages, while repository automation and
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
