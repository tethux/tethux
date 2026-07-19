package bridge

import (
	"errors"
)

type ContainerBridgeOptions struct {
	PID         int
	HostIf      string
	ContainerIf string
	Listen      string
	Remote      string
	MTU         int
	UsePcap     bool
	Immediate   bool
	// LocalMiddleware is applied to the host-side container port; RemoteMiddleware
	// is applied to the UDP uplink. Use NewPacketLossMiddleware and WithLatency
	// to model an impaired link.
	LocalMiddleware  []PortMiddleware
	RemoteMiddleware []PortMiddleware
}

type ContainerBridge struct {
	switcher *Switch
	hostIf   string
}

func StartContainerBridge(opts *ContainerBridgeOptions) (*ContainerBridge, error) {
	if opts == nil {
		return nil, errors.New("container bridge options are nil")
	}
	if opts.PID <= 0 || opts.HostIf == "" || opts.ContainerIf == "" || opts.Listen == "" || opts.Remote == "" {
		return nil, errors.New("container bridge requires pid, interface names, listen, and remote")
	}
	if opts.MTU == 0 {
		opts.MTU = 1500
	}

	CleanupLink(opts.HostIf)
	if err := AttachNamespaceInterface(NamespaceInterfaceOptions{
		PID:               opts.PID,
		HostSideName:      opts.HostIf,
		ContainerSideName: opts.ContainerIf,
		MTU:               opts.MTU,
	}); err != nil {
		return nil, err
	}

	scheme := RawScheme
	if opts.UsePcap {
		scheme = PcapScheme
	}
	local, err := NewPort(scheme, &PortOptions{
		ID:            "container",
		Interface:     opts.HostIf,
		MTU:           opts.MTU,
		ImmediateMode: opts.Immediate,
		SnapLen:       opts.MTU + 32,
	})
	if err != nil {
		CleanupLink(opts.HostIf)
		return nil, err
	}
	local = WrapPort(local, opts.LocalMiddleware...)
	remote, err := NewPort(UDPScheme, &PortOptions{
		ID:        "remote",
		LocalAddr: opts.Listen,
		Remote:    opts.Remote,
		MTU:       opts.MTU,
	})
	if err != nil {
		_ = local.Close()
		CleanupLink(opts.HostIf)
		return nil, err
	}
	remote = WrapPort(remote, opts.RemoteMiddleware...)

	sw := NewSwitch(SwitchOptions{})
	if err := sw.AttachPort(local); err != nil {
		_ = local.Close()
		_ = remote.Close()
		CleanupLink(opts.HostIf)
		return nil, err
	}
	if err := sw.AttachPort(remote); err != nil {
		_ = sw.Stop()
		CleanupLink(opts.HostIf)
		return nil, err
	}
	if err := sw.Start(); err != nil {
		_ = sw.Stop()
		CleanupLink(opts.HostIf)
		return nil, err
	}
	return &ContainerBridge{switcher: sw, hostIf: opts.HostIf}, nil
}

func (b *ContainerBridge) Close() error {
	if b == nil {
		return nil
	}
	err := b.switcher.Stop()
	CleanupLink(b.hostIf)
	return err
}
