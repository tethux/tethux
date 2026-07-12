# tethux - sne silly network emulator

> why?

gns3util tried to make gns3 scale but its not made for that so this is my attemtp at making a gns3like thing autoscale

this is a monorpo btw

right now `tethux` contains a small Ethernet switch implementation in
`internal/libtethux` and a Cobra CLI in `cmd/tethux` for exercising it.
the immediate target is a simple uBridge-style UDP/TAP bridge in Go: real
network namespaces get normal interfaces, virtualizers can attach TAP devices,
and switch-to-switch links are UDP sockets.

evil libcap that uses cgo contained in this

todo: move libary docs into readme of the libary

## Library sketch

```go
sw := libtethux.NewSwitch(libtethux.SwitchOptions{})

left, _ := libtethux.NewPort(libtethux.RawScheme, libtethux.PortOptions{
	ID:        "left",
	Interface: "vethA-host",
	MTU:       1500,
})
right, _ := libtethux.NewPort(libtethux.RawScheme, libtethux.PortOptions{
	ID:        "right",
	Interface: "vethB-host",
	MTU:       1500,
})

_ = sw.AttachPort(left)
_ = sw.AttachPort(right)
_ = sw.Start()
defer sw.Stop()
```

Linux namespace setup is still available as a helper:

```go
libtethux.AttachVethToNamespace(pid, "vethA-host", "tx01", 1500)
```

## CLI

Show the command tree:

```bash
go run ./cmd/tethux --help
```

Run the automated tests:

```bash
go test ./...
```

Run the structured two-image integration suite against every container
provider (this requires the three rootful runtime sockets):

```bash
sudo go run ./cmd/tethux virt test --provider all --output json
```

Each JSON Lines record has schema `tethux.provider-test/v1` and identifies the
host, provider, image, API surface, operation, status, and duration. The suite
tests create/delete through the generic provider interface and then pull,
create, start, state, reload, list, inspect, exec, logs, suspend, resume,
restart, stop, and cleanup through both lifecycle APIs.

If you want just the switch behavior tests:

```bash
go test ./internal/libtethux -run TestSwitch -v
```

### UDP bridge ports

Use the generic port form when you want to wire a switch directly to UDP
endpoints:

```bash
go run ./cmd/tethux bridge ports \
  --port id=left,scheme=udp,listen=127.0.0.1:10001,remote=127.0.0.1:11001 \
  --port id=right,scheme=udp,listen=127.0.0.1:10002,remote=127.0.0.1:11002 \
  --port id=uplink,scheme=udp,listen=0.0.0.0:12000,remote=198.51.100.10:13000
```

That exposes local UDP ingress ports and forwards Ethernet frames to remote
emulator endpoints or hosts.

### Mixed transport flow

The generic `bridge ports` command can mix `udp`, `tap`, `raw`, and `pcap` ports in one switch:

```bash
sudo go run ./cmd/tethux bridge ports \
  --port id=vm,scheme=tap,if=tap0 \
  --port id=uplink,scheme=raw,if=tx01 \
  --port id=mirror,scheme=pcap,if=eth1,immediate=true \
  --port id=emulator,scheme=udp,listen=127.0.0.1:10001,remote=127.0.0.1:11001
```

UDP-only bridges run without elevated privileges. TAP/raw/pcap ports need the
normal host privileges for opening those device types, so use `sudo` only for
commands that include them. Use TAP for virtualizers that can attach a TAP NIC,
and UDP for remote emulator links.

### Container UDP topology demo

This is the closer uBridge-style proof: real containers get deterministic
interfaces such as `tx01`, while each `tethux` switch talks to the next switch through a UDP
tunnel. The containers do not need `socat` or any userspace transport helper;
they only see Ethernet and IP.

Run two containers through two switches with Podman:

```bash
sudo ./scripts/container-udp-topology.sh podman 2
```

Or use Docker:

```bash
sudo ./scripts/container-udp-topology.sh docker 2
```

Or through mise:

```bash
RUNTIME=podman N=4 mise run demo-container-udp
```

The shell wrapper is intentionally transparent and prints phase timings. For
larger local tests, the Go runner does the same topology with parallel runtime
operations and local UDP remote pairs:

```bash
sudo go run ./scripts/container-udp-topology.go --runtime podman --n 67
```

Each switch-to-switch link is still a local uBridge-style UDP pair: one switch
listens on `127.0.0.1:<left-port>` and sends to the adjacent switch's
`127.0.0.1:<right-port>`. That makes the remote endpoint behavior testable on
one machine without a separate virtualizer. Increase or reduce runtime
concurrency with `--parallel-jobs 32` or `PARALLEL_JOBS=32`; on slower
container hosts, raise interface setup waiting with `--interface-timeout 30s`.

Container interfaces are deterministic. The default names are `tx01`, `tx02`,
and so on. Set `CONTAINER_IF_PREFIX` to choose another prefix:

```bash
sudo CONTAINER_IF_PREFIX=lab ./scripts/container-udp-topology.sh podman 2
```

The script:

- starts `N` `--net=none` Alpine containers with Podman or Docker
- starts `N` `tethux bridge container` switch processes
- each Go switch creates one veth pair and moves the peer into its container as a deterministic interface
- the wrapper assigns `10.77.0.x/24` addresses after that interface exists
- connects adjacent switches with UDP ports
- verifies the path with `ping` from container 1 to container `N`

The command the wrapper uses per node is `tethux bridge container`. It owns the
interface setup and accepts UDP ports for emulator links:

```bash
sudo go run ./cmd/tethux bridge container \
  --pid "$container_pid" \
  --interface-mode create-veth \
  --host-if tx-demo-1 \
  --container-if tx01 \
  --port id=uplink,scheme=udp,listen=127.0.0.1:23000,remote=127.0.0.1:23001
```

`--interface-mode=create-veth` is the default: the Go code creates the veth
pair and moves the namespace side. Use `--interface-mode=existing` when an
external virtualizer or setup tool already created the host interface and
`tethux` should only open it as a switch port.

### Namespace test flow

This mode needs root privileges and creates veth pairs for two Linux namespaces or containers.

Start two containers:

```bash
sudo podman run --name a --rm -it --cap-add=NET_ADMIN --net=none alpine
sudo podman run --name b --rm -it --cap-add=NET_ADMIN --net=none alpine
```

Bridge them:

```bash
sudo go run ./cmd/tethux bridge namespace \
  "$(sudo podman inspect -f '{{.State.Pid}}' a)" \
  "$(sudo podman inspect -f '{{.State.Pid}}' b)"
```

Inside each container:

```bash
ip addr add 10.0.0.1/24 dev tx01
ip addr add 10.0.0.2/24 dev tx01
```

Useful flags:

- `--pcap` uses pcap instead of raw sockets.
- `--container-if` changes the namespace interface name.
- `--host-a` and `--host-b` change the host-side veth names.
