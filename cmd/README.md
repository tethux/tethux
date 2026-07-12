# Command packages

`cmd` contains the public tethux command tree and its executable entrypoints.

- `tethux` builds the multicall binary with the `bridge` and `virt` subcommands.
- `bridge` defines Ethernet switch, port, namespace, and container commands.
- `virt` defines provider inspection and integration-test commands.
- `bridge/main` and `virt/main` build standalone component binaries.

The preferred user-facing binary is `tethux`. The standalone binaries use the
same Cobra command constructors and are retained for focused deployments and
compatibility.
