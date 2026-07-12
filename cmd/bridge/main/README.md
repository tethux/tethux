# Standalone bridge entrypoint

This entrypoint builds only the bridge CLI:

```bash
go build -o tethux-bridge ./cmd/bridge/main
./tethux-bridge --help
```

It uses the same command implementation as `tethux bridge`.
