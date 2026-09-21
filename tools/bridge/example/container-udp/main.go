// Command container-udp verifies a chain through the shared topology runner.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gotoml "github.com/pelletier/go-toml/v2"
	"github.com/tethux/tethux/topology/local"
	topologytoml "github.com/tethux/tethux/topology/toml"
	"github.com/tethux/tethux/virt/container"
)

type config struct {
	runtime     string
	n           int
	basePort    int
	image       string
	mtu         int
	ifTimeout   time.Duration
	pingCount   int
	pingTimeout int
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := run(ctx, parseFlags(), logger)
	if err != nil {
		logger.Error("Topology test failed", "error", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.runtime, "runtime", envDefault("RUNTIME", "podman"), "container provider: docker or podman")
	flag.IntVar(&cfg.n, "n", 4, "container count (2..254)")
	flag.IntVar(&cfg.basePort, "base-port", 23000, "first loopback UDP port")
	flag.StringVar(&cfg.image, "image", envDefault("IMAGE", "127.0.0.1:5000/tethux/fixture-a:1"), "image with ip, bridge support, and ping")
	flag.IntVar(&cfg.mtu, "mtu", 1500, "link MTU")
	flag.DurationVar(&cfg.ifTimeout, "interface-timeout", 15*time.Second, "guest network setup timeout")
	flag.IntVar(&cfg.pingCount, "ping-count", 2, "ping packet count")
	flag.IntVar(&cfg.pingTimeout, "ping-timeout", 1, "ping timeout in seconds")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config, logger *slog.Logger) (resultErr error) {
	if os.Geteuid() != 0 {
		return errors.New("root privileges are required for the topology test")
	}
	if cfg.n < 2 || cfg.n > 254 || cfg.mtu < 68 || cfg.mtu > 65535 || cfg.pingCount < 1 || cfg.pingTimeout < 1 || cfg.ifTimeout <= 0 {
		return errors.New("require 2..254 nodes, MTU 68..65535, and positive ping counts and timeouts")
	}
	if cfg.runtime != "docker" && cfg.runtime != "podman" {
		return errors.New("--runtime must be docker or podman")
	}
	provider, err := local.SelectContainerProvider(ctx, cfg.runtime)
	if err != nil {
		return err
	}
	document, err := chainTOML(cfg)
	if err != nil {
		return err
	}
	err = printTOML(os.Stdout, document, os.Getenv("NO_COLOR") == "")
	if err != nil {
		return err
	}
	top, err := topologytoml.Decode(bytes.NewReader(document))
	if err != nil {
		return err
	}
	running, err := local.Start(ctx, top, local.Options{Provider: provider, BasePort: cfg.basePort, Logger: logger})
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, running.Close(cleanupCtx))
	}()
	first, err := running.ContainerID(top.Nodes[0].ID)
	if err != nil {
		return err
	}
	last, err := running.ContainerID(top.Nodes[len(top.Nodes)-1].ID)
	if err != nil {
		return err
	}
	setupCtx, cancel := context.WithTimeout(ctx, cfg.ifTimeout)
	defer cancel()
	for index, node := range top.Nodes {
		id, lookupErr := running.ContainerID(node.ID)
		if lookupErr != nil {
			return lookupErr
		}
		err = waitForAddress(setupCtx, provider, id, fmt.Sprintf("10.77.0.%d/24", index+1))
		if err != nil {
			return err
		}
	}
	for _, probe := range []struct{ source, destination string }{
		{first, fmt.Sprintf("10.77.0.%d", cfg.n)}, {last, "10.77.0.1"},
	} {
		logger.InfoContext(ctx, "Testing connectivity", "source", probe.source[:12], "destination", probe.destination)
		command := exec.CommandContext(ctx, cfg.runtime, "exec", probe.source, "ping", "-c", fmt.Sprint(cfg.pingCount), "-W", fmt.Sprint(cfg.pingTimeout), probe.destination)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		err = command.Run()
		if err != nil {
			return err
		}
	}
	logger.InfoContext(ctx, "Topology test passed", "nodes", len(top.Nodes), "links", len(top.Links))
	return nil
}

const guestSetup = `until ip link show eth0 >/dev/null 2>&1; do sleep 0.1; done
interface=eth0
if [ "$BRIDGE" = 1 ]; then
 until ip link show eth1 >/dev/null 2>&1; do sleep 0.1; done
 ip link add br0 type bridge
 ip link set eth0 master br0
 ip link set eth1 master br0
 ip link set br0 up
 interface=br0
fi
ip addr add "$ADDRESS" dev "$interface"
exec sleep infinity`

func chainTOML(cfg config) ([]byte, error) {
	document := chainDocument{ID: "container-udp-test", Nodes: make([]chainNode, 0, cfg.n), Links: make([]chainLink, 0, cfg.n-1)}
	for index := 0; index < cfg.n; index++ {
		ports := []chainPort{{ID: "eth0", Kind: "ethernet"}}
		bridge := "0"
		if index > 0 && index < cfg.n-1 {
			ports = append(ports, chainPort{ID: "eth1", Kind: "ethernet"})
			bridge = "1"
		}
		document.Nodes = append(document.Nodes, chainNode{
			ID: fmt.Sprintf("node-%d", index+1), Ports: ports,
			Container: chainContainer{Image: cfg.image, Command: []string{"sh", "-ec", guestSetup}, Env: []chainEnv{
				{Name: "ADDRESS", Value: fmt.Sprintf("10.77.0.%d/24", index+1)}, {Name: "BRIDGE", Value: bridge},
			}},
		})
		if index == 0 {
			continue
		}
		leftPort := "eth1"
		if index == 1 {
			leftPort = "eth0"
		}
		document.Links = append(document.Links, chainLink{
			ID: fmt.Sprintf("link-%d", index), Kind: "ethernet", MTU: cfg.mtu,
			A: chainEndpoint{Node: document.Nodes[index-1].ID, Port: leftPort},
			B: chainEndpoint{Node: document.Nodes[index].ID, Port: "eth0"},
		})
	}
	return gotoml.Marshal(document)
}

type chainDocument struct {
	ID    string      `toml:"id"`
	Nodes []chainNode `toml:"nodes"`
	Links []chainLink `toml:"links"`
}

type chainNode struct {
	ID        string         `toml:"id"`
	Ports     []chainPort    `toml:"ports,inline"`
	Container chainContainer `toml:"container"`
}

type chainPort struct {
	ID   string `toml:"id"`
	Kind string `toml:"kind"`
}

type chainContainer struct {
	Image   string     `toml:"image"`
	Command []string   `toml:"command,multiline"`
	Env     []chainEnv `toml:"env,inline"`
}

type chainEnv struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

type chainLink struct {
	ID   string        `toml:"id"`
	Kind string        `toml:"kind"`
	MTU  int           `toml:"mtu"`
	A    chainEndpoint `toml:"a,inline"`
	B    chainEndpoint `toml:"b,inline"`
}

type chainEndpoint struct {
	Node string `toml:"node"`
	Port string `toml:"port"`
}

func waitForAddress(ctx context.Context, provider container.ContainerProvider, id, address string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		stdout, _, err := provider.Exec(ctx, id, []string{"ip", "-o", "-4", "addr", "show"}, nil, nil)
		if err != nil {
			return err
		}
		if strings.Contains(string(stdout), "inet "+address+" ") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s in container %s: %w", address, id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
