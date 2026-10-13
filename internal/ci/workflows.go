package ci

import (
	"os"
	"path/filepath"
	"time"
)

func BuiltinWorkflows(root string) []Workflow {
	image := os.Getenv("TETHUX_DYNAMIPS_IMAGE")
	if image == "" {
		image = filepath.Join(os.Getenv("HOME"), ".local", "share", "tethux", "images", "cisco", "c7200-a3jk9s-mz.124-25g.bin")
	}
	artifactDir := filepath.Join(root, "results", "current", "artifacts")
	return []Workflow{
		{Name: "mixed", Description: "all-provider Ethernet topology and lifecycle", Steps: []Step{{
			Name: "provider-matrix", Command: "go", Args: []string{"run", "./tools/topology/testing/provider-matrix", "--image", image},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 8 * time.Minute,
			Outputs:       []Output{{Name: "provider-matrix", Path: filepath.Join(artifactDir, "provider-matrix.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "provider-matrix.jsonl"),
		}}},
		{Name: "dynamips", Description: "real IOS topology, lifecycle, persistence, and CLI", Steps: []Step{{
			Name: "dynamips-topology", Command: "go", Args: []string{"run", "./tools/dynamips/testing/topology-smoke"},
			Dir: root, Timeout: 8 * time.Minute,
			Outputs:       []Output{{Name: "dynamips", Path: filepath.Join(artifactDir, "dynamips-results.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "dynamips-results.jsonl"),
		}}},
		{Name: "provider", Description: "container provider lifecycle", Steps: []Step{{
			Name: "provider", Command: "go",
			Args: []string{"run", "./cmd/tethux", "virt", "test", "--provider", "all", "--output", "json"},
			Dir:  root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
			Outputs:       []Output{{Name: "providers", Path: filepath.Join(artifactDir, "provider-results.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "provider-results.jsonl"),
		}}},
		{Name: "topology", Description: "Lua and TOML container topologies", Steps: []Step{{
			Name: "topology", Command: "go", Args: []string{"run", "./tools/bridge/example/container-udp", "--runtime", "all"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
		}}},
		{Name: "libvirt", Description: "three-domain Alpine network topology", Steps: []Step{{
			Name: "libvirt-network", Command: "go",
			Args: []string{"run", "./tools/libvirt/testing/network-smoke"},
			Dir:  root, Privilege: PrivilegeRoot, Timeout: 6 * time.Minute,
		}}},
		{Name: "bridge", Description: "exact-frame backend conformance", Steps: []Step{{
			Name: "bridge", Command: "go", Args: []string{"run", "./tools/bridge/testing/backend-smoke"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 30 * time.Minute,
			Outputs:       []Output{{Name: "bridge", Path: filepath.Join(artifactDir, "bridge-backends.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "bridge-backends.jsonl"),
		}}},
	}
}
