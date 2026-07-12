#!/usr/bin/env bash
set -uo pipefail

workflow="${1:?usage: $0 WORKFLOW COMMAND [ARG ...]}"
shift

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
revision="${CI_COMMIT_SHA:-$(git -C "$repo_root" rev-parse HEAD)}"
if [[ -n "${TETHUX_TEST_ARCHIVE_ROOT:-}" ]]; then
  archive_root="$TETHUX_TEST_ARCHIVE_ROOT"
elif [[ -w /var/cache/tethux-ci/archive ]]; then
  archive_root=/var/cache/tethux-ci/archive
elif [[ -w /var/lib/tethux-ci/archive ]]; then
  archive_root=/var/lib/tethux-ci/archive
else
  archive_root="$repo_root/results/archive"
fi

run_id="${TETHUX_RUN_ID:-$(uuidgen --time-v7)}"
workflow_dir="$archive_root/$revision/$workflow"
stage_dir="$workflow_dir/.$run_id.partial"
archive_partial="$workflow_dir/$run_id.tar.zst.partial"
archive_final="$workflow_dir/$run_id.tar.zst"
started_at="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
started_ms="$(date -u +%s%3N)"

umask 022
mkdir -p "$stage_dir/logs/tests" "$stage_dir/configs" "$stage_dir/artifacts"
export TETHUX_CI_ARCHIVE_DIR="$stage_dir"
export TETHUX_RUN_ID="$run_id"

jq -n \
  --arg run_id "$run_id" \
  --arg workflow "$workflow" \
  --arg commit "$revision" \
  --arg source_type "${TETHUX_SOURCE_TYPE:-${CI:+ci}}" \
  --arg trigger "${CI_PIPELINE_EVENT:-manual}" \
  '{schema_version:1,run_id:$run_id,workflow:$workflow,commit_sha:$commit,source_type:($source_type|if .=="" then "local" else . end),trigger:$trigger}' \
  >"$stage_dir/configs/execution.json"

set +e
"$@" 2>&1 | tee "$stage_dir/logs/runner.log"
status="${PIPESTATUS[0]}"
set -e

finished_at="$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
finished_ms="$(date -u +%s%3N)"
duration_ms="$((finished_ms - started_ms))"

"$repo_root/nix/scripts/test-archive-finalize.sh" \
  "$stage_dir" "$archive_partial" "$archive_final" \
  "$workflow" "$revision" "$run_id" "$status" \
  "$started_at" "$finished_at" "$duration_ms"

exit "$status"
