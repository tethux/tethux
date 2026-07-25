# Test automation

`internal/ci` is the reusable test harness behind `tools/ci`.

- `Workflow` describes named steps and dependencies.
- `Step` declares an executable, arguments, environment, directory, privilege,
  timeout, captured output, and artifacts.
- `Runner` validates the graph, streams logs, propagates cancellation, preserves
  exit codes, and records structured step results.
- `Registry` keeps workflow and CI-provider registration separate from the
  execution engine.
- `Remote` runs explicit SSH/SCP argument vectors with optional jump hosts.
- `ArchiveWriter` normalizes test events and atomically publishes Test Archive
  Format v1 archives with checksum-bearing completion markers.

```go
workflow := ci.Workflow{
    Name: "unit",
    Steps: []ci.Step{{
        Name:    "go-test",
        Command: "go",
        Args:    []string{"test", "./..."},
    }},
}
result, err := ci.NewRunner(os.Stdout, os.Stderr).Run(ctx, workflow)
```

Add a test by registering a workflow; the runner does not need to change.
Product commands remain below `cmd`. Repository tests, archives, and host
operations belong in `tools/ci`.
