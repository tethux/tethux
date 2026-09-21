package main

import (
	"bytes"
	"testing"

	topologytoml "github.com/tethux/tethux/topology/toml"
)

func TestChainTopology(t *testing.T) {
	for _, count := range []int{2, 4} {
		document, err := chainTOML(config{n: count, image: "alpine", mtu: 1500})
		if err != nil {
			t.Fatal(err)
		}
		top, err := topologytoml.Decode(bytes.NewReader(document))
		if err != nil {
			t.Fatal(err)
		}
		if err := top.Validate(); err != nil {
			t.Fatalf("%d-node chain: %v", count, err)
		}
		if len(top.Links) != count-1 {
			t.Fatalf("%d-node chain has %d links", count, len(top.Links))
		}
		for index, link := range top.Links {
			if link.A.Node != top.Nodes[index].ID || link.B.Node != top.Nodes[index+1].ID {
				t.Fatalf("link skips a node: %#v", link)
			}
		}
	}
}
