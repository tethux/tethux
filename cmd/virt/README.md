# Virtualization and container providers

The virt command provides a common CLI over Docker, Podman, and containerd.
Provider sockets are discovered automatically or selected with `--socket`.

## Provider commands

```bash
tethux virt list --provider docker
tethux virt pull --provider containerd public.ecr.aws/docker/library/alpine:3.20
tethux virt logs --provider podman CONTAINER_ID
```

Rootless containerd is discovered at
`$XDG_RUNTIME_DIR/containerd/containerd.sock`. Its OCI image configuration,
delegated cgroup path, logs, and exec FIFOs are handled without requiring the
caller to enter the RootlessKit mount namespace.

`smoke` is the compact interactive check for one image. `test` is the complete
CI contract:

```bash
sudo tethux virt test --provider all --output json
```

The portable default suite uses public Alpine and BusyBox images. Canary CI
overrides them with two Nix-built images from each host's loopback-only fixture
registry. It verifies image pull, generic
provider create/delete, container create/start, state, reload, list, inspect,
exec, logs, suspend/resume, restart, stop, and cleanup. Lifecycle calls use the
generic `Provider` API for one image and the extended `ContainerProvider` API
for the other.

Every output line is an independent JSON object with schema
`tethux.provider-test/v1`, making the stream suitable for CI logs and later
processing with `jq`:

```bash
tethux virt test --provider docker --output json | jq -c 'select(.status == "failed")'
```

To select private or local fixtures explicitly:

```bash
tethux virt test --provider all \
  --images 127.0.0.1:5000/tethux/fixture-a:1,127.0.0.1:5000/tethux/fixture-b:1
```

## Remote execution

The provider suite can be forwarded to a NixOS canary over SSH:

```bash
tethux virt test --host ci@10.0.0.100 --provider all --output json
```

The remote user must have passwordless sudo and the `tethux` binary available.
The repository's laptop CI copies the exact commit to each host instead of
depending on a stale installed checkout.

## Cross-host managed link

`link test` creates a provider-managed container on each laptop, attaches a
veth to each container, connects the host switches over UDP, and proves the
first container can ping the second:

```bash
tethux virt link test \
  --host-a ci@10.0.0.100 --provider-a docker \
  --host-b ci@10.0.0.78 --provider-b podman
```

The endpoint command is also available for manual orchestration:

```bash
sudo tethux virt link endpoint \
  --provider docker \
  --name edge-a \
  --listen 0.0.0.0:24000 \
  --remote 10.0.0.78:24000 \
  --address 10.88.0.1/24 \
  --peer 10.88.0.2
```

Endpoint containers use `--network=none`; all cross-host traffic therefore
has to pass through the tethux veth/raw-socket switch and UDP transport. The
endpoint and its container are removed on completion or signal.

## Errors

Runtime implementations return typed errors from
`internal/libtethux/virt/container/errs`. Callers can use `errors.Is` with the
stable category (for example `ErrFailedToStartContainer`) while retaining the
Docker, Podman, or containerd daemon error as the underlying cause.
