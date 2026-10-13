# Topology packages

`topology` describes workloads, logical ports, point-to-point links, and optional
views without choosing a provider or host. `topology/toml` decodes that model,
`topology/lua` builds it from trusted Lua scripts, and `topology/local` runs
container and Dynamips workloads with Ethernet links on one Linux host, using
container providers, native router NIOs, and UDP bridges.

## Run a TOML topology

From the repository root:

```bash
RUNTIME=docker mise run fixture-registry:start
mise exec -- go build -tags debug -o ./tethux ./cmd/tethux
pkexec "$PWD/tethux" topology run "$PWD/topology/examples/container-chain.toml"
```

Use absolute paths with `pkexec`, which changes the working directory. The CLI
selects an available Docker or Podman provider; `--provider docker`,
`--provider podman`, or `--provider containerd` selects one explicitly.
`--base-port 24000` changes the first loopback UDP port.
Each link uses two consecutive ports.

Progress, node/container mappings, discovered IPv4 addresses, and links are
logged through slog as one JSON object per line on stderr.
Address discovery is optional: the guest supplies `ip`, and its startup command
owns network configuration. The topology being ready means its containers and
links exist; guest configuration can still be finishing.

The [container chain](examples/container-chain.toml) connects
`client/eth0` to `switch/eth0` and `switch/eth1` to `server/eth0`. Guest commands
assign `10.77.0.1/24` and `10.77.0.3/24` to the endpoints and create a Linux
bridge in the middle container. All containers start with `network=none`.

In another terminal, find the example's nodes by their labels:

```bash
client=$(docker ps -q --filter label=tethux.topology=container-chain --filter label=tethux.node=client)
server=$(docker ps -q --filter label=tethux.topology=container-chain --filter label=tethux.node=server)
switch=$(docker ps -q --filter label=tethux.topology=container-chain --filter label=tethux.node=switch)

docker exec "$client" ping -c 3 -W 2 10.77.0.3
docker exec "$server" ping -c 3 -W 2 10.77.0.1
docker exec -it "$client" sh
docker exec "$switch" ip -br link
```

For Podman, replace `docker` with `podman` and use the same rootful socket/user
as the running topology. Ctrl+C stops bridges and deletes the run's containers
and interfaces. Failed startup rolls back its resources as well.

## Model and TOML

IDs for nodes, links, and views are unique within their own collections. Port
IDs are local to a node, and each port can belong to one link. Endpoints contain
`node` and `port` IDs. View placements reference existing nodes and links;
coordinates must be finite. Views can show any subset without changing the
network.

A TOML document has a required `id`, then `[[nodes]]`, `[[links]]`, and optional
`[[views]]` arrays. Every node has exactly one spec table: `container`, `domain`,
`dynamips`, or `shitnet`. Ports have explicit `id` and `kind` values. Link
endpoints use inline tables, as in the example. Unknown fields are rejected.

Container specs contain `image`, `command`, and `env`, where environment
variables are `{ name = "NAME", value = "value" }` entries. Domain specs use
`image`, `cpus`, and `memory_mb`; Dynamips specs use `image`, `ram_mb`, `platform`, optional `npe`/`midplane`,
`console_port`, `enhanced_console`, `startup_config`, `cards`, and `interfaces`.
Cards select native slot drivers; interfaces map logical port IDs to native
slot and port numbers.
A view's `nodes` place `{ node, position = { x, y } }` entries, while `links`
provide `{ link, points = [{ x, y }] }` entries.

The current local runner accepts container, Dynamips, and Shitnet specs with
Ethernet ports/links.
Linked container port IDs become Linux interface names, must fit within 15 bytes, and
cannot be `lo`. Unlinked ports are left unattached. MTU zero selects 1500.
Domain and serial resources are rejected before allocation.

## Library use

