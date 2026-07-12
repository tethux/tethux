#!/usr/bin/env bash
set -euo pipefail

host="${1:?usage: $0 user@host docker|podman}"
runtime="${2:?usage: $0 user@host docker|podman}"
revision="${CI_COMMIT_SHA:-$(git rev-parse HEAD)}"
remote_dir="/tmp/tethux-ci-${revision:0:12}"
ssh_opts=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=4
  -o StrictHostKeyChecking=accept-new
  -o UserKnownHostsFile=/tmp/tethux-ci-known-hosts
)

ssh "${ssh_opts[@]}" "$host" "rm -rf '$remote_dir' && mkdir -p '$remote_dir'"
tar --exclude=.git --exclude=.jj --exclude=bin --exclude=results -czf - . | \
  ssh "${ssh_opts[@]}" "$host" "tar -xzf - -C '$remote_dir'"

cleanup() {
  ssh "${ssh_opts[@]}" "$host" "rm -rf '$remote_dir'" >/dev/null 2>&1 || true
}
trap cleanup EXIT

set +e
ssh "${ssh_opts[@]}" "$host" \
  "cd '$remote_dir' && TETHUX_DEVICE_ID='${TETHUX_DEVICE_ID:-$host}' TETHUX_TOPOLOGY_SMALL_N='${TETHUX_TOPOLOGY_SMALL_N:-2}' TETHUX_TOPOLOGY_LARGE_N='${TETHUX_TOPOLOGY_LARGE_N:-4}' TETHUX_TOPOLOGY_PARALLEL_JOBS='${TETHUX_TOPOLOGY_PARALLEL_JOBS:-4}' nix develop .#integration --extra-experimental-features 'nix-command flakes' -c ./nix/scripts/laptop-integration.sh '$runtime'"
status=$?
set -e

if [[ -n "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  mkdir -p "$TETHUX_CI_ARCHIVE_DIR/artifacts/remote"
  ssh "${ssh_opts[@]}" "$host" \
    "test ! -d '$remote_dir/results' || tar -czf - -C '$remote_dir' results" | \
    tar -xzf - -C "$TETHUX_CI_ARCHIVE_DIR/artifacts/remote"
fi

exit "$status"
