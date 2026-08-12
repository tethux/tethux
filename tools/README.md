# Repository tools

`tools` contains repository-maintenance programs. These are not public Tethux
library APIs or end-user command packages.

| Path | Purpose | Documentation |
| --- | --- | --- |
| `ci/` | Unified test, archive, remote-runner, and host-management command | [`ci/README.md`](ci/README.md) |
| `ci-results/` | Test-archive ingestion and the results viewer | [`ci-results/viewer/README.md`](ci-results/viewer/README.md) |
| `assertlint/` | Repository-specific assertion and structured-error checks | source and tests |
| `bridge/` | Private bridge integration drivers invoked by `tools/ci` | [`bridge/README.md`](bridge/README.md) |

`tools/ci` owns the monorepo task declarations and execution. `mise run check`
is a short alias for `go run ./tools/ci task check`. Privileged and remote
workflows use the same command; do not invoke implementation drivers directly.