Decode with `topology/toml.Decode(reader)`, or construct a `topology.Topology`
and call `Validate`. Choose a provider with
`local.SelectContainerProvider(ctx, "auto")`, then call
`local.Start(ctx, top, local.Options{Provider: provider})`. An optional
`Options.Logger` receives slog lifecycle messages; nil keeps the library quiet.

For procedural graphs, `topology/lua.Decode(reader)` executes a trusted Lua
script and returns the same validated model. Scripts use `local tx =
require("tethux")`, build with `tx.topology(id)`, `net:node(id, kind, image, command)`,
`node:port(name, medium)`, and `net:link(a, b, mtu)`, then `return net`.
The initial API supports `tx.Kind.Container`, `tx.Medium.Ethernet`, and
`tx.Medium.Serial`. The optional command array overrides the image command,
for example `{ "sleep", "infinity" }` to keep an Alpine container running.
The optional link MTU defaults to zero, letting the local runner select 1500.
Library callers can pass script arguments to `Decode(reader, args...)`; Lua
receives them in `arg`, starting at index 1.
Ports default to Ethernet; repeated calls retrieve the
existing port, and a port cannot belong to two links. Scripts have access to
the standard Lua libraries. Decoding only builds a plan; `tethux topology run`
executes `.lua` files and starts the resulting topology through the local runner.
Other files are decoded as TOML. Only run trusted scripts, as they execute with
the command's privileges. See [pair.lua](examples/pair.lua) and
[ring.lua](examples/ring.lua).
LuaLS definitions live in `lua/types`, with a scoped `.luarc.json` for this
directory.

`Running.ContainerID` resolves a declarative node ID to its runtime ID. The
caller owns `Running` and must call `Close` with a live cleanup context.
The methods are not safe for concurrent use. Errors preserve operation details,
categories, and underlying causes for `errors.Is` and `errors.As`.

API reference: [topology](https://pkg.go.dev/github.com/tethux/tethux/topology) ·
[topology/toml](https://pkg.go.dev/github.com/tethux/tethux/topology/toml) ·
[topology/lua](https://pkg.go.dev/github.com/tethux/tethux/topology/lua) ·
[topology/local](https://pkg.go.dev/github.com/tethux/tethux/topology/local)

Local reference:

```bash
mise exec -- go doc ./topology
mise exec -- go doc ./topology/toml
mise exec -- go doc ./topology/lua
mise exec -- go doc ./topology/local
```

## Dynamips routers

The [two-router example](examples/dynamips-pair.toml) provisions C7200 routers
with connected FastEthernet interfaces. Supply an unpacked IOS image:

```bash
export TETHUX_DYNAMIPS_IMAGE="$HOME/.local/share/tethux/images/cisco/c7200-a3jk9s-mz.124-25g.bin"
tethux topology run topology/examples/dynamips-pair.toml
```

Router-only topologies require no root privileges. Console ports start at 20000;
`--console-base-port` selects another range. `--state-root` defaults to
`~/.local/share/tethux/routers`. Each topology/node pair owns a persistent
directory containing native NVRAM and disk files. Ctrl+C deletes the runtime
instances and releases storage leases while retaining those files. An explicit
`startup_config` provisions the router again on each run.

Library callers supply `Options.Dynamips` with a caller-owned adapter, storage
root, and optional image override. One native runtime owns multiple router VMs;
each router has its own leased state directory. Close the running topology
before closing the adapter and native runtime.

## Shitnet endpoints

`[nodes.shitnet]` assigns an `ip` and a unicast Ethernet `mac` to a native IPv4
endpoint with one linked Ethernet port. The [pair example](examples/shitnet-pair.toml)
runs without root privileges. The native stack answers ARP and ICMP requests;
its adapter also exposes context-bounded outbound `Ping` exchanges.

```bash
nix develop -c go run ./cmd/tethux topology run topology/examples/shitnet-pair.toml
```

Library callers supply `Options.Shitnet` with a caller-owned adapter. Cleanup
closes UDP sockets, joins packet workers, and frees native stacks. The Nix
shell supplies the pinned native pkg-config package used by the Go binding.
