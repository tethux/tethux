# Declarative CI framework

`internal/ci` contains the reusable automation layer for repository CI,
integration labs, archives, and remote hosts. The package has no CLI parsing
and can be embedded by tests or additional provider adapters.

Work is described as `Workflow` values containing named `Step` values. A step
declares its executable, argument vector, environment overlay, working
directory, dependencies, privilege policy, timeout, and outputs. `Runner`
validates the dependency graph, streams output, propagates cancellation, and
preserves child exit codes.

```go
workflow := ci.Workflow{
    Name: "example",
    Steps: []ci.Step{{
        Name: "unit",
        Command: "go",
        Args: []string{"test", "./..."},
        Timeout: 10 * time.Minute,
    }},
}
_, err := ci.NewRunner(os.Stdout, os.Stderr).Run(ctx, workflow)
```

`Registry` separates provider and workflow registration from execution.
Woodpecker is the first source provider; another provider can register its
metadata without modifying the runner. `Remote` builds explicit SSH/SCP
argument vectors and supports jump hosts without routing through a local
shell. Archive generation lives beside these primitives so CI and local runs
use the same Test Archive Format implementation.

The stdlib-`flag` adapter is `tools/ci`. Product commands remain in the Cobra
tree under `cmd`; operational and test automation belongs here.
