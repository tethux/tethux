package bridge

import (
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket/pcap"
	"github.com/tethux/tethux/internal/libtethux/bridge/errs"
)

func init() {
	RegisterPortFactory("raw", NewRawSocketPort)
	RegisterPortFactory("pcap", NewPcapPort)
	RegisterPortFactory("tap", NewTAPPort)
	RegisterPortFactory("udp", NewUDPPort)
}

type AvailableScheme string

const (
	RawScheme  AvailableScheme = "raw"
	PcapScheme AvailableScheme = "pcap"
	TAPScheme  AvailableScheme = "tap"
	UDPScheme  AvailableScheme = "udp"
)

type PortOptions struct {
	ID            string
	Interface     string
	LocalAddr     string
	Remote        string
	MTU           int
	ImmediateMode bool
	SnapLen       int
}

type PortFactory func(opts *PortOptions) (Port, error)

type PortRegistry struct {
	mu        sync.RWMutex
	factories map[string]PortFactory
}

var defaultRegistry = &PortRegistry{
	factories: make(map[string]PortFactory),
}

func NewPort(scheme AvailableScheme, opts *PortOptions) (Port, error) {
	factory, ok := defaultRegistry.Get(string(scheme))
	if !ok {
		return nil, errs.New("create port", errs.ErrUnknownScheme, string(scheme))
	}

	return factory(opts)
}

func NewRawSocketPort(opts *PortOptions) (Port, error) {
	ifi, err := net.InterfaceByName(opts.Interface)
	if err != nil {
		return nil, err
	}

	proto := htons(syscall.ETH_P_ALL)

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(proto))
	if err != nil {
		return nil, err
	}

	addr := &syscall.SockaddrLinklayer{
		Protocol: proto,
		Ifindex:  ifi.Index,
	}

	if err := syscall.Bind(fd, addr); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}

	id := opts.ID
	if id == "" {
		id = opts.Interface
	}

	return &RawSocketPort{
		id:     id,
		mtu:    opts.MTU,
		fd:     fd,
		ifName: opts.Interface,
	}, nil
}

func NewPcapPort(opts *PortOptions) (Port, error) {
	inactive, errInactive := pcap.NewInactiveHandle(opts.Interface)
	if errInactive != nil {
		return nil, errs.Wrap("create pcap port", errs.ErrPortSetup, opts.Interface, errInactive)
	}
	defer inactive.CleanUp()

	if errMode := inactive.SetImmediateMode(opts.ImmediateMode); errMode != nil {
		return nil, errs.Wrap("configure pcap immediate mode", errs.ErrPortSetup, opts.Interface, errMode)
	}
	if errSnap := inactive.SetSnapLen(opts.SnapLen); errSnap != nil {
		return nil, errs.Wrap("configure pcap snaplen", errs.ErrPortSetup, opts.Interface, errSnap)
	}
	if errPromisc := inactive.SetPromisc(true); errPromisc != nil {
		return nil, errs.Wrap("configure pcap promiscuous mode", errs.ErrPortSetup, opts.Interface, errPromisc)
	}
	if errTimeout := inactive.SetTimeout(1 * time.Millisecond); errTimeout != nil {
		return nil, errs.Wrap("configure pcap timeout", errs.ErrPortSetup, opts.Interface, errTimeout)
	}

	handle, errActivate := inactive.Activate()
	if errActivate != nil {
		return nil, errs.Wrap("activate pcap", errs.ErrPortSetup, opts.Interface, errActivate)
	}

	id := opts.ID
	if id == "" {
		id = opts.Interface
	}

	return &PcapPort{
		id:     id,
		mtu:    opts.MTU,
		ifName: opts.Interface,
		handle: handle,
	}, nil
}

func NewUDPPort(opts *PortOptions) (Port, error) {
	addr, err := net.ResolveUDPAddr("udp", opts.Remote)
	if err != nil {
		return nil, err
	}

	var local *net.UDPAddr
	if opts.LocalAddr != "" {
		local, err = net.ResolveUDPAddr("udp", opts.LocalAddr)
		if err != nil {
			return nil, err
		}
	}

	conn, err := net.ListenUDP("udp", local)
	if err != nil {
		return nil, err
	}

	id := opts.ID
	if id == "" {
		id = conn.LocalAddr().String()
	}

	return &UDPPort{
		id:         id,
		mtu:        opts.MTU,
		conn:       conn,
		remoteAddr: addr,
	}, nil
}

func RegisterPortFactory(name string, f PortFactory) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()

	defaultRegistry.factories[name] = f
}

func (r *PortRegistry) Get(name string) (PortFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.factories[name]
	return f, ok
}
