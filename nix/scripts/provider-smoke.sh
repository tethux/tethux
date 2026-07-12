#!/usr/bin/env bash
set -euo pipefail

provider="${1:-all}"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
output="${TETHUX_PROVIDER_RESULTS:-/tmp/tethux-provider-results.jsonl}"
binary="${TETHUX_BIN:-$repo_root/bin/tethux}"
images="${TETHUX_TEST_IMAGES:-}"

case "$provider" in
  all | docker | podman | containerd) ;;
  *)
    echo "usage: $0 [all|docker|podman|containerd]" >&2
    exit 2
    ;;
esac

if [[ ! -x "$binary" ]]; then
  binary="$(mktemp -t tethux-provider-test.XXXXXX)"
  trap 'rm -f "$binary"' EXIT
  (cd "$repo_root" && go build -o "$binary" ./cmd/tethux)
fi

mkdir -p "$(dirname "$output")"
root=(env)
if [[ "$(id -u)" -ne 0 ]]; then
  root=(sudo -n env)
fi

args=(virt test --provider "$provider" --output json)
if [[ -n "$images" ]]; then
  args+=(--images "$images")
fi

"${root[@]}" \
  "PATH=$PATH" \
  "XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-}" \
  DOCKER_HOST=unix:///var/run/docker.sock \
  CONTAINER_HOST=unix:///run/podman/podman.sock \
  CONTAINERD_ADDRESS=/run/containerd/containerd.sock \
  "TETHUX_TEST_IMAGES=$images" \
  "$binary" "${args[@]}" | tee "$output"

echo "provider results: $output" >&2
