// Package shitnet runs native IPv4 endpoints over Ethernet frame transports.
package shitnet

import (
	"net/netip"

	native "github.com/tethux/shitnet"
)

// Config assigns one address and UDP Ethernet link to an endpoint.
type Config struct {
	ID     string
	Name   string
	IP     netip.Addr
	MAC    native.MAC
	Local  netip.AddrPort
	Remote netip.AddrPort
}

var _ Provider = (*Adapter)(nil)
