# Topology commands

`cmd/topology` defines the `tethux topology` command tree. It reads TOML through
`topology/toml` and uses `topology/local` for provider selection and execution.

```bash
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/container-chain.toml"
```

`run` logs lifecycle progress with slog on stderr and prints nodes, links, and
copyable guest commands on stdout. Docker is tried before Podman by default.
Use `--provider` to select one explicitly and `--base-port` to change the
loopback UDP range. Ctrl+C cleans up this run's resources.

See the [topology README](../../topology/README.md) for fixture preparation,
TOML fields, library contracts, and bidirectional ping commands.
