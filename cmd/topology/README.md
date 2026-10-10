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
The local runner supports container nodes and Ethernet links.

See the [topology README](../../topology/README.md) for fixture preparation,
TOML fields, library contracts, and bidirectional ping commands.
