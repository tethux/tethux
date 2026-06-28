# Tethux NixOS Canary Hosts

This tree keeps the disposable bare-metal test-host setup in the tethux
monorepo. Forgejo and Woodpecker server components live elsewhere; these
profiles are only for privileged canary runners.

Disclaimer: this Nix setup was shamelessly vibed in with GPT-5.5 in T3 Code.
T3's local state recorded this thread as `NixOS CI Test VM Plan`, starting at
`2026-07-11T02:04:47Z`. At the last checked token event
(`2026-07-11T15:44:59Z`), the thread had 19 user messages, 17 turns, and
23,230,282 total processed tokens: 23,138,942 input tokens, 21,801,344 cached
input tokens, 91,340 output tokens, and 18,446 reasoning output tokens.

## Hosts

- `canary-10-0-0-11`: known current SSH host at `10.0.0.11`.
- `canary-former-10-0-0-12`: old `10.0.0.12`, currently discovered as `10.0.0.78`.

Run host discovery:

```bash
mise run host:discover
```

Audit a host after SSH access works:

```bash
HOST=veya@10.0.0.11 mise run host:audit
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
  veya@10.0.0.11 \
  canary-10-0-0-11
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

Containerd is skipped by `provider-smoke.sh` until the CLI provider is wired.
VirtualBox and VMware are optional checks and do not block hosts where those
tools are absent.

The virt smoke CLI can target a canary over SSH:

```bash
tethux virt smoke --host ci@10.0.0.78 --provider docker
TETHUX_VIRT_TEST_HOST=ci@10.0.0.78 tethux virt smoke --provider podman
```

The remote host needs the `tethux` package in its NixOS profile and passwordless
sudo for the canary user.

## Woodpecker Agent Notes

Keep the Woodpecker server on the existing control-plane machine. Register these
laptops as privileged agents/runners with labels similar to:

```text
tethux-canary=true
linux=true
privileged=true
baremetal=true
host=canary-former-10-0-0-12
current-ip=10.0.0.78
```

The canary pipelines in `.woodpecker/` match those labels and assume they run on
the host with passwordless sudo available.

## Codeberg

This repo currently has only the GitHub `origin`. Add a Codeberg remote only
when the private target URL is known:

```bash
git remote add codeberg git@codeberg.org:<owner>/<private-repo>.git
git push codeberg HEAD:refs/heads/nixos-canary-ci
```
