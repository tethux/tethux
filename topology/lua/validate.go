package lua

import (
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
)

func (b *builder) portKind(handle portHandle) (topology.PortKind, error) {
	if handle.builder != b {
		return "", errs.New(
			"resolve Lua port", errs.ErrInvalidPort,
			"port belongs to another topology",
		)
	}

	nodeIndex, nodeExists := b.nodes[handle.endpoint.Node]
	portIndex, portExists := b.ports[handle.endpoint]
	if !nodeExists || !portExists {
		return "", errs.New(
			"resolve Lua port", errs.ErrNotFound,
			handle.endpoint.String(),
		)
	}

	return b.top.Nodes[nodeIndex].Ports[portIndex].Kind, nil
}
