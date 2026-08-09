package bridge

import "github.com/tethux/tethux/bridge/errs"

// ContainerBridgeOptions configures a bridge between a container interface and UDP.
type ContainerBridgeOptions struct {
	PID         int
	HostIf      string
	ContainerIf string
	Listen      string
	Remote      string
	MTU         int
	UsePcap     bool
	Immediate   bool
	// local middleware wraps the container port; remote middleware wraps udp.
	LocalMiddleware  []PortMiddleware
	RemoteMiddleware []PortMiddleware
}

// ContainerBridge owns a running switch and its host-side interface.
type ContainerBridge struct {
	switcher *Switch
	hostIf   string
}

// StartContainerBridge creates the interfaces, ports, and running switch described by opts.
func StartContainerBridge(opts *ContainerBridgeOptions) (*ContainerBridge, error) {
	if opts == nil {
		return nil, errs.New("start container bridge", errs.ErrInvalidOptions, "options are nil")
	}
	if opts.PID <= 0 || opts.HostIf == "" || opts.ContainerIf == "" || opts.Listen == "" || opts.Remote == "" {
		return nil, errs.New("start container bridge", errs.ErrInvalidOptions, "pid, interface names, listen, and remote are required")
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

// Close stops the switch and cleans up its host-side interface.
func (b *ContainerBridge) Close() error {
	if b == nil {
		return nil
	}
	err := b.switcher.Stop()
	CleanupLink(b.hostIf)
	return err
}
