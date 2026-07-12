# Tethux NixOS Canary Hosts

This tree keeps the disposable bare-metal test-host setup in the tethux
monorepo. Forgejo and Woodpecker server components live elsewhere; these
profiles are only for privileged canary runners.

Disclaimer: this Nix setup was shamelessly vibed in with GPT-5.5 in T3 Code.
T3's local state records the current work as `Expand CI integration coverage`
(thread `1a388d12-4db4-466d-828a-9ed8d127e24a`). At the pre-push completion
audit (`2026-07-12T21:04:22Z`), its user requests had processed 101,867,661
tokens: 101,688,542 input tokens, 99,969,280 of them cached, 179,119 output
tokens, and 49,013 reasoning-output tokens. The active context at that event
was 72,692 of 353,400 tokens.

## Hosts

- `canary-10-0-0-100`: current SSH host at `10.0.0.100`.
- `canary-former-10-0-0-12`: old `10.0.0.12`, currently discovered as `10.0.0.78`.

Run host discovery:

```bash
mise run host:discover
```

Audit a host after SSH access works:

```bash
HOST=veya@10.0.0.100 mise run host:audit
HOST=veya@10.0.0.78 mise run host:audit
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
