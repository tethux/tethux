# Bridge commands

The bridge command package exposes tethux's Ethernet switch implementation.
It supports UDP, TAP, raw-socket, and pcap ports, plus helpers for attaching a
veth interface to a container or network namespace.

Useful commands:

```bash
tethux bridge ports --port id=a,scheme=udp,listen=127.0.0.1:10001,remote=127.0.0.1:10002
sudo tethux bridge container --pid 1234 --container-if tx01 --port id=uplink,scheme=udp,listen=0.0.0.0:23000,remote=10.0.0.2:23000
```

UDP-only switches are unprivileged. TAP, raw sockets, veth creation, and
namespace attachment require the relevant capabilities or root.

## Port types

- `udp` carries Ethernet frames between tethux switches or emulator hosts;
- `tap` attaches a virtual machine or another TAP consumer;
- `raw` reads and writes directly through a Linux interface;
- `pcap` provides immediate packet capture/injection through libpcap.

One switch can mix all four types:

```bash
sudo tethux bridge ports \
  --port id=vm,scheme=tap,if=tap0 \
  --port id=namespace,scheme=raw,if=tx01 \
  --port id=mirror,scheme=pcap,if=eth1,immediate=true \
  --port id=uplink,scheme=udp,listen=0.0.0.0:12000,remote=10.0.0.78:12000
```

## Container and namespace attachment

`bridge container` creates a veth pair, moves one end into the target network
namespace, and owns the host-side switch port:

```bash
sudo tethux bridge container \
  --pid 1234 \
  --host-if tx-demo-1 \
  --container-if tx01 \
  --port id=uplink,scheme=udp,listen=127.0.0.1:23000,remote=127.0.0.1:23001
```

`--interface-mode=create-veth` is the default. Use `existing` when a
virtualizer already created the interface and tethux should only open it.

The `bridge namespace PID_A PID_B` helper directly joins two existing network
namespaces. Useful flags include `--pcap`, `--container-if`, `--host-a`, and
`--host-b`.
