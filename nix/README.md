# Tethux NixOS Canary Hosts

This tree keeps the disposable bare-metal test-host setup in the tethux
monorepo. Forgejo and Woodpecker server components live elsewhere; these
profiles are only for privileged canary runners.

Disclaimer: this Nix setup was shamelessly vibed in with GPT-5.5 in T3 Code.
T3's local state records the current work as `Expand CI integration coverage`
(thread `1a388d12-4db4-466d-828a-9ed8d127e24a`). At the completion-audit token
event (`2026-07-12T18:01:37Z`), its five user requests had processed 54,911,358
tokens: 54,800,575 input tokens, 53,699,840 of them cached, 110,783 output
tokens, and 30,655 reasoning-output tokens. The active context at that event
was 109,111 of 353,400 tokens.

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

The default fixtures use Docker's public ECR mirror to avoid anonymous Docker
Hub pull limits making hardware CI flaky.

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
