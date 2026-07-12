# Topology scripts

This directory contains standalone end-to-end topology demonstrations. Public
CLI behavior belongs under `cmd/`; these scripts orchestrate multiple runtime
objects for tests, demos, and load measurements.

## Container UDP topology

The shell runner creates `N` network-isolated containers, gives each a
deterministic interface such as `tx01`, starts one tethux switch per container,
connects adjacent switches with local UDP pairs, and verifies the complete path
with ping:

```bash
sudo ./scripts/container-udp-topology.sh podman 2
sudo ./scripts/container-udp-topology.sh docker 4
```

The Go runner performs the same topology with parallel runtime operations and
is better for larger runs:

```bash
sudo go run ./scripts/container-udp-topology.go \
  --runtime podman --n 67 --parallel-jobs 32
```

Configuration is available through `IMAGE`, `CONTAINER_IF_PREFIX`,
`PARALLEL_JOBS`, and the corresponding Go flags. The canary CI sets `IMAGE` to
its Nix-built local-registry fixture, so topology runs do not depend on an
external registry.
