# Topology commands

`cmd/topology` defines the `tethux topology` command tree. It reads `.lua` files
through `topology/lua` and other files through `topology/toml`, then uses
`topology/local` for provider selection and execution.

```bash
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/container-chain.toml"
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/ring.lua"
```

`run` logs lifecycle progress, nodes, discovered IPv4 addresses, and links with
slog as one JSON object per line on stderr. Docker is tried before Podman by default.
Use `--provider docker`, `--provider podman`, or `--provider containerd` to
select one explicitly and `--base-port` to change the
loopback UDP range. Ctrl+C cleans up this run's resources.

Lua scripts must return their topology builder. They execute with the command's
privileges and have access to standard Lua libraries, so only run trusted scripts.
The local runner supports container, libvirt domain, Dynamips, and Shitnet
nodes with Ethernet links. Container and domain links require root privileges.

Router-only topologies run without root privileges:

```bash
TETHUX_DYNAMIPS_IMAGE=/absolute/path/to/c7200.bin ./tethux topology run topology/examples/dynamips-pair.toml
```

`--dynamips-image` overrides router image paths, `--console-base-port` selects
the first TCP console port, and `--state-root` selects persistent router storage.
Ctrl+C releases native instances while retaining NVRAM and disks. `--duration`
sets an optional run deadline.

Container specs may select `provider` individually; `--provider` supplies the
default for unspecified providers. Explicit providers never fall back.

`--libvirt-uri` selects the domain daemon, defaulting to `qemu:///system`.
`--domain-image` overrides qcow2 base images and `--domain-seed-image` overrides
seed ISOs. Domains use temporary overlays removed at shutdown.
Shitnet specs supply an IPv4 address and MAC; endpoint-only graphs run without
root privileges. The native Shitnet build dependency comes from the Nix shell.

See the [topology README](../../topology/README.md) for fixture preparation,
TOML fields, library contracts, and bidirectional ping commands.
