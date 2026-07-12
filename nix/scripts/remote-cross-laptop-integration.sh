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

go run ./cmd/tethux virt link test \
  --host-a "$host_a" \
  --host-b "$host_b" \
  --provider-a docker \
  --provider-b podman \
  --remote-binary "$remote_dir/bin/tethux"
