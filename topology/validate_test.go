package topology_test

import (
	"errors"
	"math"
	"testing"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
)

func testTopology() *topology.Topology {
	return &topology.Topology{
		ID: "lab",
		Nodes: []topology.Node{
			{ID: "a", Spec: topology.ContainerSpec{Image: "alpine:3.23"}, Ports: []topology.Port{{ID: "eth0", Kind: topology.PortEthernet}}},
			{ID: "b", Spec: topology.DomainSpec{Image: "alpine.qcow2"}, Ports: []topology.Port{{ID: "eth0", Kind: topology.PortEthernet}}},
		},
		Links: []topology.Link{{ID: "ab", Kind: topology.LinkEthernet, A: topology.Endpoint{Node: "a", Port: "eth0"}, B: topology.Endpoint{Node: "b", Port: "eth0"}}},
		Views: []topology.View{{ID: "main", Nodes: []topology.NodeView{{Node: "a"}}, Links: []topology.LinkView{{Link: "ab"}}}},
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name     string
		change   func(*topology.Topology)
		category error
	}{
		{name: "valid"},
		{name: "missing topology ID", change: func(top *topology.Topology) { top.ID = "" }, category: errs.ErrInvalidTopology},
		{name: "duplicate node", change: func(top *topology.Topology) { top.Nodes = append(top.Nodes, top.Nodes[0]) }, category: errs.ErrDuplicateID},
		{name: "missing spec", change: func(top *topology.Topology) { top.Nodes[0].Spec = nil }, category: errs.ErrInvalidNode},
		{name: "typed nil spec", change: func(top *topology.Topology) { var spec *topology.ContainerSpec; top.Nodes[0].Spec = spec }, category: errs.ErrInvalidNode},
		{name: "pointer spec", change: func(top *topology.Topology) { top.Nodes[0].Spec = &topology.ContainerSpec{Image: "alpine"} }, category: errs.ErrInvalidNode},
		{name: "missing dynamips image", change: func(top *topology.Topology) { top.Nodes[0].Spec = topology.DynamipsSpec{} }, category: errs.ErrInvalidNode},
		{name: "duplicate environment", change: func(top *topology.Topology) {
			top.Nodes[0].Spec = topology.ContainerSpec{Image: "alpine", Env: []topology.Env{{Name: "PATH"}, {Name: "PATH"}}}
		}, category: errs.ErrInvalidNode},
		{name: "invalid environment name", change: func(top *topology.Topology) {
			top.Nodes[0].Spec = topology.ContainerSpec{Image: "alpine", Env: []topology.Env{{Name: "A=B"}}}
		}, category: errs.ErrInvalidNode},
		{name: "duplicate port", change: func(top *topology.Topology) { top.Nodes[0].Ports = append(top.Nodes[0].Ports, top.Nodes[0].Ports[0]) }, category: errs.ErrDuplicateID},
		{name: "missing endpoint", change: func(top *topology.Topology) { top.Links[0].B.Port = "missing" }, category: errs.ErrNotFound},
		{name: "incompatible port", change: func(top *topology.Topology) { top.Nodes[1].Ports[0].Kind = topology.PortSerial }, category: errs.ErrInvalidLink},
		{name: "self connection", change: func(top *topology.Topology) { top.Links[0].B = top.Links[0].A }, category: errs.ErrInvalidLink},
		{name: "duplicate link", change: func(top *topology.Topology) { top.Links = append(top.Links, top.Links[0]) }, category: errs.ErrDuplicateID},
		{name: "reused endpoint", change: func(top *topology.Topology) {
			link := top.Links[0]
			link.ID = "other"
			top.Links = append(top.Links, link)
		}, category: errs.ErrInvalidLink},
		{name: "duplicate view", change: func(top *topology.Topology) { top.Views = append(top.Views, top.Views[0]) }, category: errs.ErrDuplicateID},
		{name: "missing view node", change: func(top *topology.Topology) { top.Views[0].Nodes[0].Node = "missing" }, category: errs.ErrInvalidView},
		{name: "missing view link", change: func(top *topology.Topology) { top.Views[0].Links[0].Link = "missing" }, category: errs.ErrInvalidView},
		{name: "duplicate node placement", change: func(top *topology.Topology) { top.Views[0].Nodes = append(top.Views[0].Nodes, top.Views[0].Nodes[0]) }, category: errs.ErrDuplicateID},
		{name: "duplicate link placement", change: func(top *topology.Topology) { top.Views[0].Links = append(top.Views[0].Links, top.Views[0].Links[0]) }, category: errs.ErrDuplicateID},
		{name: "non-finite node position", change: func(top *topology.Topology) { top.Views[0].Nodes[0].Position.X = math.NaN() }, category: errs.ErrInvalidView},
		{name: "non-finite link point", change: func(top *topology.Topology) { top.Views[0].Links[0].Points = []topology.Position{{Y: math.Inf(1)}} }, category: errs.ErrInvalidView},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			top := testTopology()
			if test.change != nil {
				test.change(top)
			}
			err := top.Validate()
			if test.category == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, test.category) {
				t.Fatalf("got %v, want category %v", err, test.category)
			}
			var operation *errs.OpError
			if !errors.As(err, &operation) {
				t.Fatalf("missing operation error: %v", err)
			}
		})
	}
}

func TestValidateNil(t *testing.T) {
	var top *topology.Topology
	if err := top.Validate(); !errors.Is(err, errs.ErrInvalidTopology) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateSerial(t *testing.T) {
	top := testTopology()
	top.Links[0].Kind = topology.LinkSerial
	for index := range top.Nodes {
		top.Nodes[index].Ports[0].Kind = topology.PortSerial
	}
	if err := top.Validate(); err != nil {
		t.Fatal(err)
	}
}
