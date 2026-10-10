# tethux entrypoint

This directory builds the primary multicall CLI:

```bash
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
./tethux --help
```

The binary dispatches `tethux bridge`, `tethux virt`, and `tethux topology`.
When invoked through a symlink named `bridge` or `virt`, it dispatches directly
to that component. The Nix package and CI use this entrypoint.

Run the example topology from the repository root:

```bash
RUNTIME=docker mise run fixture-registry:start
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/container-chain.toml"
```

The command reads TOML or Lua, selects an available Docker or Podman provider,
and logs startup, nodes, links, and cleanup as JSON Lines through slog on stderr.
Use `--provider containerd` to select containerd. Use absolute paths with `pkexec`.
Ctrl+C removes the run's resources.
See the [topology README](../../topology/README.md) for TOML, Lua, and library details,
and [topology commands](../topology/README.md) for the CLI options.
