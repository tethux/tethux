# tethux entrypoint

This directory builds the primary multicall CLI:

```bash
go build -o tethux ./cmd/tethux
./tethux --help
```

The binary dispatches `tethux bridge` and `tethux virt`. When invoked through a
symlink named `bridge` or `virt`, it dispatches directly to that component. The
Nix package and CI use this entrypoint.
