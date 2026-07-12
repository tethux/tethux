#!/usr/bin/env bash
set -euo pipefail

runtime="${1:?usage: $0 docker|podman}"
case "$runtime" in
  docker | podman) ;;
  *) echo "usage: $0 docker|podman" >&2; exit 2 ;;
esac

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
results_dir="${TETHUX_RESULTS_DIR:-$repo_root/results}"
mkdir -p "$results_dir" "$repo_root/bin"

cd "$repo_root"
golangci-lint run -c .golangci.yml
go test ./... -json | tee "$results_dir/go-test.jsonl"
go build -o "$repo_root/bin/tethux" ./cmd/tethux
go build ./cmd/bridge/main ./cmd/virt/main

TETHUX_BIN="$repo_root/bin/tethux" \
TETHUX_PROVIDER_RESULTS="$results_dir/providers.jsonl" \
  ./nix/scripts/provider-smoke.sh all

TETHUX_TOPOLOGY_SMALL_N="${TETHUX_TOPOLOGY_SMALL_N:-2}" \
TETHUX_TOPOLOGY_LARGE_N="${TETHUX_TOPOLOGY_LARGE_N:-4}" \
TETHUX_TOPOLOGY_PARALLEL_JOBS="${TETHUX_TOPOLOGY_PARALLEL_JOBS:-4}" \
  ./nix/scripts/topology-smoke.sh "$runtime" 2>&1 | tee "$results_dir/topology-$runtime.log"

printf '{"schema":"tethux.laptop-integration/v1","host":"%s","runtime":"%s","status":"passed"}\n' \
  "$(hostname)" "$runtime" | tee "$results_dir/summary.jsonl"
