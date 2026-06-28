#!/usr/bin/env bash
set -euo pipefail

host="${1:?usage: nix/scripts/audit-host.sh user@host}"

ssh -o BatchMode=yes -o ConnectTimeout=8 "$host" '
set -eu
echo "== identity =="
hostname
id
. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || true
uname -a

echo "== cpu =="
lscpu

echo "== memory =="
free -h

echo "== block devices =="
lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS,MODEL

echo "== virtualization =="
systemd-detect-virt || true
test -e /dev/kvm && echo "KVM=yes" || echo "KVM=no"

echo "== runtimes =="
for bin in docker podman ctr containerd qemu-system-x86_64 virsh VBoxManage vmrun dynamips; do
  if command -v "$bin" >/dev/null 2>&1; then
    printf "%s: " "$bin"
    "$bin" --version 2>/dev/null | head -n1 || true
  else
    echo "$bin: missing"
  fi
done
'
