#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage:
  TETHUX_INSTALL_DISK=/dev/nvme0n1 nix/scripts/install-canary.sh root@HOST FLAKE_HOST

examples:
  TETHUX_INSTALL_DISK=/dev/sda nix/scripts/install-canary.sh veya@10.0.0.100 canary-10-0-0-100
  TETHUX_INSTALL_DISK=/dev/nvme0n1 nix/scripts/install-canary.sh veya@10.0.0.78 canary-former-10-0-0-12

This is destructive. It runs nixos-anywhere and overwrites TETHUX_INSTALL_DISK.
The target must have SSH plus passwordless sudo or root SSH.
EOF
}

target="${1:-}"
flake_host="${2:-}"
disk="${TETHUX_INSTALL_DISK:-}"

if [[ -z "$target" || -z "$flake_host" || -z "$disk" ]]; then
  usage
  exit 2
fi

case "$disk" in
  /dev/sd* | /dev/vd* | /dev/nvme*n* | /dev/mmcblk*)
    ;;
  *)
    echo "refusing suspicious TETHUX_INSTALL_DISK: $disk" >&2
    exit 2
    ;;
esac

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
install_flake_host="$flake_host"
if [[ "$install_flake_host" != *-install ]]; then
  install_flake_host="$install_flake_host-install"
fi

echo "Target: $target"
echo "Flake host: $install_flake_host"
echo "Install disk: $disk"
echo
echo "Remote identity and disks:"
ssh -o BatchMode=yes -o ConnectTimeout=8 "$target" \
  'hostname; ip -brief addr; lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS,MODEL; lscpu | sed -n "1,16p"; free -h'

echo
echo "Starting destructive nixos-anywhere install in 10 seconds. Ctrl-C to abort."
sleep 10

cd "$repo_root"
TETHUX_INSTALL_DISK="$disk" nix run github:nix-community/nixos-anywhere -- \
  --flake "$repo_root#$install_flake_host" \
  --option pure-eval false \
  --build-on remote \
  "$target"
