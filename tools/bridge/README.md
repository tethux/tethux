# Private bridge integration drivers

The programs in this directory implement bridge integration scenarios for the
unified `tools/ci` command. They are not separate operator-facing commands and
are not part of the public `tethux` command surface.

`example/container-udp` generates a chain as TOML, prints that exact document
with syntax colors, decodes it through `topology/toml`, and starts it through
`topology/local`, the same runner used by the CLI. Set `NO_COLOR=1` for plain
TOML output. Colors remain enabled in CI logs by default. Guest commands
configure addresses and bridge the middle containers. The driver
waits for those addresses, verifies pings in both directions, then closes the
run. Container and bridge lifecycle handling belongs to the shared runner:

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
