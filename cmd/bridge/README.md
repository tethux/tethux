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
