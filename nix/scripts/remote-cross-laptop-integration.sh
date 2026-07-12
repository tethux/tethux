#!/usr/bin/env bash
set -euo pipefail

host_a="${TETHUX_LINK_HOST_A:-ci@10.0.0.100}"
host_b="${TETHUX_LINK_HOST_B:-ci@10.0.0.78}"
revision="${CI_COMMIT_SHA:-$(git rev-parse HEAD)}"
remote_dir="/tmp/tethux-cross-${revision:0:12}"
ssh_opts=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
  -o UserKnownHostsFile=/tmp/tethux-ci-known-hosts
)

stage_host() {
  local host="$1"
  ssh "${ssh_opts[@]}" "$host" "rm -rf '$remote_dir' && mkdir -p '$remote_dir'"
  tar --exclude=.git --exclude=.jj --exclude=bin --exclude=results -czf - . | \
    ssh "${ssh_opts[@]}" "$host" "tar -xzf - -C '$remote_dir'"
  ssh "${ssh_opts[@]}" "$host" \
    "cd '$remote_dir' && mkdir -p bin && timeout 10m nix develop .#integration --extra-experimental-features 'nix-command flakes' -c go build -o bin/tethux ./cmd/tethux </dev/null >build.log 2>&1"
  ssh "${ssh_opts[@]}" "$host" "test -x '$remote_dir/bin/tethux'"
}

cleanup() {
  ssh "${ssh_opts[@]}" "$host_a" \
    "sudo -n pkill -TERM -f '$remote_dir/bin/[t]ethux virt link endpoint' || true; sudo -n docker rm -f tethux-cross-a >/dev/null 2>&1 || true; sudo -n ip link delete txcrossa >/dev/null 2>&1 || true" \
    >/dev/null 2>&1 || true
  ssh "${ssh_opts[@]}" "$host_b" \
    "sudo -n pkill -TERM -f '$remote_dir/bin/[t]ethux virt link endpoint' || true; sudo -n podman rm -f tethux-cross-b >/dev/null 2>&1 || true; sudo -n ip link delete txcrossb >/dev/null 2>&1 || true" \
    >/dev/null 2>&1 || true
  ssh "${ssh_opts[@]}" "$host_a" "rm -rf '$remote_dir'" >/dev/null 2>&1 || true
  ssh "${ssh_opts[@]}" "$host_b" "rm -rf '$remote_dir'" >/dev/null 2>&1 || true
}
trap cleanup EXIT

stage_host "$host_a"
stage_host "$host_b"

image="${TETHUX_FIXTURE_IMAGE_A:-127.0.0.1:5000/tethux/fixture-a:1}"
cross_log="${TETHUX_CI_ARCHIVE_DIR:-/tmp}/logs/topology.log"
if [[ -z "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  cross_log=/tmp/tethux-cross-link.log
fi

if [[ -n "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  jq -n \
    --arg host_a "$host_a" --arg host_b "$host_b" \
    --arg provider_a docker --arg provider_b podman --arg image "$image" \
    '{schema_version:1,kind:"cross-laptop",endpoints:[{host:$host_a,provider:$provider_a,address:"10.88.0.1/24"},{host:$host_b,provider:$provider_b,address:"10.88.0.2/24"}],transport:{type:"udp",port:24000},image:$image}' \
    >"$TETHUX_CI_ARCHIVE_DIR/configs/topology.json"
fi

go run ./cmd/tethux virt link test \
  --host-a "$host_a" \
  --host-b "$host_b" \
  --provider-a docker \
  --provider-b podman \
  --remote-binary "$remote_dir/bin/tethux" \
  --image "$image" | tee "$cross_log"

if [[ -n "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  sed -n -e '/^\[host-b\] /{s///;p;b;}' -e '/^{/p' "$cross_log" \
    >"$TETHUX_CI_ARCHIVE_DIR/artifacts/cross-link.jsonl"
fi
