# Test automation

`internal/ci` is the reusable test harness behind `tools/ci`.

- `Workflow` describes named steps and dependencies.
- `Step` declares an executable, arguments, environment, directory, privilege,
  timeout, accepted exit codes, stdout requirements, captured output, and
  artifacts.
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
    Steps: []ci.Step{
        {Name: "lint", Command: "golangci-lint", Args: []string{"run"}},
        {
            Name:      "test",
            Command:   "go",
            Args:      []string{"test", "./..."},
            DependsOn: []string{"lint"},
        },
    },
}
result, err := ci.NewRunner(os.Stdout, os.Stderr).Run(ctx, workflow)
```

Each step prints `PASS`, `FAIL`, or `SKIP` with its exit code and duration. Add
a step to `BuiltinWorkflows`, or register another `Workflow`; the runner does
not need to change. Dependencies are step names, so the Go declaration stays
as readable as YAML while keeping typed fields and ordinary composition.
Product commands remain below `cmd`. Repository tests, archives, and host
operations belong in `tools/ci`.
