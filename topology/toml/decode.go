package toml

import (
	"fmt"
	"io"

	gotoml "github.com/pelletier/go-toml/v2"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
)

// Decode reads a TOML document and returns a validated topology.
// Unknown fields and nodes with multiple spec tables are rejected.
// The caller retains ownership of the reader.
func Decode(reader io.Reader) (*topology.Topology, error) {
	if reader == nil {
		return nil, errs.New("decode topology TOML", errs.ErrDecode, "nil reader")
	}
	var document document
	decoder := gotoml.NewDecoder(reader).DisallowUnknownFields()
	err := decoder.Decode(&document)
	if err != nil {
		return nil, errs.Wrap("decode topology TOML", errs.ErrDecode, "", err)
	}
	result := &topology.Topology{
		ID:    document.ID,
		Nodes: make([]topology.Node, 0, len(document.Nodes)),
		Links: make([]topology.Link, 0, len(document.Links)),
		Views: make([]topology.View, 0, len(document.Views)),
	}
	for _, node := range document.Nodes {
		spec, specErr := node.spec()
		if specErr != nil {
			return nil, errs.Wrap("decode topology TOML", errs.ErrDecode, "", specErr)
		}
		ports := make([]topology.Port, 0, len(node.Ports))
		for _, port := range node.Ports {
			ports = append(ports, topology.Port{ID: port.ID, Kind: port.Kind})
		}
		result.Nodes = append(result.Nodes, topology.Node{
			ID: node.ID, Name: node.Name, Spec: spec, Ports: ports,
		})
	}
	for _, link := range document.Links {
		result.Links = append(result.Links, topology.Link{
			ID: link.ID, Kind: link.Kind, MTU: link.MTU,
			A: topology.Endpoint{Node: link.A.Node, Port: link.A.Port},
			B: topology.Endpoint{Node: link.B.Node, Port: link.B.Port},
		})
	}
	for _, view := range document.Views {
		resultView := topology.View{
			ID: view.ID, Name: view.Name,
			Nodes: make([]topology.NodeView, 0, len(view.Nodes)),
			Links: make([]topology.LinkView, 0, len(view.Links)),
		}
		for _, node := range view.Nodes {
			resultView.Nodes = append(resultView.Nodes, topology.NodeView{
				Node: node.Node, Position: node.Position.position(),
			})
		}
		for _, link := range view.Links {
			points := make([]topology.Position, 0, len(link.Points))
			for _, point := range link.Points {
				points = append(points, point.position())
			}
			resultView.Links = append(resultView.Links, topology.LinkView{Link: link.Link, Points: points})
		}
		result.Views = append(result.Views, resultView)
	}
	err = result.Validate()
	if err != nil {
		return nil, errs.Wrap("decode topology TOML", errs.ErrDecode, "", err)
	}
	return result, nil
}

type document struct {
	ID    topology.ID    `toml:"id"`
	Nodes []nodeDocument `toml:"nodes"`
	Links []linkDocument `toml:"links"`
	Views []viewDocument `toml:"views"`
}

type nodeDocument struct {
	ID        topology.NodeID    `toml:"id"`
	Name      string             `toml:"name"`
	Ports     []portDocument     `toml:"ports"`
	Container *containerDocument `toml:"container"`
	Domain    *domainDocument    `toml:"domain"`
	Shitnet   *struct{}          `toml:"shitnet"`
	Dynamips  *dynamipsDocument  `toml:"dynamips"`
}

type containerDocument struct {
	Image   string        `toml:"image"`
	Command []string      `toml:"command"`
	Env     []envDocument `toml:"env"`
}

type envDocument struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

type domainDocument struct {
	Image    string `toml:"image"`
	CPUs     uint16 `toml:"cpus"`
	MemoryMB uint32 `toml:"memory_mb"`
}

type dynamipsDocument struct {
	Image string `toml:"image"`
	RAMMB uint32 `toml:"ram_mb"`
}

func (node *nodeDocument) spec() (topology.NodeSpec, error) {
	count := 0
	var result topology.NodeSpec
	if node.Container != nil {
		count++
		env := make([]topology.Env, 0, len(node.Container.Env))
		for _, variable := range node.Container.Env {
			env = append(env, topology.Env{Name: variable.Name, Value: variable.Value})
		}
		result = topology.ContainerSpec{Image: node.Container.Image, Command: node.Container.Command, Env: env}
	}
	if node.Domain != nil {
		count++
		result = topology.DomainSpec{Image: node.Domain.Image, CPUs: node.Domain.CPUs, MemoryMB: node.Domain.MemoryMB}
	}
	if node.Shitnet != nil {
		count++
		result = topology.ShitnetSpec{}
	}
	if node.Dynamips != nil {
		count++
		result = topology.DynamipsSpec{Image: node.Dynamips.Image, RAMMB: node.Dynamips.RAMMB}
	}
	if count != 1 {
		return nil, errs.New("decode topology node", errs.ErrInvalidNode,
			fmt.Sprintf("node %q requires exactly one spec table; got %d", node.ID, count))
	}
	return result, nil
}

type portDocument struct {
	ID   topology.PortID   `toml:"id"`
	Kind topology.PortKind `toml:"kind"`
}

type endpointDocument struct {
	Node topology.NodeID `toml:"node"`
	Port topology.PortID `toml:"port"`
}

type linkDocument struct {
	ID   topology.LinkID   `toml:"id"`
	Kind topology.LinkKind `toml:"kind"`
	A    endpointDocument  `toml:"a"`
	B    endpointDocument  `toml:"b"`
	MTU  uint16            `toml:"mtu"`
}

type viewDocument struct {
	ID    topology.ViewID    `toml:"id"`
	Name  string             `toml:"name"`
	Nodes []nodeViewDocument `toml:"nodes"`
	Links []linkViewDocument `toml:"links"`
}

type nodeViewDocument struct {
	Node     topology.NodeID  `toml:"node"`
	Position positionDocument `toml:"position"`
}

type linkViewDocument struct {
	Link   topology.LinkID    `toml:"link"`
	Points []positionDocument `toml:"points"`
}

type positionDocument struct {
	X float64 `toml:"x"`
	Y float64 `toml:"y"`
}

func (position positionDocument) position() topology.Position {
	return topology.Position{X: position.X, Y: position.Y}
}
