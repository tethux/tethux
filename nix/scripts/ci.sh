#!/usr/bin/env bash
set -uo pipefail

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root"

run_laptop() {
  local label="$1"
  shift
  set -o pipefail
  ./nix/scripts/remote-laptop-integration.sh "$@" 2>&1 | sed -u "s/^/[$label] /"
}

run_laptop laptop-11 ci@10.0.0.11 docker &
laptop_11_pid=$!
run_laptop laptop-78 ci@10.0.0.78 podman &
laptop_78_pid=$!

normal_status=0
golangci-lint run -c .golangci.yml || normal_status=$?
if (( normal_status == 0 )); then
  go test ./... -json | tee go-test.jsonl || normal_status=$?
fi
if (( normal_status == 0 )); then
  go build ./cmd/tethux || normal_status=$?
fi
if (( normal_status == 0 )); then
  nix eval .#nixosConfigurations.canary-10-0-0-11.config.system.build.toplevel.drvPath \
    --extra-experimental-features "nix-command flakes" || normal_status=$?
fi
if (( normal_status == 0 )); then
  nix eval .#nixosConfigurations.canary-former-10-0-0-12.config.system.build.toplevel.drvPath \
    --extra-experimental-features "nix-command flakes" || normal_status=$?
fi
if (( normal_status == 0 )); then
  nix build .#checks.x86_64-linux.unit .#checks.x86_64-linux.build \
    --extra-experimental-features "nix-command flakes" || normal_status=$?
fi

laptop_11_status=0
laptop_78_status=0
wait "$laptop_11_pid" || laptop_11_status=$?
wait "$laptop_78_pid" || laptop_78_status=$?

printf 'CI results: normal=%d laptop-11=%d laptop-78=%d\n' \
  "$normal_status" "$laptop_11_status" "$laptop_78_status"

if (( normal_status != 0 || laptop_11_status != 0 || laptop_78_status != 0 )); then
  exit 1
fi
