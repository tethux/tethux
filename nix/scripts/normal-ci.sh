#!/usr/bin/env bash
set -euo pipefail

archive_dir="${TETHUX_CI_ARCHIVE_DIR:-.}/artifacts"
mkdir -p "$archive_dir"

golangci-lint run -c .golangci.yml
go test ./... -json | tee "$archive_dir/go-test.jsonl"
go build ./cmd/tethux
nix eval .#nixosConfigurations.canary-10-0-0-100.config.system.build.toplevel.drvPath \
  --extra-experimental-features "nix-command flakes"
nix eval .#nixosConfigurations.canary-former-10-0-0-12.config.system.build.toplevel.drvPath \
  --extra-experimental-features "nix-command flakes"
nix eval .#nixosConfigurations.canary-proxmox-vm-9901.config.system.build.toplevel.drvPath \
  --extra-experimental-features "nix-command flakes"
nix build .#checks.x86_64-linux.unit .#checks.x86_64-linux.build \
  --extra-experimental-features "nix-command flakes"
