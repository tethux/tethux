#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

usage() {
  echo "usage: sudo $0 [podman|docker] [switch-and-container-count >= 2]" >&2
}

RUNTIME="${1:-${RUNTIME:-podman}}"
N="${2:-${N:-2}}"
BASE_PORT="${BASE_PORT:-23000}"
IMAGE="${IMAGE:-public.ecr.aws/docker/library/alpine:3.20}"
MTU="${MTU:-1500}"
CONTAINER_IF_PREFIX="${CONTAINER_IF_PREFIX:-tx}"
WAIT_ATTEMPTS="${WAIT_ATTEMPTS:-20}"
WAIT_INTERVAL="${WAIT_INTERVAL:-0.1}"
PING_COUNT="${PING_COUNT:-2}"
PING_TIMEOUT="${PING_TIMEOUT:-1}"
PARALLEL_JOBS="${PARALLEL_JOBS:-16}"

if [[ "$RUNTIME" != "podman" && "$RUNTIME" != "docker" ]]; then
  usage
  exit 2
fi

if (( N < 2 )); then
  usage
  exit 2
fi

if (( ${#CONTAINER_IF_PREFIX} > 13 )); then
  echo "CONTAINER_IF_PREFIX must be 13 characters or fewer" >&2
  exit 2
fi

if (( PARALLEL_JOBS < 1 )); then
  echo "PARALLEL_JOBS must be >= 1" >&2
  exit 2
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "this demo needs root for veth/raw sockets: sudo $0 ${RUNTIME} ${N}" >&2
  exit 1
fi

if ! command -v "$RUNTIME" >/dev/null 2>&1; then
  echo "$RUNTIME is required" >&2
  exit 1
fi

SUFFIX="$(date +%s | tail -c 7)"
TETHUX_BIN="${TETHUX_DEMO_BIN:-/run/tethux-demo/tethux-demo-${SUFFIX}}"
GO_RUN=("$TETHUX_BIN")

PIDS=()
CONTAINERS=()

wait_for_job_slot() {
  while (( $(jobs -rp | wc -l) >= PARALLEL_JOBS )); do
    wait -n
  done
}

log_phase_done() {
  local started="$1"
  local label="$2"

  echo "${label} completed in $(( SECONDS - started ))s"
}

remove_container() {
  local name="$1"

  case "$RUNTIME" in
    podman)
      "$RUNTIME" rm -f --time 0 "$name" >/dev/null 2>&1 || true
      ;;
    docker)
      "$RUNTIME" rm -f "$name" >/dev/null 2>&1 || true
      ;;
  esac
}

cleanup_stale_demo_state() {
  echo "preflight: cleaning stale tethux demo state" >&2

  pkill -TERM -f '/tmp/tethux-demo-[0-9]+ bridge container' 2>/dev/null || true
  sleep 2
  pkill -KILL -f '/tmp/tethux-demo-[0-9]+ bridge container' 2>/dev/null || true

  while IFS= read -r name; do
    remove_container "$name"
  done < <("$RUNTIME" ps -a --format '{{.Names}}' 2>/dev/null | grep -E '^tethux-demo-' || true)

  while IFS= read -r ifname; do
    ip link delete "$ifname" 2>/dev/null || true
  done < <(ip -o link show | sed -nE 's/^[0-9]+: (tx[0-9]{8})(@|:).*/\1/p')
}

ensure_udp_ports_available() {
  local last_port=$(( BASE_PORT + (N - 2) * 2 + 1 ))
  local port

  for port in $(seq "$BASE_PORT" "$last_port"); do
    if ss -H -lun "sport = :${port}" | grep -q .; then
      echo "UDP port ${port} is still in use after stale cleanup" >&2
      return 1
    fi
  done
}

cleanup() {
  set +e
  local started="$SECONDS"

  if (( ${#PIDS[@]} > 0 )); then
    echo "cleanup: stopping ${#PIDS[@]} switch processes" >&2
  fi
  for pid in "${PIDS[@]:-}"; do
    kill -TERM "$pid" 2>/dev/null || true
  done
  sleep 0.2
  for pid in "${PIDS[@]:-}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill -KILL "$pid" 2>/dev/null || true
    fi
    wait "$pid" 2>/dev/null || true
  done

  if (( ${#CONTAINERS[@]} > 0 )); then
    echo "cleanup: removing ${#CONTAINERS[@]} containers" >&2
  fi
  for name in "${CONTAINERS[@]:-}"; do
    remove_container "$name" &
  done
  wait

  rm -f "$TETHUX_BIN"

  if (( SECONDS - started > 1 )); then
    echo "cleanup: finished in $(( SECONDS - started ))s" >&2
  fi
}
trap cleanup EXIT
trap 'trap - EXIT; cleanup; exit 130' INT TERM

container_name() {
  printf "tethux-demo-%s-%02d" "$SUFFIX" "$1"
}

host_if_name() {
  printf "tx%s%02d" "$SUFFIX" "$1"
}

container_if_name() {
  printf "%s%02d" "$CONTAINER_IF_PREFIX" "$1"
}

link_port_left() {
  local link_index="$1"
  echo $(( BASE_PORT + (link_index - 1) * 2 ))
}

link_port_right() {
  local link_index="$1"
  echo $(( BASE_PORT + (link_index - 1) * 2 + 1 ))
}

container_pid() {
  "$RUNTIME" inspect -f '{{.State.Pid}}' "$1"
}

wait_for_container_if() {
  local name="$1"
  local ifname="$2"

  for _ in $(seq 1 "$WAIT_ATTEMPTS"); do
    if "$RUNTIME" exec "$name" ip link show "$ifname" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$WAIT_INTERVAL"
  done

  echo "timed out waiting for $ifname in $name" >&2
  return 1
}

cleanup_stale_demo_state
ensure_udp_ports_available
mkdir -p "$(dirname "$TETHUX_BIN")"

echo "[1/5] starting ${N} ${RUNTIME} containers with no network"
phase_started="$SECONDS"
env GOCACHE="${GOCACHE:-/tmp/gocache}" go build -o "$TETHUX_BIN" ./cmd/bridge/main

for i in $(seq 1 "$N"); do
  name="$(container_name "$i")"
  CONTAINERS+=("$name")
  wait_for_job_slot
  "$RUNTIME" run -d --name "$name" --rm --net=none --cap-add=NET_ADMIN --cap-add=NET_RAW "$IMAGE" sleep infinity >/dev/null &
done
wait
log_phase_done "$phase_started" "[1/5]"

echo "[2/5] starting ${N} Go switches; each switch creates its own container veth"
phase_started="$SECONDS"
for i in $(seq 1 "$N"); do
  name="$(container_name "$i")"
  host_if="$(host_if_name "$i")"
  container_if="$(container_if_name "$i")"
  pid="$(container_pid "$name")"
  args=(bridge container "--pid" "$pid" "--interface-mode" "create-veth" "--host-if" "$host_if" "--container-if" "$container_if" "--mtu" "$MTU")

  if (( i > 1 )); then
    left_link=$(( i - 1 ))
    listen="$(link_port_right "$left_link")"
    remote="$(link_port_left "$left_link")"
    args+=("--port" "id=sw${i}-left,scheme=udp,listen=127.0.0.1:${listen},remote=127.0.0.1:${remote},mtu=${MTU}")
  fi

  if (( i < N )); then
    right_link="$i"
    listen="$(link_port_left "$right_link")"
    remote="$(link_port_right "$right_link")"
    args+=("--port" "id=sw${i}-right,scheme=udp,listen=127.0.0.1:${listen},remote=127.0.0.1:${remote},mtu=${MTU}")
  fi

  "${GO_RUN[@]}" "${args[@]}" >"/tmp/tethux-switch-${SUFFIX}-${i}.log" 2>&1 &
  PIDS+=("$!")
done
log_phase_done "$phase_started" "[2/5]"

echo "[3/5] assigning container IPs after Go attaches deterministic interfaces"
phase_started="$SECONDS"
for i in $(seq 1 "$N"); do
  name="$(container_name "$i")"
  container_if="$(container_if_name "$i")"
  wait_for_container_if "$name" "$container_if"
  "$RUNTIME" exec "$name" ip addr add "10.77.0.${i}/24" dev "$container_if"
done
log_phase_done "$phase_started" "[3/5]"

echo "[4/5] topology"
for i in $(seq 1 "$N"); do
  printf "  %s:%s 10.77.0.%-3d <-> switch %-2d" "$(container_name "$i")" "$(container_if_name "$i")" "$i" "$i"
  if (( i < N )); then
    printf " ==udp:%s/%s==" "$(link_port_left "$i")" "$(link_port_right "$i")"
  fi
  printf "\n"
done

echo "[5/5] proving container 1 reaches container ${N} through UDP switch links"
EXIT_CODE=0
"$RUNTIME" exec "$(container_name 1)" ping -c "$PING_COUNT" -W "$PING_TIMEOUT" "10.77.0.${N}" || EXIT_CODE=$?

echo

if [ $EXIT_CODE -eq 0 ]; then
  echo "success: containers are networked through ${N} Go switches and UDP links"
  echo "switch logs: /tmp/tethux-switch-${SUFFIX}-*.log"
else
  echo "ERROR: ping failed with exit code $EXIT_CODE"
  echo "switch logs: /tmp/tethux-switch-${SUFFIX}-*.log"
fi

exit $EXIT_CODE
