# Tethux NixOS Canary Hosts

This tree keeps the disposable bare-metal test-host setup in the tethux
monorepo. Forgejo and Woodpecker server components live elsewhere; these
profiles are only for privileged canary runners.

Disclaimer: this Nix setup was shamelessly vibed in with GPT-5.5 in T3 Code.
T3's local state records the current work as `Expand CI integration coverage`
(thread `1a388d12-4db4-466d-828a-9ed8d127e24a`). At the completion audit
(`2026-07-12T21:47:33Z`), T3 recorded 8 user prompts and 136,449,583 processed
tokens: 136,207,027 input tokens, 134,170,112 of them cached, 242,556 output
tokens, and 66,399 reasoning-output tokens. The active context at that event
was 256,582 of 353,400 tokens.

## Hosts

- `canary-10-0-0-100`: current SSH host at `10.0.0.100`.
- `canary-former-10-0-0-12`: old `10.0.0.12`, currently discovered as `10.0.0.78`.
- `canary-proxmox-vm-9901`: optional KVM guest on a remote Proxmox host;
  bootstrap access is `root@192.168.0.107` through the Tailscale jump host
  `root@100.115.225.73`.

Run host discovery:

```bash
mise run host:discover
```

Audit a host after SSH access works:

```bash
HOST=veya@10.0.0.100 mise run host:audit
HOST=veya@10.0.0.78 mise run host:audit
mise run host:audit:proxmox-vm-9901
```

## Before Installing

The disko installer intentionally refuses to define a disk layout unless
`TETHUX_INSTALL_DISK` is set. Confirm the disk manually first:

```bash
ssh root@HOST 'hostname; ip addr; lsblk -o NAME,SIZE,TYPE,MODEL; lscpu; free -h'
```

Then run the installer from a trusted checkout:

```bash
TETHUX_INSTALL_DISK=/dev/nvme0n1 nix/scripts/install-canary.sh \
  veya@10.0.0.100 \
  canary-10-0-0-100
```

For Proxmox VM 9901, the installer must target the guest's 80 GiB `/dev/sda`,
never the Proxmox host's physical disk. The size and KVM assertions make a
mistargeted install fail before the destructive countdown:

```bash
TETHUX_INSTALL_DISK=/dev/sda \
TETHUX_SSH_JUMP=root@100.115.225.73 \
TETHUX_EXPECT_VIRTUALIZATION=kvm \
TETHUX_EXPECT_DISK_SIZE_BYTES=85899345920 \
  nix/scripts/install-canary.sh \
  root@192.168.0.107 canary-proxmox-vm-9901
```

The installed guest enables Tailscale but does not embed an auth key. Complete
tailnet enrollment interactively with `sudo tailscale up`; once it has a stable
Tailscale address, CI can replace the bootstrap ProxyJump route with that
direct address.

## Tests

Normal, unprivileged path:

```bash
mise run ci:normal
```

Privileged canary paths:

```bash
mise run ci:canary:providers
mise run ci:canary:topology
mise run ci:canary:hypervisors
```

The provider suite is available directly as structured JSON Lines. It tests
Docker, Podman, and containerd with Alpine and BusyBox and covers the complete
base-provider and container-provider lifecycles:

```bash
sudo tethux virt test --provider all --output json
```

Each NixOS canary runs a loopback-only OCI registry on `127.0.0.1:5000`.
`fixture-registry.nix` builds two images entirely from Nix packages, seeds the
registry during activation, and exports their references through
`TETHUX_TEST_IMAGES`. Provider and topology CI therefore exercise real pull
operations without depending on Docker Hub, ECR, or another public registry.
The registry persists blobs in `/var/lib/docker-registry`; Docker, Podman, and
containerd keep their normal image caches too. Integration scripts require
these fixture variables and registry health rather than silently falling back
to a public image. Woodpecker's bootstrap image is cache-only (`pull: false`).

VirtualBox and VMware remain optional checks and do not block hosts where those
tools are absent.

The full provider CLI can target a canary over SSH:

```bash
tethux virt test --host ci@10.0.0.78 --provider all --output json
```

The remote host needs the `tethux` package in its NixOS profile and passwordless
sudo for the canary user.

## Woodpecker Topology

The Woodpecker agent remains on `nas` and reaches each laptop over SSH. Every
push, pull request, and manual run has four required, ordered workflows so the
web UI reports each concern independently without exhausting Docker networks:

- `normal`: lint, tests, build, both deployable NixOS evaluations, and flake
  checks on the NAS runner;
