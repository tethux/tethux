package topology

import (
	"fmt"
	"math"
	"strings"

	"github.com/tethux/tethux/topology/errs"
)

// Validate checks specs, identities, connections, and view references.
// Specs must be concrete values, rather than pointers. Each port can belong to one link.
func (t *Topology) Validate() error {
	if t == nil {
		return errs.New("validate topology", errs.ErrInvalidTopology, "nil topology")
	}

	if t.ID == "" {
		return errs.New("validate topology", errs.ErrInvalidTopology, "id is required")
	}

	nodes := make(map[NodeID]Node, len(t.Nodes))

	for _, node := range t.Nodes {
		if err := validateNode(node); err != nil {
			return err
		}

		if _, exists := nodes[node.ID]; exists {
			return errs.New(
				"validate topology",
				errs.ErrDuplicateID,
				fmt.Sprintf("node %q", node.ID),
			)
		}

		nodes[node.ID] = node
	}

	links := make(map[LinkID]struct{}, len(t.Links))
	connected := make(map[Endpoint]LinkID, 2*len(t.Links))

	for _, link := range t.Links {
		if _, exists := links[link.ID]; exists {
			return errs.New(
				"validate topology",
				errs.ErrDuplicateID,
				fmt.Sprintf("link %q", link.ID),
			)
		}

		if err := validateLink(nodes, &link); err != nil {
			return err
		}

		for _, endpoint := range []Endpoint{link.A, link.B} {
			if previous, exists := connected[endpoint]; exists {
				return errs.New("validate topology link", errs.ErrInvalidLink,
					fmt.Sprintf("link %q reuses endpoint %q from link %q", link.ID, endpoint, previous))
			}
			connected[endpoint] = link.ID
		}
		links[link.ID] = struct{}{}
	}

	if err := validateViews(nodes, links, t.Views); err != nil {
		return err
	}

	return nil
}

func validateNode(node Node) error {
	if node.ID == "" {
		return errs.New("validate topology node", errs.ErrInvalidNode, "id is required")
	}

	if node.Spec == nil {
		return errs.New(
			"validate topology node",
			errs.ErrInvalidNode,
			fmt.Sprintf("node %q has no spec", node.ID),
		)
	}

	switch spec := node.Spec.(type) {
	case ContainerSpec:
		if spec.Image == "" {
			return errs.New("validate topology node", errs.ErrInvalidNode,
				fmt.Sprintf("container %q requires an image", node.ID))
		}
		names := make(map[string]struct{}, len(spec.Env))
		for _, env := range spec.Env {
			if env.Name == "" || strings.ContainsAny(env.Name, "=\x00") || strings.ContainsRune(env.Value, '\x00') {
				return errs.New("validate topology node", errs.ErrInvalidNode,
					fmt.Sprintf("container %q has invalid environment variable %q", node.ID, env.Name))
			}
			if _, exists := names[env.Name]; exists {
				return errs.New("validate topology node", errs.ErrInvalidNode,
					fmt.Sprintf("container %q repeats environment variable %q", node.ID, env.Name))
			}
			names[env.Name] = struct{}{}
		}
	case DomainSpec:
		if spec.Image == "" {
			return errs.New("validate topology node", errs.ErrInvalidNode,
				fmt.Sprintf("domain %q requires an image", node.ID))
		}
	case ShitnetSpec:
	case DynamipsSpec:
		if spec.Image == "" {
			return errs.New("validate topology node", errs.ErrInvalidNode,
				fmt.Sprintf("dynamips node %q requires an image", node.ID))
		}
	default:
		return errs.New("validate topology node", errs.ErrInvalidNode,
			fmt.Sprintf("node %q has unsupported spec %T; use a concrete spec value", node.ID, node.Spec))
	}

	ports := make(map[PortID]struct{}, len(node.Ports))

	for _, port := range node.Ports {
		if port.ID == "" {
			return errs.New(
				"validate topology node",
				errs.ErrInvalidPort,
				fmt.Sprintf("node %q has port without id", node.ID),
			)
		}

		switch port.Kind {
		case PortEthernet, PortSerial:
		default:
			return errs.New(
				"validate topology node",
				errs.ErrInvalidPort,
				fmt.Sprintf("node %q port %q has unknown kind %q", node.ID, port.ID, port.Kind),
			)
		}

		if _, exists := ports[port.ID]; exists {
			return errs.New(
				"validate topology node",
				errs.ErrDuplicateID,
				fmt.Sprintf("node %q port %q", node.ID, port.ID),
			)
		}

		ports[port.ID] = struct{}{}
	}

	return nil
}

