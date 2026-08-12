# Tethux CI command

`tools/ci` is the single operator-facing command for repository checks,
privileged integration suites, remote test hosts, and Test Archive Format
artifacts. Its workflow engine remains private in `internal/ci`.

```console
go run ./tools/ci help
```

## Repository tasks

The Go command owns the monorepo quality gates:

```console
go run ./tools/ci task format
go run ./tools/ci task lint
go run ./tools/ci task test
go run ./tools/ci task build
go run ./tools/ci task check
```

The matching `mise run fmt`, `lint`, `test`, `build`, and `check` tasks are
short aliases. Add repository-wide behavior to `repository.go`, not to
`mise.toml`.

## Dagger and SigNoz

Dagger is the privileged integration boundary. GitHub Actions runs ordinary
repository checks directly on Blacksmith. The Go CLI remains useful locally and
on test hosts, while Woodpecker calls the hardware workflows through traced
Dagger functions:

```console
dagger call normal --source=.
dagger call laptop-100 --source=. \
  --ssh-key=file:$HOME/.ssh/id_rsa \
  --known-hosts=file:$HOME/.ssh/known_hosts
```

Point Dagger at SigNoz with the standard OTLP variables. Self-hosted SigNoz
accepts gRPC on port 4317 without a token:

```console
export OTEL_EXPORTER_OTLP_ENDPOINT=http://nas:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_EXPORTER_OTLP_INSECURE=true
```

Woodpecker only schedules the physical-host functions. Their steps, output,
failures, and timings are inspected in SigNoz; there is no separate ingestion
database or CI viewer.

## Workflows

Run ordinary checks locally:

```console
go run ./tools/ci run normal --archive
```

Run all privileged integration suites on a disposable local lab host:

```console
go run ./tools/ci laptop --runtime podman
```

Run the same laptop workflow remotely. The remote `ci` user needs
non-interactive sudo as configured by the NixOS test-host module.

```console
go run ./tools/ci run remote-laptop \
  --host ci@10.0.0.78 \
  --runtime podman \
  --archive
```

Available workflow names are `normal`, `laptop`, `local`, `remote-laptop`,
`cross-laptop`, `provider`, `topology`, and `bridge`.

## Bridge integration

The `bridge` group is the supported entrypoint for the private drivers under
`tools/bridge`:

```console
go run ./tools/ci bridge list
go run ./tools/ci bridge test --archive
go run ./tools/ci bridge topology --runtime podman --n 4
go run ./tools/ci bridge all --runtime podman
```

These workflows create real network interfaces and containers. CI uses
passwordless `sudo -n`; interactive local runs print a copyable `sudo` command
instead of prompting unexpectedly.

## Archives and hosts

```console
go run ./tools/ci archive inventory --host nas
go run ./tools/ci host discover --subnet 10.0.0
go run ./tools/ci host audit --host ci@10.0.0.100
```

See [`nix/test-archive/README.md`](../../nix/test-archive/README.md) for the
archive contract and [`nix/README.md`](../../nix/README.md) for test-host
installation, privilege, and recovery details.
