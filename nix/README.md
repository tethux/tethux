# Nix test hosts

This directory defines disposable NixOS hosts used for privileged provider,
bridge, topology, and hypervisor tests. Woodpecker runs on the NAS and reaches
the test hosts over SSH.

## Hosts

| Configuration | Address | Purpose |
| --- | --- | --- |
| `test-host-10-0-0-100` | `10.0.0.100` | Docker integration |
| `test-host-former-10-0-0-12` | `10.0.0.78` | Podman integration |
| `test-host-proxmox-vm-9901` | `192.168.0.107` through the configured jump host | Optional remote integration |

Discover and inspect hosts with:

```console
mise run host:discover
HOST=ci@10.0.0.100 mise run host:audit
mise run host:audit:proxmox-vm-9901
```

Installation is deliberately guarded by an explicit whole-disk path and
confirmation:

```console
go run ./tools/ci host install \
  --host ci@10.0.0.100 \
  --flake-host test-host-10-0-0-100 \
  --disk /dev/nvme0n1 \
  --yes
```

Use `--expect-size` and `--expect-virtualization` for remote or virtual targets.
The installer stops before mutation when an assertion fails.

## Tests

The everyday foundation is:

```console
mise run check
mise run test:bridge
```

Privileged suites are explicit:

```console
mise run test:host:providers
mise run test:host:topology
mise run test:host:hypervisors
RUNTIME=podman mise run test:integration:local
```

Each test host provides a loopback OCI fixture registry. Provider tests use
those deterministic images and never silently substitute public images.

Push CI runs the normal checks on the NAS alongside the integration chain.
Docker, Podman, and cross-host workflows still run in that order, but a lint or
unit-test failure does not prevent them from collecting their own results. The
optional Proxmox workflow is manual so remote-site availability cannot block
normal development.

## Test archives

Archive-aware runs write:

```text
/var/cache/tethux-ci/archive/<commit>/<workflow>/<uuidv7>.tar.zst
```

The writer first creates `.partial`, atomically renames the validated archive,
then writes `<archive>.done` containing its SHA-256. Consumers wait for the
marker and verify it before ingestion. The viewer watcher uses filesystem
events plus periodic reconciliation, so missed events on Docker bind mounts or
network filesystems recover automatically.

```console
go run ./tools/ci run normal --archive
mise run archive:nas:inventory
```

See [test-archive/README.md](test-archive/README.md) for the file contract.

## Recovery

Disk mounts use `/dev/disk/by-partlabel/disk-main-root` and
`/dev/disk/by-partlabel/disk-main-ESP`. If a test host enters emergency mode,
boot its previous NixOS generation, restore SSH, and deploy the corrected
generation.
