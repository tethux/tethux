# Virtualization packages

`virt` defines the common lifecycle model for workloads. `virt/container`
adds OCI container configuration and implementations for Docker, Podman, and
containerd; `virt/domain` defines the virtual-machine domain model.

```go
image := container.ParseImage("docker.io/library/alpine:3.22")
provider, err := docker.New()
info := provider.Info()
```

Import these packages beneath `github.com/tethux/tethux/virt`. Provider
operations require their corresponding runtime socket and may require elevated
host privileges. The command workflows are documented in
[`cmd/virt`](../cmd/virt/README.md); use `go doc github.com/tethux/tethux/virt`
and `go doc github.com/tethux/tethux/virt/container` for API details.
