#!/usr/bin/env bash
set -euo pipefail

version="${ALPINE_VERSION:-3.24.1}"
base="https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/cloud"
name="generic_alpine-${version}-x86_64-bios-tiny-r0.qcow2"
out="${1:-.local/libvirt/example.qcow2}"
mkdir -p "$(dirname "$out")"

if [[ -e "$out" ]]; then
  printf 'disk already exists: %s\n' "$out"
  exit 0
fi

printf 'downloading Alpine tiny libvirt disk to %s\n' "$out"
curl --fail --location --retry 3 --output "$out" "$base/$name"
qemu-img info "$out"
printf '\nUse it with:\n  go run ./cmd/virt/main libvirt --disk %s\n' "$out"
