#!/usr/bin/env bash
set -euo pipefail

integration_started_at="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
integration_started_ms="$(date -u +%s%3N)"

runtime="${1:?usage: $0 docker|podman}"
case "$runtime" in
  docker | podman) ;;
  *) echo "usage: $0 docker|podman" >&2; exit 2 ;;
esac

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
if [[ -n "${TETHUX_RESULTS_DIR:-}" ]]; then
  results_dir="$TETHUX_RESULTS_DIR"
elif [[ -n "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  results_dir="$TETHUX_CI_ARCHIVE_DIR/artifacts"
else
  results_dir="$repo_root/results"
fi
mkdir -p "$results_dir" "$repo_root/bin"

cd "$repo_root"
: "${TETHUX_FIXTURE_IMAGE_A:?Nix fixture registry image A is required}"
: "${TETHUX_FIXTURE_IMAGE_B:?Nix fixture registry image B is required}"
: "${TETHUX_TEST_IMAGES:?Nix fixture registry image list is required}"
curl --fail --silent http://127.0.0.1:5000/v2/ >/dev/null
architecture="$(uname -m)"
case "$architecture" in x86_64) architecture=amd64 ;; aarch64) architecture=arm64 ;; esac
jq -n \
  --arg device_id "${TETHUX_DEVICE_ID:-$(hostname)}" \
  --arg hostname "$(hostname)" \
  --arg os_version "$(. /etc/os-release && printf '%s' "$PRETTY_NAME")" \
  --arg kernel "$(uname -r)" \
  --arg architecture "$architecture" \
  --arg cpu "$(lscpu | awk -F: '/Model name/{sub(/^[[:space:]]+/,"",$2); print $2; exit}')" \
  --argjson memory_bytes "$(awk '/MemTotal/{print $2*1024}' /proc/meminfo | cut -d. -f1)" \
  --arg runtime "$runtime" \
  --arg runtime_version "$(sudo -n "$runtime" --version | head -1)" \
  --arg image_a "${TETHUX_FIXTURE_IMAGE_A:-}" \
  --arg image_b "${TETHUX_FIXTURE_IMAGE_B:-}" \
  '{device_id:$device_id,display_name:$device_id,hostname:$hostname,os:"linux",os_version:$os_version,kernel:$kernel,architecture:$architecture,cpu:$cpu,memory_bytes:$memory_bytes,container_runtime:$runtime,container_runtime_version:$runtime_version,fixture_images:[$image_a,$image_b]}' \
  >"$results_dir/runner.json"

golangci-lint cache clean
golangci-lint run -c .golangci.yml
go test ./... -json | tee "$results_dir/go-test.jsonl"
go build -o "$repo_root/bin/tethux" ./cmd/tethux
go build ./cmd/bridge/main ./cmd/virt/main

TETHUX_RESULTS_DIR="$results_dir" ./nix/scripts/bridge-backend-smoke.sh

TETHUX_BIN="$repo_root/bin/tethux" \
TETHUX_PROVIDER_RESULTS="$results_dir/providers.jsonl" \
  ./nix/scripts/provider-smoke.sh all

topology_small_n="${TETHUX_TOPOLOGY_SMALL_N:-2}"
topology_large_n="${TETHUX_TOPOLOGY_LARGE_N:-4}"
TETHUX_TOPOLOGY_SMALL_N="$topology_small_n" \
TETHUX_TOPOLOGY_LARGE_N="$topology_large_n" \
TETHUX_TOPOLOGY_PARALLEL_JOBS="${TETHUX_TOPOLOGY_PARALLEL_JOBS:-4}" \
IMAGE="$TETHUX_FIXTURE_IMAGE_A" \
  ./nix/scripts/topology-smoke.sh "$runtime" 2>&1 | tee "$results_dir/topology-$runtime.log"

integration_finished_at="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
integration_finished_ms="$(date -u +%s%3N)"
jq -nc \
  --arg host "$(hostname)" --arg runtime "$runtime" \
  --arg started_at "$integration_started_at" --arg finished_at "$integration_finished_at" \
  --arg image "$TETHUX_FIXTURE_IMAGE_A" \
  --argjson duration_ms "$((integration_finished_ms - integration_started_ms))" \
  --argjson small_n "$topology_small_n" --argjson large_n "$topology_large_n" \
  '{schema:"tethux.laptop-integration/v1",host:$host,runtime:$runtime,status:"passed",started_at:$started_at,finished_at:$finished_at,duration_ms:$duration_ms,image:$image,topology:{small_n:$small_n,large_n:$large_n}}' \
  | tee "$results_dir/summary.jsonl"
