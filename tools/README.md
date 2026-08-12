# Repository tools

`tools` contains repository-maintenance programs. These are not public Tethux
library APIs or end-user command packages.

| Path | Purpose | Documentation |
| --- | --- | --- |
| `ci/` | Unified test, archive, remote-runner, and host-management command | [`ci/README.md`](ci/README.md) |
| `assertlint/` | Repository-specific assertion and structured-error checks | source and tests |
| `bridge/` | Private bridge integration drivers invoked by `tools/ci` | [`bridge/README.md`](bridge/README.md) |

`tools/ci` owns the monorepo task declarations and local execution. Dagger uses
the same command inside its traced CI functions. Privileged and remote
workflows still go through this CLI; do not invoke implementation drivers directly.
