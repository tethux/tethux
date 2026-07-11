#!/usr/bin/env bash
set -euo pipefail

provider="${1:-all}"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
remote_host="${TETHUX_VIRT_TEST_HOST:-}"

run_provider() {
  local name="$1"
  case "$name" in
    containerd)
      if grep -q 'containerd provider not yet implemented' "$repo_root/cmd/virt/temp_entry_point.go"; then
        echo "skip containerd: tethux CLI provider is not wired yet"
        return 0
      fi
      ;;
  esac

  if [[ -n "$remote_host" ]]; then
    echo "provider smoke: $name on $remote_host"
    go run ./cmd/tethux virt smoke --host "$remote_host" --provider "$name" --name "tethux-smoke-$name"
    return 0
  fi

  case "$name" in
    docker)
      if [[ ! -S /var/run/docker.sock && ! -S /run/docker.sock ]]; then
        echo "skip docker: socket not found"
        return 0
      fi
      ;;
    podman)
      if [[ ! -S /run/podman/podman.sock && ! -S /var/run/podman/podman.sock ]]; then
        echo "skip podman: rootful socket not found"
        return 0
      fi
      ;;
    containerd)
      if [[ ! -S /run/containerd/containerd.sock && ! -S /var/run/containerd/containerd.sock ]]; then
        echo "skip containerd: socket not found"
        return 0
      fi
      ;;
    *)
      echo "unknown provider: $name" >&2
      return 2
      ;;
  esac

  echo "provider smoke: $name"
  go run ./cmd/tethux virt smoke --provider "$name" --name "tethux-smoke-$name"
}

cd "$repo_root"
case "$provider" in
  all)
    run_provider docker
    run_provider podman
    run_provider containerd
    ;;
  docker | podman | containerd)
    run_provider "$provider"
    ;;
  *)
    echo "usage: $0 [all|docker|podman|containerd]" >&2
    exit 2
    ;;
esac
