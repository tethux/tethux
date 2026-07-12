# tethux

tethux is an experimental, autoscalable network-emulation toolkit. It combines
an Ethernet switch, UDP/TAP/raw/pcap transports, container and VM provider
interfaces, and integration tooling for building GNS3/uBridge-style topologies.

This repository is a Go and Nix monorepo. The root README is intentionally an
overview; each subsystem owns its commands, examples, and operational notes in
the closest README.

## Monorepo map

| Path | Purpose | Documentation |
| --- | --- | --- |
| `cmd/` | Public CLI packages and executable entrypoints | [`cmd/README.md`](cmd/README.md) |
| `cmd/bridge/` | Ethernet switch and namespace/container bridge commands | [`cmd/bridge/README.md`](cmd/bridge/README.md) |
| `cmd/virt/` | Docker, Podman, and containerd providers and integration CLI | [`cmd/virt/README.md`](cmd/virt/README.md) |
| `internal/libtethux/` | Switch, transport, bridge, and provider libraries | package Go documentation |
| `scripts/` | Standalone topology demonstrations and load runners | [`scripts/README.md`](scripts/README.md) |
| `nix/` | Development shells, NixOS canaries, fixture registry, and CI operations | [`nix/README.md`](nix/README.md) |
| `.woodpecker/` | Ordered NAS and two-laptop CI workflows | [`nix/README.md`](nix/README.md#woodpecker-topology) |

## Current capabilities

- learning Ethernet switch with UDP, TAP, raw-socket, and pcap ports;
- deterministic veth attachment to Linux namespaces and containers;
- a common lifecycle API over Docker, Podman, and containerd;
- JSON Lines provider tests covering two images and every provider operation;
- provider-managed container links between physical hosts over UDP;
- reproducible NixOS canaries with a local OCI fixture registry;
- commit-addressed CI reports archived on the NAS.
- byte-exact libpcap-observed tests for every bridge transport backend.

## Quick start

Enter the development shell and run the normal checks:

```bash
nix develop
go test ./...
golangci-lint run -c .golangci.yml
go run ./cmd/tethux --help
```

Build the primary multicall binary:

```bash
nix build .#tethux
./result/bin/tethux --help
```

For bridge examples, provider testing, cross-host links, canary installation,
recovery, and CI archives, follow the subsystem README from the map above.

## Project status

tethux is pre-release research software. Privileged bridge and provider tests
create real containers, veth devices, namespaces, and UDP listeners; use the
NixOS canaries or another disposable lab host for integration work.
Local privileged integration is never automatic: opt in with
`TETHUX_RUN_INTEGRATION=1` and the Mise tasks documented in
[`nix/README.md`](nix/README.md).
