#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
mode="${1:-}"

if [[ "${TETHUX_RUN_INTEGRATION:-}" != "1" ]]; then
  cat >&2 <<'MESSAGE'
Local privileged integration tests are opt-in.
Set TETHUX_RUN_INTEGRATION=1 after confirming this is a disposable lab host.
MESSAGE
  exit 2
fi

archive_root="${TETHUX_TEST_ARCHIVE_ROOT:-$repo_root/results/archive}"
export TETHUX_TEST_ARCHIVE_ROOT="$archive_root"
export TETHUX_SOURCE_TYPE=local
export TETHUX_DEVICE_ID="${TETHUX_DEVICE_ID:-local-$(hostname)}"

case "$mode" in
  bridge-backends)
    exec "$repo_root/nix/scripts/test-archive-run.sh" \
      local-bridge-backends \
      "$repo_root/nix/scripts/bridge-backend-smoke.sh"
    ;;
  docker | podman)
    registry="${TETHUX_FIXTURE_REGISTRY:-http://127.0.0.1:5000}"
    if ! curl --fail --silent "$registry/v2/" >/dev/null; then
      echo "fixture registry is unavailable at $registry" >&2
      echo "run this on a configured NixOS canary; no public-image fallback is used" >&2
      exit 1
    fi
    export TETHUX_FIXTURE_IMAGE_A="${TETHUX_FIXTURE_IMAGE_A:-127.0.0.1:5000/tethux/fixture-a:1}"
    export TETHUX_FIXTURE_IMAGE_B="${TETHUX_FIXTURE_IMAGE_B:-127.0.0.1:5000/tethux/fixture-b:1}"
    export TETHUX_TEST_IMAGES="${TETHUX_TEST_IMAGES:-$TETHUX_FIXTURE_IMAGE_A,$TETHUX_FIXTURE_IMAGE_B}"
    for repository in tethux/fixture-a tethux/fixture-b; do
      curl --fail --silent "$registry/v2/$repository/tags/list" | jq -e '.tags | index("1") != null' >/dev/null
    done
    exec "$repo_root/nix/scripts/test-archive-run.sh" \
      "local-laptop-$mode" \
      "$repo_root/nix/scripts/laptop-integration.sh" "$mode"
    ;;
  *)
    echo "usage: TETHUX_RUN_INTEGRATION=1 $0 bridge-backends|docker|podman" >&2
    exit 2
    ;;
esac
