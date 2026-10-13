# Shitnet endpoints

The adapter runs the published Shitnet native stack over the existing UDP
Ethernet transport. Each endpoint owns one IPv4 address, one unicast MAC, and
one link. No VM, TAP, root privileges, or persistent disk is required.

`CreateEndpoint` allocates a stopped stack. `Start` binds its UDP socket and
starts packet processing; `Stop` closes the socket and joins its worker.
`Restart` retains addresses and learned ARP entries. `Delete` releases the
native stack. State, list, reload, and categorized errors follow the common
provider contract. Pause and resume are explicitly unsupported.

The provider owns stack allocation, Ethernet transport, and lifecycle only.
It does not implement REPL commands or a console transport. Packet handling
serializes native calls within each endpoint; adapter calls must be externally
serialized.

The binding needs the installed native `shitnet` pkg-config package. The Nix
development and CI shells build a pinned published source revision and supply
it automatically. A local upstream installation can instead be selected with
`PKG_CONFIG_PATH=/path/to/prefix/lib/pkgconfig`.

Run the [two-endpoint topology](../../../topology/examples/shitnet-pair.toml):

```bash
nix develop -c go run ./cmd/tethux topology run topology/examples/shitnet-pair.toml
```
