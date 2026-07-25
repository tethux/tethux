# Bridge examples and test tools

The tools in this directory exercise bridge behavior without becoming part of
the public `tethux` command surface.

`example/container-udp` creates network-isolated Docker or Podman containers,
attaches deterministic interfaces, connects tethux switches over UDP, and
verifies the complete path:

```console
go run ./tools/ci topology container-udp --runtime podman --n 4
```

`testing/backend-smoke` performs privileged, byte-exact UDP, raw-socket, pcap,
and TAP conformance checks. Use the archive-aware wrapper:

```console
go run ./tools/ci run bridge --archive
```

CI and local automation invoke the reusable APIs in `internal/ci`; these
programs remain focused examples or test drivers.
