package toml_test

import (
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	gotoml "github.com/pelletier/go-toml/v2"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/toml"
)

func TestDecode(t *testing.T) {
	file, err := os.Open("testdata/lab.toml")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	top, err := toml.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if top.ID != "lab" || len(top.Nodes) != 4 || len(top.Links) != 2 || len(top.Views) != 1 {
		t.Fatalf("unexpected topology: %#v", top)
	}
	wantSpecs := []topology.NodeSpec{
		topology.ContainerSpec{Image: "alpine:3.23", Command: []string{"sleep", "infinity"}, Env: []topology.Env{{Name: "ROLE", Value: "client"}}},
		topology.DomainSpec{Image: "alpine.qcow2", CPUs: 2, MemoryMB: 512},
		topology.DynamipsSpec{Image: "router.bin", RAMMB: 256},
		topology.ShitnetSpec{},
	}
	for index, want := range wantSpecs {
		if !reflect.DeepEqual(top.Nodes[index].Spec, want) {
			t.Fatalf("node %d spec: %#v, want %#v", index, top.Nodes[index].Spec, want)
		}
	}
	if top.Nodes[0].Name != "Client" || top.Nodes[0].Ports[0] != (topology.Port{ID: "eth0", Kind: topology.PortEthernet}) {
		t.Fatalf("unexpected node: %#v", top.Nodes[0])
	}
	wantLink := topology.Link{ID: "lan", Kind: topology.LinkEthernet, MTU: 1500, A: topology.Endpoint{Node: "client", Port: "eth0"}, B: topology.Endpoint{Node: "server", Port: "eth0"}}
	if top.Links[0] != wantLink {
		t.Fatalf("link: %#v", top.Links[0])
	}
	wantView := topology.View{ID: "main", Name: "Overview", Nodes: []topology.NodeView{{Node: "client", Position: topology.Position{X: 10.5, Y: -20}}}, Links: []topology.LinkView{{Link: "lan", Points: []topology.Position{{X: 20, Y: 30}}}}}
	if !reflect.DeepEqual(top.Views[0], wantView) {
		t.Fatalf("view: %#v", top.Views[0])
	}
}

func TestDecodeRejectsInvalidDocuments(t *testing.T) {
	cases := []struct {
		name, input string
		category    error
	}{
		{"syntax", "id = [", nil},
		{"unknown field", "id = 'lab'\nunknown = true", nil},
		{"missing ID", "", errs.ErrInvalidTopology},
		{"missing spec", "id = 'lab'\n[[nodes]]\nid = 'a'", errs.ErrInvalidNode},
		{"multiple specs", "id = 'lab'\n[[nodes]]\nid = 'a'\n[nodes.container]\nimage = 'alpine'\n[nodes.domain]\nimage = 'disk'", errs.ErrInvalidNode},
		{"unknown spec field", "id = 'lab'\n[[nodes]]\nid = 'a'\n[nodes.domain]\nimage = 'disk'\nmemroy_mb = 512", nil},
		{"negative memory", "id = 'lab'\n[[nodes]]\nid = 'a'\n[nodes.domain]\nimage = 'disk'\nmemory_mb = -1", nil},
		{"overflow CPU", "id = 'lab'\n[[nodes]]\nid = 'a'\n[nodes.domain]\nimage = 'disk'\ncpus = 65536", nil},
		{"invalid link", "id = 'lab'\n[[links]]\nid = 'ab'\nkind = 'ethernet'\na = {node = 'missing', port = 'eth0'}\nb = {node = 'other', port = 'eth0'}", errs.ErrNotFound},
		{"invalid view", "id = 'lab'\n[[views]]\nid = 'main'\nlinks = [{link = 'missing'}]", errs.ErrInvalidView},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			top, err := toml.Decode(strings.NewReader(test.input))
			if top != nil || !errors.Is(err, errs.ErrDecode) {
				t.Fatalf("got %v, %v", top, err)
			}
			if test.category != nil && !errors.Is(err, test.category) {
				t.Fatalf("got %v, want category %v", err, test.category)
			}
		})
	}
}

func TestDecodePreservesParserError(t *testing.T) {
	_, err := toml.Decode(strings.NewReader("id = ["))
	var parserError *gotoml.DecodeError
	if !errors.As(err, &parserError) {
		t.Fatalf("parser cause lost: %v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestDecodeReaderErrors(t *testing.T) {
	_, err := toml.Decode(failingReader{})
	if !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, errs.ErrDecode) {
		t.Fatalf("reader cause lost: %v", err)
	}
	_, err = toml.Decode(nil)
	if !errors.Is(err, errs.ErrDecode) {
		t.Fatalf("nil reader: %v", err)
	}
}
