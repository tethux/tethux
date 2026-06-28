#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
runtime="${1:-all}"
small_n="${TETHUX_TOPOLOGY_SMALL_N:-4}"
large_n="${TETHUX_TOPOLOGY_LARGE_N:-16}"
parallel_jobs="${TETHUX_TOPOLOGY_PARALLEL_JOBS:-8}"
sudo_env=(
  sudo
  env
  "PATH=${PATH}"
  "PKG_CONFIG_PATH=${PKG_CONFIG_PATH:-}"
  "CGO_ENABLED=${CGO_ENABLED:-1}"
  "CGO_CFLAGS=${CGO_CFLAGS:-}"
  "CGO_LDFLAGS=${CGO_LDFLAGS:-}"
  "LD_LIBRARY_PATH=${LD_LIBRARY_PATH:-}"
)

run_runtime() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "skip $name topology: runtime binary missing"
    return 0
  fi

  echo "topology smoke: $name n=$small_n"
  "${sudo_env[@]}" "$repo_root/scripts/container-udp-topology.sh" "$name" "$small_n"

  echo "topology smoke: $name n=$large_n"
  "${sudo_env[@]}" go run "$repo_root/scripts/container-udp-topology.go" \
    --runtime "$name" \
    --n "$large_n" \
    --parallel-jobs "$parallel_jobs"
}

case "$runtime" in
  all)
    run_runtime podman
    run_runtime docker
    ;;
  podman | docker)
    run_runtime "$runtime"
    ;;
  *)
    echo "usage: $0 [all|podman|docker]" >&2
    exit 2
    ;;
esac
