package topology

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tethux/tethux/topology/errs"
)

func TestReadTopologyFormats(t *testing.T) {
	cases := []struct {
		path         string
		id           string
		nodes, links int
	}{
		{"../../topology/examples/ring.lua", "ring-6", 6, 6},
		{"../../topology/examples/pair.lua", "pair", 2, 1},
		{"../../topology/examples/container-chain.toml", "container-chain", 3, 2},
	}
	for _, test := range cases {
		t.Run(test.id, func(t *testing.T) {
			top, err := readTopology(test.path)
			if err != nil {
				t.Fatal(err)
			}
			if string(top.ID) != test.id || len(top.Nodes) != test.nodes || len(top.Links) != test.links {
				t.Fatalf("unexpected topology: %#v", top)
			}
		})
	}
}

func TestRunRejectsInvalidLuaBeforeStarting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.LUA")
	err := os.WriteFile(path, []byte(`local tx = require("tethux"); local net = tx.topology("bad"); net:node("a", tx.Kind.Container, ""); return net`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	command := NewRootCmd()
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"run", path})
	err = command.Execute()
	if !errors.Is(err, errs.ErrDecode) || !errors.Is(err, errs.ErrInvalidNode) || !strings.Contains(err.Error(), path) {
		t.Fatalf("unexpected error: %v", err)
	}
}
