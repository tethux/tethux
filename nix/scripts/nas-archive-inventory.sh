#!/usr/bin/env bash
set -euo pipefail

nas_host="${1:-nas}"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
output="$repo_root/.local/nas-test-archive.md"
partial="$output.partial"
archive_root=/var/cache/tethux-ci/archive

mkdir -p "$(dirname "$output")"

archive_count="$(ssh "$nas_host" "find '$archive_root' -type f -name '*.tar.zst' | wc -l")"
partial_count="$(ssh "$nas_host" "find '$archive_root' -name '*.partial' | wc -l")"
filesystem="$(ssh "$nas_host" "df -h '$archive_root' | tail -1")"
permissions="$(ssh "$nas_host" "stat -c '%A %U:%G %n' '$archive_root'")"
recent="$(ssh "$nas_host" "find '$archive_root' -type f -name '*.tar.zst' -printf '%T@ %p\n' | sort -nr | head -12 | cut -d' ' -f2-")"

{
  printf '%s\n' \
    '# Local NAS test-archive inventory' \
    '' \
    'This file is deliberately ignored by Git. It is a safe place to add local' \
    'notes or future ingestion ideas without changing the archive contract.' \
    ''
  printf -- '- Generated: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)"
  printf -- '- NAS SSH target: `%s`\n' "$nas_host"
  printf -- '- Archive root: `%s`\n' "$archive_root"
  printf -- '- Final archives: %s\n' "$archive_count"
  printf -- '- Incomplete `.partial` entries: %s\n' "$partial_count"
  printf -- '- Permissions: `%s`\n' "$permissions"
  printf -- '- Filesystem: `%s`\n' "$filesystem"
  printf '%s\n' \
    '' \
    '## Contract and layout' \
    '' \
    'The versioned source of truth is in:' \
    '' \
    '- `nix/test-archive/manifest.schema.json`' \
    '- `nix/test-archive/results.schema.json`' \
    '- `nix/test-archive/README.md`' \
    '' \
    'NAS paths use:' \
    '' \
    '```text'
  printf '%s/<full-commit-sha>/<workflow>/<uuidv7>.tar.zst\n' "$archive_root"
  printf '%s\n' \
    '├── manifest.json' \
    '├── results.json' \
    '├── logs/' \
    '├── configs/' \
    '└── artifacts/' \
    '```' \
    '' \
    'Final archives are immutable. Writers use `.partial` names until validation' \
    'and compression finish; ingestion must ignore those names.' \
    '' \
    '## Most recent archives' \
    '' \
    '```text'
  printf '%s\n' "$recent"
  printf '%s\n' \
    '```' \
    '' \
    '## Inspection commands' \
    '' \
    '```bash'
  printf 'find %s/COMMIT -type f -name '\''*.tar.zst'\'' -print\n' "$archive_root"
  printf '%s\n' \
    'tar --zstd -xOf ARCHIVE.tar.zst manifest.json | jq .' \
    'tar --zstd -xOf ARCHIVE.tar.zst results.json | jq .' \
    '```' \
    '' \
    '## Local opt-in runs' \
    '' \
    '```bash' \
    'TETHUX_RUN_INTEGRATION=1 RUNTIME=podman mise run test:integration:local' \
    'TETHUX_RUN_INTEGRATION=1 mise run test:bridge-backends:local' \
    'TETHUX_RUN_INTEGRATION=1 RUNTIME=podman mise run test:integration:nas' \
    'TETHUX_RUN_INTEGRATION=1 mise run test:bridge-backends:nas' \
    '```' \
    '' \
    'Local archives default to `results/archive`, which is also ignored by Git.' \
    'The full provider/topology run refuses to continue unless the canary fixture' \
    'registry is reachable; it never silently falls back to a public image.' \
    '' \
    '## Future ingestion notes' \
    '' \
    '- Add local dashboard/ingestion experiments here.' \
    '- Treat `schema_version` as the compatibility boundary.' \
    '- Never ingest secrets, absolute paths, or unfinished archives.'
} >"$partial"

mv "$partial" "$output"
printf '%s\n' "$output"
