package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTunnelConfigured(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env")
	configured, err := tunnelConfigured(path)
	if err != nil || configured {
		t.Fatalf("missing file: configured=%v err=%v", configured, err)
	}

	writeErr := os.WriteFile(path, []byte("OTHER=value\nTUNNEL_TOKEN=short\n"), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	configured, err = tunnelConfigured(path)
	if err != nil || configured {
		t.Fatalf("short token: configured=%v err=%v", configured, err)
	}

	writeErr = os.WriteFile(path, []byte("TUNNEL_TOKEN=abcdefghijklmnopqrstuvwxyz\n"), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	configured, err = tunnelConfigured(path)
	if err != nil || !configured {
		t.Fatalf("valid token: configured=%v err=%v", configured, err)
	}
}

func TestViewerComposeKeepsStableContainerNames(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("..", "ci-results", "viewer", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(content)
	for _, name := range []string{
		"container_name: tethux-ci-viewer\n",
		"container_name: tethux-ci-viewer-ingest\n",
		"container_name: tethux-ci-viewer-tunnel\n",
		"name: ${TETHUX_VIEWER_NETWORK:-tethux-ci-viewer}",
	} {
		if !strings.Contains(compose, name) {
			t.Errorf("compose file does not contain %q", name)
		}
	}
}
