package ci

import (
	"path/filepath"
	"time"
)

func BuiltinWorkflows(root string) []Workflow {
	artifactDir := filepath.Join(root, "results", "current", "artifacts")
	return []Workflow{
		{Name: "provider", Description: "container provider lifecycle", Steps: []Step{{
			Name: "provider", Command: "go",
			Args: []string{"run", "./cmd/tethux", "virt", "test", "--provider", "all", "--output", "json"},
			Dir:  root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
			Outputs:       []Output{{Name: "providers", Path: filepath.Join(artifactDir, "provider-results.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "provider-results.jsonl"),
		}}},
		{Name: "topology", Description: "container topology", Steps: []Step{{
			Name: "topology", Command: "go", Args: []string{"run", "./tools/bridge/example/container-udp", "--runtime", "all"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 45 * time.Minute,
		}}},
		{Name: "bridge", Description: "exact-frame backend conformance", Steps: []Step{{
			Name: "bridge", Command: "go", Args: []string{"run", "./tools/bridge/testing/backend-smoke"},
			Dir: root, Privilege: PrivilegeRoot, Timeout: 30 * time.Minute,
			Outputs:       []Output{{Name: "bridge", Path: filepath.Join(artifactDir, "bridge-backends.jsonl"), Kind: "application/x-ndjson"}},
			CaptureStdout: filepath.Join(artifactDir, "bridge-backends.jsonl"),
		}}},
	}
}
