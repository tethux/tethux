#!/usr/bin/env bash
set -euo pipefail

archive="${1:?archive path required}"
nas_host="${2:?NAS SSH host required}"
revision="${3:?commit SHA required}"
workflow="${4:?workflow required}"
remote_root="${TETHUX_NAS_ARCHIVE_ROOT:-/var/cache/tethux-ci/archive}"

[[ "$revision" =~ ^[0-9a-f]{40}$ ]]
[[ "$workflow" =~ ^[A-Za-z0-9._-]+$ ]]
[[ -f "$archive" ]]

filename="$(basename "$archive")"
remote_dir="$remote_root/$revision/$workflow"
remote_final="$remote_dir/$filename"
remote_partial="$remote_final.partial"
expected_sha256="$(sha256sum "$archive" | awk '{print $1}')"

ssh "$nas_host" "mkdir -p '$remote_dir' && test ! -e '$remote_final'"
scp -q "$archive" "$nas_host:$remote_partial"
ssh "$nas_host" "test -s '$remote_partial' && test \"\$(sha256sum '$remote_partial' | awk '{print \$1}')\" = '$expected_sha256' && mv '$remote_partial' '$remote_final'"
printf 'published test archive: %s:%s\n' "$nas_host" "$remote_final"