func validateLink(
	nodes map[NodeID]Node,
	link *Link,
) error {
	if link.ID == "" {
		return errs.New("validate topology link", errs.ErrInvalidLink, "id is required")
	}

	switch link.Kind {
	case LinkEthernet, LinkSerial:
	default:
		return errs.New(
			"validate topology link",
			errs.ErrInvalidLink,
			fmt.Sprintf("link %q has unknown kind %q", link.ID, link.Kind),
		)
	}

	if link.A == link.B {
		return errs.New("validate topology link", errs.ErrInvalidLink,
			fmt.Sprintf("link %q connects endpoint %q to itself", link.ID, link.A))
	}

	a, err := findPort(nodes, link.A)
	if err != nil {
		return errs.Wrap(
			"validate topology link",
			errs.ErrInvalidLink,
			fmt.Sprintf("link %q endpoint A", link.ID),
			err,
		)
	}

	b, err := findPort(nodes, link.B)
	if err != nil {
		return errs.Wrap(
			"validate topology link",
			errs.ErrInvalidLink,
			fmt.Sprintf("link %q endpoint B", link.ID),
			err,
		)
	}

	switch link.Kind {
	case LinkEthernet:
		if a.Kind != PortEthernet || b.Kind != PortEthernet {
			return errs.New(
				"validate topology link",
				errs.ErrInvalidLink,
				fmt.Sprintf("ethernet link %q requires ethernet ports", link.ID),
			)
		}

	case LinkSerial:
		if a.Kind != PortSerial || b.Kind != PortSerial {
			return errs.New(
				"validate topology link",
				errs.ErrInvalidLink,
				fmt.Sprintf("serial link %q requires serial ports", link.ID),
			)
		}
	}

	return nil
}

func findPort(
	nodes map[NodeID]Node,
	endpoint Endpoint,
) (Port, error) {
	node, exists := nodes[endpoint.Node]
	if !exists {
		return Port{}, errs.New(
			"find topology port",
			errs.ErrNotFound,
			fmt.Sprintf("node %q", endpoint.Node),
		)
	}

	for _, port := range node.Ports {
		if port.ID == endpoint.Port {
			return port, nil
		}
	}

	return Port{}, errs.New(
		"find topology port",
		errs.ErrNotFound,
		fmt.Sprintf("port %q on node %q", endpoint.Port, endpoint.Node),
	)
}

func validateViews(
	nodes map[NodeID]Node,
	links map[LinkID]struct{},
	views []View,
) error {
	if len(views) == 0 {
		return nil
	}

	ids := make(map[ViewID]struct{}, len(views))

	for _, view := range views {
		if view.ID == "" {
			return errs.New(
				"validate topology view",
				errs.ErrInvalidView,
				"id is required",
			)
		}

		if _, exists := ids[view.ID]; exists {
			return errs.New(
				"validate topology view",
				errs.ErrDuplicateID,
				fmt.Sprintf("view %q", view.ID),
			)
		}

		placedNodes := make(map[NodeID]struct{}, len(view.Nodes))
		for _, placement := range view.Nodes {
			if _, exists := nodes[placement.Node]; !exists {
				return errs.New(
					"validate topology view",
					errs.ErrInvalidView,
					fmt.Sprintf("view %q references node %q", view.ID, placement.Node),
				)
			}
			if _, exists := placedNodes[placement.Node]; exists {
				return errs.New("validate topology view", errs.ErrDuplicateID,
					fmt.Sprintf("view %q repeats node %q", view.ID, placement.Node))
			}
			if !validPosition(placement.Position) {
				return errs.New("validate topology view", errs.ErrInvalidView,
					fmt.Sprintf("view %q node %q has non-finite position", view.ID, placement.Node))
			}
			placedNodes[placement.Node] = struct{}{}
		}
		placedLinks := make(map[LinkID]struct{}, len(view.Links))
		for _, placement := range view.Links {
			if _, exists := links[placement.Link]; !exists {
				return errs.New("validate topology view", errs.ErrInvalidView,
					fmt.Sprintf("view %q references link %q", view.ID, placement.Link))
			}
			if _, exists := placedLinks[placement.Link]; exists {
				return errs.New("validate topology view", errs.ErrDuplicateID,
					fmt.Sprintf("view %q repeats link %q", view.ID, placement.Link))
			}
			for _, point := range placement.Points {
				if !validPosition(point) {
					return errs.New("validate topology view", errs.ErrInvalidView,
						fmt.Sprintf("view %q link %q has non-finite point", view.ID, placement.Link))
				}
			}
			placedLinks[placement.Link] = struct{}{}
		}

		ids[view.ID] = struct{}{}
	}

	return nil
}

func validPosition(position Position) bool {
	return !math.IsNaN(position.X) && !math.IsInf(position.X, 0) &&
		!math.IsNaN(position.Y) && !math.IsInf(position.Y, 0)
}
