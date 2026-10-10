// Command container-udp verifies Lua and TOML graphs through the shared topology runner.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/local"
	topologylua "github.com/tethux/tethux/topology/lua"
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
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
	flag.StringVar(&cfg.runtime, "runtime", envDefault("RUNTIME", "podman"), "container provider: docker, podman, containerd, or all")
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

func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges are required for the topology test")
	}
	if cfg.n < 2 || cfg.n > 254 || cfg.mtu < 68 || cfg.mtu > 65535 || cfg.pingCount < 1 || cfg.pingTimeout < 1 || cfg.ifTimeout <= 0 {
		return errors.New("require 2..254 nodes, MTU 68..65535, and positive ping counts and timeouts")
	}
	if cfg.runtime != "docker" && cfg.runtime != "podman" && cfg.runtime != "containerd" && cfg.runtime != "all" {
		return errors.New("--runtime must be docker, podman, containerd, or all")
	}
	top, err := chainTopology(cfg)
	if err != nil {
		return err
	}
	pair, pairErr := tomlTopology(cfg)
	if pairErr != nil {
		return pairErr
	}
	names := []string{cfg.runtime}
	if cfg.runtime == "all" {
		names = []string{"docker", "podman", "containerd"}
	}
	for _, name := range names {
		provider, providerErr := local.SelectContainerProvider(ctx, name)
		if providerErr != nil {
			return providerErr
		}
		for _, plan := range []*topology.Topology{top, pair} {
			planLogger := logger.With("provider", name, "topology", plan.ID)
			planLogger.InfoContext(ctx, "Testing topology")
			startErr := testTopology(ctx, cfg, planLogger, provider, plan)
			if startErr != nil {
				return startErr
			}
		}
	}
	return nil
}

func testTopology(ctx context.Context, cfg config, logger *slog.Logger, provider container.ContainerProvider, top *topology.Topology) (resultErr error) {
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
		{first, fmt.Sprintf("10.77.0.%d", len(top.Nodes))}, {last, "10.77.0.1"},
	} {
		logger.InfoContext(ctx, "Testing connectivity", "source", probe.source[:12], "destination", probe.destination)
		stdout, stderr, execErr := provider.Exec(ctx, probe.source, []string{"ping", "-c", fmt.Sprint(cfg.pingCount), "-W", fmt.Sprint(cfg.pingTimeout), probe.destination}, nil, nil)
		logger.InfoContext(ctx, "Connectivity result", "stdout", string(stdout), "stderr", string(stderr))
		if execErr != nil {
			return execErr
		}
	}
	logger.InfoContext(ctx, "Topology test passed", "nodes", len(top.Nodes), "links", len(top.Links))
	return nil
}

//go:embed chain.lua
var chainScript string

func chainTopology(cfg config) (*topology.Topology, error) {
	return topologylua.Decode(strings.NewReader(chainScript), strconv.Itoa(cfg.n), cfg.image, strconv.Itoa(cfg.mtu))
}

//go:embed pair.toml
var pairDocument string

func tomlTopology(cfg config) (*topology.Topology, error) {
	top, err := topologytoml.Decode(strings.NewReader(pairDocument))
	if err != nil {
		return nil, err
	}
	for index := range top.Nodes {
		spec, ok := top.Nodes[index].Spec.(topology.ContainerSpec)
		if !ok {
			return nil, errors.New("TOML topology fixture requires container nodes")
		}
		spec.Image = cfg.image
		top.Nodes[index].Spec = spec
	}
	err = top.Validate()
	if err != nil {
		return nil, err
	}
	return top, nil
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
