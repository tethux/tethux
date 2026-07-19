#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
if [[ -n "${TETHUX_RESULTS_DIR:-}" ]]; then
  results_dir="$TETHUX_RESULTS_DIR"
elif [[ -n "${TETHUX_CI_ARCHIVE_DIR:-}" ]]; then
  results_dir="$TETHUX_CI_ARCHIVE_DIR/artifacts"
else
  results_dir="$repo_root/results"
fi
binary="$repo_root/bin/bridge-backend-smoke"
tethux_binary="$repo_root/bin/tethux"

mkdir -p "$repo_root/bin" "$results_dir"
go build -o "$binary" "$repo_root/scripts/bridge-backend-smoke.go"
go build -o "$tethux_binary" "$repo_root/cmd/tethux"

root_command=(
  env
  "PATH=$PATH"
  "$binary"
  --tethux "$tethux_binary"
  --output "$results_dir/bridge-backends.jsonl"
  --pcap "$results_dir/bridge-backends.pcap"
)
if [[ "$(id -u)" -eq 0 ]]; then
  "${root_command[@]}"
else
  sudo -n "${root_command[@]}"
fi
