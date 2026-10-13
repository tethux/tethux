# Command packages

`cmd/tethux` builds the multicall CLI with `bridge`, `virt`, and `topology`
subcommands. Invoking it through a symlink named `bridge` or `virt` dispatches
to that component. Nix and CI build this entrypoint.

```bash
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
./tethux --help
```

The component packages define the shared Cobra commands:

- [`bridge`](bridge/README.md): Ethernet ports, switches, and namespace attachments.
- [`topology`](topology/README.md): TOML and Lua graphs across local providers.
- [`virt`](virt/README.md): container, libvirt, and Dynamips management and tests.

For focused deployments, `cmd/bridge/main` and `cmd/virt/main` build standalone
binaries using the same command constructors:

```bash
mise exec -- go build -o tethux-bridge ./cmd/bridge/main
mise exec -- go build -o tethux-virt ./cmd/virt/main
```

Build-time native dependencies are described in the [root README](../README.md#quick-start).
