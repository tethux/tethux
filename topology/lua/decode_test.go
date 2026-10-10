package lua_test

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/lua"
	glua "github.com/yuin/gopher-lua"
)

func TestDecodeExamples(t *testing.T) {
	for _, name := range []string{"pair", "ring"} {
		t.Run(name, func(t *testing.T) {
			file, err := openExample(name)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				closeErr := file.Close()
				if closeErr != nil {
					t.Error(closeErr)
				}
			})
			top, decodeErr := lua.Decode(file)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if name == "pair" {
				if top.ID != "pair" || len(top.Nodes) != 2 || len(top.Links) != 1 {
					t.Fatalf("unexpected pair: %#v", top)
				}
				want := topology.Link{
					ID: "link-1", Kind: topology.LinkEthernet,
					A: topology.Endpoint{Node: "a", Port: "eth0"}, B: topology.Endpoint{Node: "b", Port: "eth0"},
				}
				if top.Links[0] != want {
					t.Fatalf("link: %#v, want %#v", top.Links[0], want)
				}
				return
			}
			if top.ID != "ring-6" || len(top.Nodes) != 6 || len(top.Links) != 6 {
				t.Fatalf("unexpected ring: %#v", top)
			}
			for i, node := range top.Nodes {
				id := topology.NodeID(fmt.Sprintf("r%d", i+1))
				if node.ID != id || len(node.Ports) != 2 || !reflect.DeepEqual(node.Spec, topology.ContainerSpec{Image: "alpine:latest", Command: []string{"sleep", "infinity"}}) {
					t.Fatalf("unexpected node: %#v", node)
				}
				want := topology.Link{
					ID: topology.LinkID(fmt.Sprintf("link-%d", i+1)), Kind: topology.LinkEthernet,
					A: topology.Endpoint{Node: id, Port: "eth1"},
					B: topology.Endpoint{Node: topology.NodeID(fmt.Sprintf("r%d", (i+1)%6+1)), Port: "eth0"},
				}
				if top.Links[i] != want {
					t.Fatalf("link: %#v, want %#v", top.Links[i], want)
				}
			}
		})
	}
}

func openExample(name string) (*os.File, error) {
	switch name {
	case "pair":
		return os.Open("../examples/pair.lua")
	case "ring":
		return os.Open("../examples/ring.lua")
	default:
		return nil, errs.New("open Lua example", errs.ErrNotFound, name)
	}
}

func TestDecodePortReuseAndCaughtFailure(t *testing.T) {
	input := `local tx = require("tethux")
local net = tx.topology("lab")
local a = net:node("a", tx.Kind.Container, "alpine")
local b = net:node("b", tx.Kind.Container, "alpine")
local c = net:node("c", tx.Kind.Container, "alpine")
local p = a:port("s0", tx.Medium.Serial)
local q = b:port("s0", tx.Medium.Serial)
assert(not pcall(function() net:link(p, c:port("eth0")) end))
net:link(a:port("s0"), q)
assert(not pcall(function() net:link(p, c:port("s0", tx.Medium.Serial)) end))
return net`
	top, err := lua.Decode(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(top.Nodes[0].Ports) != 1 || len(top.Links) != 1 || top.Links[0].ID != "link-1" || top.Links[0].Kind != topology.LinkSerial {
		t.Fatalf("unexpected topology: %#v", top)
	}
	second, secondErr := lua.Decode(strings.NewReader(input))
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	if !reflect.DeepEqual(top, second) {
		t.Fatal("decoding the same script changed the topology")
	}
}

func TestDecodeRejectsInvalidScripts(t *testing.T) {
	prefix := `local tx = require("tethux")
local net = tx.topology("lab")
local a = net:node("a", tx.Kind.Container, "alpine")
local b = net:node("b", tx.Kind.Container, "alpine")
`
	cases := []struct {
		name     string
		input    string
		category error
	}{
		{"syntax", "local =", nil},
		{"runtime", `error("broken")`, nil},
		{"missing return", prefix, nil},
		{"table return", "return {}", nil},
		{"node return", prefix + "return a", nil},
		{"empty topology ID", `return require("tethux").topology("")`, errs.ErrInvalidTopology},
		{"empty node ID", prefix + `net:node("", tx.Kind.Container, "alpine") return net`, errs.ErrInvalidNode},
		{"empty image", prefix + `net:node("c", tx.Kind.Container, "") return net`, errs.ErrInvalidNode},
		{"duplicate node", prefix + `net:node("a", tx.Kind.Container, "alpine") return net`, errs.ErrDuplicateID},
		{"unsupported kind", prefix + `net:node("c", "domain", "disk") return net`, errs.ErrUnsupported},
		{"empty port", prefix + `a:port("") return net`, errs.ErrInvalidPort},
		{"unknown medium", prefix + `a:port("eth0", "wifi") return net`, errs.ErrInvalidPort},
		{"conflicting medium", prefix + `a:port("eth0") a:port("eth0", tx.Medium.Serial) return net`, errs.ErrInvalidPort},
		{"different media", prefix + `net:link(a:port("eth0"), b:port("s0", tx.Medium.Serial)) return net`, errs.ErrInvalidLink},
		{"self link", prefix + `net:link(a:port("eth0"), a:port("eth0")) return net`, errs.ErrInvalidLink},
		{"negative MTU", prefix + `net:link(a:port("eth0"), b:port("eth0"), -1) return net`, errs.ErrInvalidLink},
		{"overflow MTU", prefix + `net:link(a:port("eth0"), b:port("eth0"), 65536) return net`, errs.ErrInvalidLink},
		{"fractional MTU", prefix + `net:link(a:port("eth0"), b:port("eth0"), 1400.5) return net`, errs.ErrInvalidLink},
		{"used port", prefix + `net:link(a:port("eth0"), b:port("eth0")) net:link(a:port("eth0"), b:port("eth1")) return net`, errs.ErrInvalidLink},
		{"foreign port", prefix + `local other = tx.topology("other") local c = other:node("c", tx.Kind.Container, "alpine") net:link(a:port("eth0"), c:port("eth0")) return net`, errs.ErrInvalidLink},
		{"wrong receiver", prefix + `net.node(a, "c", tx.Kind.Container, "alpine") return net`, nil},
		{"table endpoint", prefix + `net:link({}, b:port("eth0")) return net`, nil},
		{"invalid command", prefix + `net:node("c", tx.Kind.Container, "alpine", {"sleep", 42}) return net`, nil},
		{"named command", prefix + `net:node("c", tx.Kind.Container, "alpine", {cmd = "sleep"}) return net`, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			top, err := lua.Decode(strings.NewReader(test.input))
			if top != nil || !errors.Is(err, errs.ErrDecode) {
				t.Fatalf("got %v, %v", top, err)
			}
			if test.category != nil && !errors.Is(err, test.category) {
				t.Fatalf("got %v, want %v", err, test.category)
			}
		})
	}
}

func TestDecodePreservesLuaError(t *testing.T) {
	_, err := lua.Decode(strings.NewReader(`error("broken")`))
	var apiError *glua.ApiError
	if !errors.As(err, &apiError) {
		t.Fatalf("Lua cause missing: %v", err)
	}
}

func TestDecodeNilReader(t *testing.T) {
	top, err := lua.Decode(nil)
	if top != nil || !errors.Is(err, errs.ErrDecode) {
		t.Fatalf("got %v, %v", top, err)
	}
}
