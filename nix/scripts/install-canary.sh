#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage:
  TETHUX_INSTALL_DISK=/dev/nvme0n1 nix/scripts/install-canary.sh root@HOST FLAKE_HOST

examples:
  TETHUX_INSTALL_DISK=/dev/sda nix/scripts/install-canary.sh veya@10.0.0.100 canary-10-0-0-100
  TETHUX_INSTALL_DISK=/dev/nvme0n1 nix/scripts/install-canary.sh veya@10.0.0.78 canary-former-10-0-0-12
  TETHUX_INSTALL_DISK=/dev/sda TETHUX_SSH_JUMP=root@100.115.225.73 \
    TETHUX_EXPECT_VIRTUALIZATION=kvm TETHUX_EXPECT_DISK_SIZE_BYTES=85899345920 \
    nix/scripts/install-canary.sh root@192.168.0.107 canary-proxmox-vm-9901

This is destructive. It runs nixos-anywhere and overwrites TETHUX_INSTALL_DISK.
The target must have SSH plus passwordless sudo or root SSH.
EOF
}

target="${1:-}"
flake_host="${2:-}"
disk="${TETHUX_INSTALL_DISK:-}"
ssh_jump="${TETHUX_SSH_JUMP:-}"
expected_virtualization="${TETHUX_EXPECT_VIRTUALIZATION:-}"
expected_disk_size_bytes="${TETHUX_EXPECT_DISK_SIZE_BYTES:-}"

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
ssh_opts=(
  -o BatchMode=yes
  -o ConnectTimeout=8
  -o StrictHostKeyChecking=accept-new
)
nixos_anywhere_ssh_opts=()
if [[ -n "$ssh_jump" ]]; then
  ssh_opts+=(-J "$ssh_jump")
  nixos_anywhere_ssh_opts+=(--ssh-option "ProxyJump=$ssh_jump")
fi
install_flake_host="$flake_host"
if [[ "$install_flake_host" != *-install ]]; then
  install_flake_host="$install_flake_host-install"
fi

echo "Target: $target"
echo "Flake host: $install_flake_host"
echo "Install disk: $disk"
echo
echo "Remote identity and disks:"
ssh "${ssh_opts[@]}" "$target" \
  'hostname; ip -brief addr; lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS,MODEL; lscpu | sed -n "1,16p"; free -h'

remote_disk_type="$(ssh "${ssh_opts[@]}" "$target" "lsblk -dnro TYPE '$disk'")"
remote_disk_size_bytes="$(ssh "${ssh_opts[@]}" "$target" "blockdev --getsize64 '$disk'")"
remote_virtualization="$(ssh "${ssh_opts[@]}" "$target" 'systemd-detect-virt || true')"

if [[ "$remote_disk_type" != "disk" ]]; then
  echo "refusing target because $disk is type '$remote_disk_type', not a whole disk" >&2
  exit 2
fi
if [[ -n "$expected_disk_size_bytes" && "$remote_disk_size_bytes" != "$expected_disk_size_bytes" ]]; then
  echo "refusing target because $disk is $remote_disk_size_bytes bytes, expected $expected_disk_size_bytes" >&2
  exit 2
fi
if [[ -n "$expected_virtualization" && "$remote_virtualization" != "$expected_virtualization" ]]; then
  echo "refusing target because virtualization is '$remote_virtualization', expected '$expected_virtualization'" >&2
  exit 2
fi

echo "Verified whole disk: $disk ($remote_disk_size_bytes bytes)"
echo "Virtualization: ${remote_virtualization:-none}"

echo
echo "Starting destructive nixos-anywhere install in 10 seconds. Ctrl-C to abort."
sleep 10

cd "$repo_root"
TETHUX_INSTALL_DISK="$disk" nix run github:nix-community/nixos-anywhere -- \
  --flake "$repo_root#$install_flake_host" \
  --option pure-eval false \
  --build-on remote \
  "${nixos_anywhere_ssh_opts[@]}" \
  "$target"