- `laptop-100`: all provider operations plus a Docker bridge topology;
- `laptop-78`: all provider operations plus a Podman bridge topology;
- `cross-laptop`: provider-managed containers connected across both machines
  through tethux UDP bridges.

Both laptop workflows also run byte-exact UDP, raw-socket, pcap, and TAP
forwarding tests. Libpcap independently observes the frames; structured
packet metrics and a pcap artifact are included in each archive.

`proxmox-vm-9901` is a fifth, manual-only workflow. It runs the same provider
and networking integration suite through the Proxmox SSH jump host and archives
the result under stable device ID `proxmox-vm-9901`. It is deliberately absent
from push and pull-request events so remote-site availability cannot block the
required fleet.

The NAS runner persists `/nix` in the Docker-managed `tethux-ci-nix` volume;
Docker seeds it from the Nix image on first use instead of hiding that image's
store with an empty bind mount. Go build and module caches remain below
`/var/cache/tethux-ci`, so later workflows and commits reuse downloads and
build products.

## CI archive

Every CI or developer execution wrapped by `test-archive-run.sh` produces one
immutable archive below:

```text
/var/cache/tethux-ci/archive/<full-commit-sha>/<workflow>/<uuidv7>.tar.zst
```

The archive implements Test Archive Format v1. It always contains versioned
`manifest.json` and `results.json`, plus separate `logs/`, `configs/`, and
`artifacts/` entries. The manifest records source/commit/timing, stable device
identity, allowlisted hardware/software metadata, image references, result
counts, file sizes, and SHA-256 checksums. Results normalize Go tests, every
provider operation, topology summaries, and both cross-host endpoints into
stable IDs with statuses, durations, features, parameters, metrics, and
machine-readable failures.

Archives are written as `.tar.zst.partial`, validated, and atomically renamed
only after run IDs, counts, paths, statuses, artifacts, and checksums pass. CI
and ingestion should ignore `.partial` files.

Use the same contract during development:

```bash
TETHUX_TEST_ARCHIVE_ROOT=/var/cache/tethux-ci/archive \
  ./nix/scripts/test-archive-run.sh local-normal \
  nix develop .#ci -c ./nix/scripts/normal-ci.sh
```

Privileged local integration is opt-in and intended for a disposable NixOS
canary. The full run verifies both registry fixtures before doing anything and
never substitutes a public image:

```bash
TETHUX_RUN_INTEGRATION=1 RUNTIME=podman mise run test:integration:local
TETHUX_RUN_INTEGRATION=1 mise run test:bridge-backends:local
```

To atomically publish the resulting archive to the same NAS hierarchy used by
CI, use the `:nas` variants. They upload a `.partial` file and rename it only
after a complete transfer:

```bash
TETHUX_RUN_INTEGRATION=1 RUNTIME=podman mise run test:integration:nas
TETHUX_RUN_INTEGRATION=1 mise run test:bridge-backends:nas
```

Local archives default to the ignored `results/archive` directory. Generate
an ignored, editable inventory of the NAS paths and schemas with:

```bash
mise run archive:nas:inventory
```

That writes `.local/nas-test-archive.md` with current counts, recent archive
paths, contract locations, inspection commands, and space for future notes.

On `nas`, list or inspect an archived commit with:

```bash
find /var/cache/tethux-ci/archive/COMMIT -type f -name '*.tar.zst' -print
tar --zstd -xOf /var/cache/tethux-ci/archive/COMMIT/WORKFLOW/RUN.tar.zst manifest.json | jq .
```

`remote-laptop-integration.sh` copies the exact checkout into a revision-scoped
temporary directory, enters the flake's `integration` shell, and removes it
afterward. The canary users need passwordless sudo. A sleeping/offline laptop
intentionally fails its required workflow instead of silently skipping tests.

## Recovery and disk mounts

The disko installer names partitions `disk-main-root` and `disk-main-ESP`; it
does not assign filesystem labels. Runtime configurations must therefore use
`/dev/disk/by-partlabel/...`. Using `/dev/disk/by-label/nixos` and
`/dev/disk/by-label/boot` caused a live switch to unmount `/boot` and enter
emergency mode while waiting for labels that did not exist.

If a canary reaches the emergency password prompt, choose the previous NixOS
generation from systemd-boot. The repository does not define a root password,
so there is no repository password to enter or recover. Once SSH is restored,
deploy the corrected generation and verify a reboot.

## Codeberg

The `master` branch is mirrored to both configured SSH remotes:

```bash
git push codeberg master
git push origin master
```
