// Package shitnet runs native IPv4 endpoints over Ethernet frame transports.
package shitnet

import (
	"net/netip"

	native "github.com/tethux/shitnet"
	"github.com/tethux/tethux/virt"
)

// Config assigns one address and UDP Ethernet link to an endpoint.
type Config struct {
	ID string
	Name string
	IP netip.Addr
	MAC native.MAC
	Local netip.AddrPort
	Remote netip.AddrPort
}

// PingResult counts requests and replies validated by source, ID, sequence, and payload.
type PingResult struct { Sent, Received uint16 }

var _ virt.Provider = (*Adapter)(nil)
