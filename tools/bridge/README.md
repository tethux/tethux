# Private bridge integration drivers

The programs in this directory implement bridge integration scenarios for the
unified `tools/ci` command. They are not separate operator-facing commands and
are not part of the public `tethux` command surface.

`example/container-udp` creates network-isolated Docker or Podman containers,
attaches deterministic interfaces, connects tethux switches over UDP, and
verifies the complete path:

```console
go run ./tools/ci bridge topology --runtime podman --n 4
```

`testing/backend-smoke` is the internal driver for privileged, byte-exact UDP,
raw-socket, pcap, and TAP conformance checks. Use the archive-aware wrapper:

```console
go run ./tools/ci bridge test --archive
```

CI and local automation invoke these drivers through `tools/ci`. See the
[`tools/ci` README](../ci/README.md) for the supported command surface.
