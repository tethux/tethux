package lua

import (
	"strconv"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
)

type builder struct {
	top   topology.Topology
	nodes map[topology.NodeID]int
	ports map[topology.Endpoint]int
	used  map[topology.Endpoint]struct{}
}

type nodeHandle struct {
	builder *builder
	id      topology.NodeID
}

type portHandle struct {
	builder  *builder
	endpoint topology.Endpoint
}

func newBuilder(id topology.ID) *builder {
	return &builder{
		top:   topology.Topology{ID: id},
		nodes: make(map[topology.NodeID]int),
		ports: make(map[topology.Endpoint]int),
		used:  make(map[topology.Endpoint]struct{}),
	}
}

func (b *builder) node(
	id topology.NodeID,
	spec topology.NodeSpec,
) (nodeHandle, error) {
	if id == "" || spec == nil {
		return nodeHandle{}, errs.New(
			"create Lua node", errs.ErrInvalidNode, string(id),
		)
	}

	if _, exists := b.nodes[id]; exists {
		return nodeHandle{}, errs.New(
			"create Lua node", errs.ErrDuplicateID, string(id),
		)
	}

	b.nodes[id] = len(b.top.Nodes)
	b.top.Nodes = append(b.top.Nodes, topology.Node{
		ID:   id,
		Spec: spec,
	})

	return nodeHandle{builder: b, id: id}, nil
}

func (n nodeHandle) port(
	id topology.PortID,
	medium topology.PortKind,
) (portHandle, error) {
	if n.builder == nil || id == "" {
		return portHandle{}, errs.New(
			"create Lua port", errs.ErrInvalidPort, string(id),
		)
	}

	b := n.builder
	nodeIndex, exists := b.nodes[n.id]
	if !exists {
		return portHandle{}, errs.New(
			"create Lua port", errs.ErrNotFound, string(n.id),
		)
	}

	endpoint := topology.Endpoint{Node: n.id, Port: id}

	if portIndex, exists := b.ports[endpoint]; exists {
		existing := b.top.Nodes[nodeIndex].Ports[portIndex]

		// An omitted medium accepts the existing port's medium.
		if medium != "" && medium != existing.Kind {
			return portHandle{}, errs.New(
				"retrieve Lua port", errs.ErrInvalidPort,
				endpoint.String(),
			)
		}

		return portHandle{builder: b, endpoint: endpoint}, nil
	}

	if medium == "" {
		medium = topology.PortEthernet
	}

	switch medium {
	case topology.PortEthernet, topology.PortSerial:
	default:
		return portHandle{}, errs.New(
			"create Lua port", errs.ErrInvalidPort, endpoint.String(),
		)
	}

	node := &b.top.Nodes[nodeIndex]
	b.ports[endpoint] = len(node.Ports)
	node.Ports = append(node.Ports, topology.Port{
		ID:   id,
		Kind: medium,
	})

	return portHandle{builder: b, endpoint: endpoint}, nil
}

func (b *builder) link(a, other portHandle, mtu uint16) error {
	aKind, err := b.portKind(a)
	if err != nil {
		return errs.Wrap("create Lua link", errs.ErrInvalidLink, "endpoint A", err)
	}
	bKind, otherErr := b.portKind(other)
	if otherErr != nil {
		return errs.Wrap("create Lua link", errs.ErrInvalidLink, "endpoint B", otherErr)
	}
	if a.endpoint == other.endpoint {
		return errs.New("create Lua link", errs.ErrInvalidLink, "endpoints must be distinct")
	}
	if aKind != bKind {
		return errs.New("create Lua link", errs.ErrInvalidLink, "port media differ")
	}
	for _, endpoint := range []topology.Endpoint{a.endpoint, other.endpoint} {
		if _, exists := b.used[endpoint]; exists {
			return errs.New("create Lua link", errs.ErrInvalidLink, endpoint.String()+" is already linked")
		}
	}
	var kind topology.LinkKind
	switch aKind {
	case topology.PortEthernet:
		kind = topology.LinkEthernet
	case topology.PortSerial:
		kind = topology.LinkSerial
	default:
		return errs.New("create Lua link", errs.ErrInvalidLink, "unsupported port medium")
	}
	b.top.Links = append(b.top.Links, topology.Link{
		ID:   topology.LinkID("link-" + strconv.Itoa(len(b.top.Links)+1)),
		Kind: kind, A: a.endpoint, B: other.endpoint, MTU: mtu,
	})
	b.used[a.endpoint] = struct{}{}
	b.used[other.endpoint] = struct{}{}
	return nil
}
