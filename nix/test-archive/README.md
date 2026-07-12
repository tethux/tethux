# Test Archive Format v1

Each execution produces one immutable `<uuidv7>.tar.zst` containing:

```text
manifest.json
results.json
logs/
configs/
artifacts/
```

`manifest.json` describes the source, full Git revision, UTC timing, stable
device identity, allowlisted machine/software data, summary, fixture images,
and every archive entry with its media type, byte size, SHA-256, and publicity
flag. `results.json` contains one normalized record per Go test, provider
operation, topology run, or cross-host endpoint.

The schemas in this directory are the machine-readable v1 contract. Stable
test IDs use lowercase path syntax. Allowed statuses are `passed`, `failed`,
`skipped`, `error`, and `cancelled`; infrastructure exits are `error`, not an
assertion failure.

The writer validates IDs, statuses, matching run IDs, summary counts, relative
paths, artifact existence, and checksums before atomically renaming the final
archive. Files ending in `.partial` are incomplete and must be ignored.

Run locally with the same writer used by Woodpecker:

```bash
TETHUX_TEST_ARCHIVE_ROOT=./results/archive \
  ./nix/scripts/test-archive-run.sh local-go \
  sh -c 'go test ./... -json | tee "$TETHUX_CI_ARCHIVE_DIR/artifacts/go-test.jsonl"'
```

Only allowlisted metadata is collected. Environment dumps, credentials, SSH
material, authorization headers, cookies, and private process environments are
never added automatically.
