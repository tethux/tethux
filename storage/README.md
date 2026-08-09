# Storage packages

`storage` defines provider-independent object storage and workload volume
preparation APIs. `storage/local` supplies a filesystem-backed provider.

```go
provider, err := local.New("/var/lib/tethux/objects")
ref := storage.Ref{Provider: provider.Name(), Key: "images/router.img"}
```

Import the packages as `github.com/tethux/tethux/storage` and
`github.com/tethux/tethux/storage/local`. Keys are relative to the configured
root; the local provider rejects absolute paths, traversal, and symlink escapes.

API reference: [storage](https://pkg.go.dev/github.com/tethux/tethux/storage) ·
[storage/local](https://pkg.go.dev/github.com/tethux/tethux/storage/local)
