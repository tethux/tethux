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
[`cmd/virt`](../cmd/virt/README.md).

API reference: [virt](https://pkg.go.dev/github.com/tethux/tethux/virt) ·
[virt/container](https://pkg.go.dev/github.com/tethux/tethux/virt/container) ·
[virt/domain](https://pkg.go.dev/github.com/tethux/tethux/virt/domain) ·
[Docker](https://pkg.go.dev/github.com/tethux/tethux/virt/container/docker) ·
[Podman](https://pkg.go.dev/github.com/tethux/tethux/virt/container/podman) ·
[containerd](https://pkg.go.dev/github.com/tethux/tethux/virt/container/containerd)
