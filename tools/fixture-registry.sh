#!/usr/bin/env bash
set -euo pipefail

runtime="${RUNTIME:-podman}"
registry_name="tethux-fixture-registry"
seed_name="tethux-fixture-seed"
registry="127.0.0.1:5000"
base_image="docker.io/library/alpine:3.22"
network_args=()
if [[ "$runtime" == "podman" ]]; then network_args=(--network=pasta); fi

case "${1:-}" in
  start)
    if "$runtime" container inspect "$registry_name" >/dev/null 2>&1; then
      "$runtime" start "$registry_name" >/dev/null
    else
      "$runtime" run -d "${network_args[@]}" --name "$registry_name" --restart unless-stopped \
        -p 127.0.0.1:5000:5000 docker.io/library/registry:2 >/dev/null
    fi
    for attempt in {1..30}; do
      curl --fail --silent "http://$registry/v2/" >/dev/null && break
      [[ "$attempt" == 30 ]] && { echo "registry did not become ready at http://$registry" >&2; exit 1; }
      sleep 1
    done
    "$runtime" rm -f "$seed_name" >/dev/null 2>&1 || true
    "$runtime" pull "$base_image" >/dev/null
    "$runtime" run "${network_args[@]}" --name "$seed_name" "$base_image" \
      sh -c 'apk add --no-cache iproute2 iputils >/dev/null'
    "$runtime" commit "$seed_name" "$registry/tethux/fixture-a:1" >/dev/null
    "$runtime" tag "$registry/tethux/fixture-a:1" "$registry/tethux/fixture-b:1"
    push_args=()
    [[ "$runtime" == "podman" ]] && push_args=(--tls-verify=false)
    "$runtime" push "${push_args[@]}" "$registry/tethux/fixture-a:1" >/dev/null
    "$runtime" push "${push_args[@]}" "$registry/tethux/fixture-b:1" >/dev/null
    "$runtime" rm -f "$seed_name" >/dev/null
    echo "fixture registry ready: $registry/tethux/fixture-a:1"
    ;;
  status)
    "$runtime" ps --filter name="$registry_name"
    curl --fail --silent "http://$registry/v2/_catalog"
    echo
    ;;
  stop) "$runtime" rm -f "$registry_name" ;;
  *) echo "usage: $0 start|status|stop" >&2; exit 2 ;;
esac
