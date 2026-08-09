# Bridge package

`bridge` is Tethux's embeddable Ethernet data plane. It provides a learning
switch, UDP/TAP/raw-socket/pcap ports, namespace attachment, packet capture,
and latency or packet-loss middleware.

```go
sw := bridge.NewSwitch(bridge.SwitchOptions{})
port, err := bridge.NewUDPPort(&bridge.PortOptions{Address: "127.0.0.1:9000"})
```

Import it as `github.com/tethux/tethux/bridge`. Some transports and namespace
operations require Linux capabilities or root privileges. See
[`tools/bridge`](../tools/bridge/README.md) for runnable examples and backend
conformance tests.

API reference: [bridge](https://pkg.go.dev/github.com/tethux/tethux/bridge) ·
[bridge/errs](https://pkg.go.dev/github.com/tethux/tethux/bridge/errs) ·
[bridge/models](https://pkg.go.dev/github.com/tethux/tethux/bridge/models)
