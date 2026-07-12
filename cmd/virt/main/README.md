# Standalone virt entrypoint

This entrypoint builds only the provider CLI:

```bash
go build -o tethux-virt ./cmd/virt/main
./tethux-virt --help
```

It exposes the same commands and structured provider tests as `tethux virt`.
