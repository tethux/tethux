package local

import (
	"context"
	"net"
	"net/netip"

	native "github.com/tethux/shitnet"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/virt/emulator/shitnet"
)

func (run *Running) startEndpoint(ctx context.Context, top *topology.Topology, node *topology.Node, spec *topology.ShitnetSpec, base int) error {
	ip, ipErr := netip.ParseAddr(spec.IP)
	if ipErr != nil {
		return errs.Wrap("parse endpoint IP", errs.ErrInvalidNode, string(node.ID), ipErr)
	}
	mac, macErr := net.ParseMAC(spec.MAC)
	if macErr != nil || len(mac) != 6 {
		return errs.Wrap("parse endpoint MAC", errs.ErrInvalidNode, string(node.ID), macErr)
	}
	for index, link := range top.Links {
		for side, endpoint := range []topology.Endpoint{link.A, link.B} {
			if endpoint.Node != node.ID {
				continue
			}
			localPort, remotePort := base+2*index+side, base+2*index+1-side
			if localPort < 1 || localPort > 65535 || remotePort < 1 || remotePort > 65535 {
				return errs.New("start endpoint", errs.ErrInvalidPort, "UDP ports must fit within 1..65535")
			}
			cfg := &shitnet.Config{
				ID: string(node.ID), Name: node.Name, IP: ip, MAC: native.MAC(mac),
				Local:  netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(localPort)),
				Remote: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(remotePort)),
			}
			if _, createErr := run.shitnet.CreateEndpoint(ctx, cfg); createErr != nil {
				return createErr
			}
			run.nodes[node.ID] = cfg.ID
			run.endpoints[node.ID] = struct{}{}
			run.order = append(run.order, node.ID)
			if startErr := run.shitnet.Start(ctx, cfg.ID); startErr != nil {
				return startErr
			}
			run.logger.InfoContext(ctx, "Endpoint running", "node", node.ID, "ipv4", spec.IP, "mac", spec.MAC)
			return nil
		}
	}
	return errs.New("start endpoint", errs.ErrInvalidPort, "Shitnet requires a linked Ethernet port")
}
