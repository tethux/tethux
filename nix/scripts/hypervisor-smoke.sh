#!/usr/bin/env bash
set -euo pipefail

status=0

section() {
  printf '\n== %s ==\n' "$1"
}

section "network primitives"
if [[ "$(id -u)" -ne 0 ]]; then
  echo "network primitive checks need root; re-running through sudo"
  exec sudo "$0" "$@"
fi

ip link add tethux-dummy0 type dummy
ip addr add 198.51.100.10/32 dev tethux-dummy0
ip link set tethux-dummy0 up
ip link delete tethux-dummy0

ip tuntap add dev tethux-tap0 mode tap
ip link set tethux-tap0 up
ip link delete tethux-tap0

section "qemu/kvm"
if command -v qemu-system-x86_64 >/dev/null 2>&1; then
  qemu-system-x86_64 --version | head -n1
  if [[ -e /dev/kvm ]]; then
    test -r /dev/kvm -a -w /dev/kvm && echo "/dev/kvm usable" || echo "/dev/kvm exists but permissions need review"
  else
    echo "skip qemu kvm acceleration: /dev/kvm missing"
  fi
else
  echo "skip qemu: binary missing"
fi

section "libvirt"
if command -v virsh >/dev/null 2>&1; then
  virsh list --all || status=1
else
  echo "skip libvirt: virsh missing"
fi

section "dynamips"
if command -v dynamips >/dev/null 2>&1; then
  port="${TETHUX_DYNAMIPS_PORT:-7200}"
  timeout 8 dynamips -H "127.0.0.1:$port" >/tmp/tethux-dynamips.log 2>&1 &
  pid="$!"
  for _ in $(seq 1 20); do
    if timeout 1 bash -c "</dev/tcp/127.0.0.1/$port" >/dev/null 2>&1; then
      echo "dynamips hypervisor opened 127.0.0.1:$port"
      kill "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
      break
    fi
    sleep 0.2
  done
  if kill -0 "$pid" >/dev/null 2>&1; then
    echo "dynamips did not open port $port"
    kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
    status=1
  fi
else
  echo "skip dynamips: binary missing"
fi

section "virtualbox optional"
if command -v VBoxManage >/dev/null 2>&1; then
  VBoxManage --version
  VBoxManage list vms >/dev/null
else
  echo "skip virtualbox: VBoxManage missing"
fi

section "vmware optional"
if command -v vmrun >/dev/null 2>&1; then
  vmrun 2>&1 | head -n1 || true
else
  echo "skip vmware: vmrun missing"
fi

exit "$status"
