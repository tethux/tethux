# Private bridge integration drivers

The programs in this directory implement bridge integration scenarios for the
unified `tools/ci` command. They are not separate operator-facing commands and
are not part of the public `tethux` command surface.

`example/container-udp` builds its chain from the embedded `chain.lua` script
through `topology/lua` and starts it through `topology/local`, the same runner
used by the CLI. Lua owns node, port, link, and guest setup generation; the Go
driver supplies the node count, image, and MTU as script arguments. It also
decodes the embedded `pair.toml` fixture and tests its two-container link,
using the selected fixture image for both formats.
Guest commands configure addresses and bridge the middle containers. The driver
waits for those addresses, verifies pings in both directions, then closes the
run. Container and bridge lifecycle handling belongs to the shared runner:

Normal CI executes the pair, ring, and chain Lua scripts and the TOML pair
through Go tests. The privileged topology workflows run both the Lua chain and
TOML pair with real containers and bidirectional ping checks. The topology
mise task runs these checks independently of backend conformance testing.
Use `--runtime all` to run both formats on Docker, Podman, and containerd.

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
