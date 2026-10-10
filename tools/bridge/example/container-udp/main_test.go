package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tethux/tethux/topology"
)

func TestChainTopology(t *testing.T) {
	for _, count := range []int{2, 4, 254} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			image := "registry.example/alpine:latest"
			top, err := chainTopology(config{n: count, image: image, mtu: 1400})
			if err != nil {
				t.Fatal(err)
			}
			if len(top.Nodes) != count || len(top.Links) != count-1 {
				t.Fatalf("unexpected chain: %d nodes, %d links", len(top.Nodes), len(top.Links))
			}
			for index, node := range top.Nodes {
				spec, ok := node.Spec.(topology.ContainerSpec)
				if !ok || spec.Image != image || len(spec.Command) != 3 || spec.Command[0] != "sh" || spec.Command[1] != "-ec" {
					t.Fatalf("unexpected workload: %#v", node.Spec)
				}
				bridge, ports := 0, 1
				if index > 0 && index < count-1 {
					bridge, ports = 1, 2
				}
				setup := fmt.Sprintf("ADDRESS=10.77.0.%d/24\nBRIDGE=%d\n", index+1, bridge)
				if !strings.HasPrefix(spec.Command[2], setup) || len(node.Ports) != ports {
					t.Fatalf("unexpected node setup: %#v", node)
				}
			}
			for index, link := range top.Links {
				leftPort := topology.PortID("eth1")
				if index == 0 {
					leftPort = "eth0"
				}
				wantA := topology.Endpoint{Node: top.Nodes[index].ID, Port: leftPort}
				wantB := topology.Endpoint{Node: top.Nodes[index+1].ID, Port: "eth0"}
				if link.A != wantA || link.B != wantB || link.Kind != topology.LinkEthernet || link.MTU != 1400 {
					t.Fatalf("unexpected link: %#v", link)
				}
			}
		})
	}
}

func TestTOMLTopology(t *testing.T) {
	image := "registry.example/alpine:latest"
	top, err := tomlTopology(config{image: image})
	if err != nil {
		t.Fatal(err)
	}
	if top.ID != "container-udp-toml-test" || len(top.Nodes) != 2 || len(top.Links) != 1 {
		t.Fatalf("unexpected TOML topology: %#v", top)
	}
	for index, node := range top.Nodes {
		spec, ok := node.Spec.(topology.ContainerSpec)
		if !ok || spec.Image != image || len(spec.Command) != 3 {
			t.Fatalf("unexpected TOML workload: %#v", node.Spec)
		}
		if !strings.Contains(spec.Command[2], fmt.Sprintf("ip addr add 10.77.0.%d/24 dev eth0", index+1)) {
			t.Fatalf("unexpected guest setup: %q", spec.Command[2])
		}
	}
	want := topology.Link{
		ID: "lan", Kind: topology.LinkEthernet, MTU: 1500,
		A: topology.Endpoint{Node: "node-1", Port: "eth0"},
		B: topology.Endpoint{Node: "node-2", Port: "eth0"},
	}
	if top.Links[0] != want {
		t.Fatalf("unexpected TOML link: %#v", top.Links[0])
	}
}
