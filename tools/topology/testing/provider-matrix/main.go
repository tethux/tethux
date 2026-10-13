// Command provider-matrix verifies an Ethernet path through every local provider.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	native "github.com/tethux/dynamips-downgrade"
	"github.com/tethux/dynamips-downgrade/console"
	"github.com/tethux/tethux/internal/cleanup"
	"github.com/tethux/tethux/internal/integration/ios"
	"github.com/tethux/tethux/topology"
	"github.com/tethux/tethux/topology/errs"
	"github.com/tethux/tethux/topology/local"
	topologytoml "github.com/tethux/tethux/topology/toml"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/container"
	"github.com/tethux/tethux/virt/emulator/dynamips"
	"github.com/tethux/tethux/virt/emulator/shitnet"
	libvirtprovider "github.com/tethux/tethux/virt/hypervisor/libvirt"
)

type report struct {
	Name    string   `json:"name"`
	Passed  bool     `json:"passed"`
	Checks  []string `json:"checks"`
	Seconds float64  `json:"seconds"`
	Error   string   `json:"error,omitempty"`
}

func main() {
	if boundaryErr := mainCommand(); boundaryErr != nil {
		fmt.Fprintln(os.Stderr, boundaryErr)
		os.Exit(1)
	}
}

func mainCommand() error {
	image := flag.String("image", "", "local C7200 IOS image")
	binary := flag.String("tethux", "", "built tethux CLI")
	example := flag.String("example", "topology/examples/provider-matrix.toml", "all-provider topology")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 7*time.Minute)
	defer cancel()
	result := report{Name: "provider-matrix"}
	started := time.Now()
	runErr := run(ctx, *binary, *example, *image, &result)
	result.Passed = runErr == nil
	result.Seconds = time.Since(started).Seconds()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	return errors.Join(runErr, json.NewEncoder(os.Stdout).Encode(result))
}

func run(ctx context.Context, binary, example, image string, result *report) (resultErr error) {
	if binary == "" {
		return errs.New("test provider matrix", errs.ErrInvalidNode, "--tethux is required")
	}
	if image == "" {
		image = os.Getenv("TETHUX_DYNAMIPS_IMAGE")
	}
	if image == "" {
		image = filepath.Join(os.Getenv("HOME"), ".local", "share", "tethux", "images", "cisco", "c7200-a3jk9s-mz.124-25g.bin")
	}
	domainImage := os.Getenv("TETHUX_LIBVIRT_IMAGE")
	if domainImage == "" {
		return errs.New("test provider matrix", errs.ErrInvalidNode, "TETHUX_LIBVIRT_IMAGE is required")
	}
	for _, path := range []*string{&binary, &example, &image, &domainImage} {
		absolute, pathErr := filepath.Abs(*path)
		if pathErr != nil {
			return pathErr
		}
		*path = absolute
	}
	directory, directoryErr := os.MkdirTemp("/var/tmp", "tethux-matrix-")
	if directoryErr != nil {
		return directoryErr
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(directory)) }()
	if chmodErr := os.Chmod(directory, 0o755); chmodErr != nil {
		return chmodErr
	} // #nosec G302 -- QEMU must traverse the non-secret fixture directory.
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return cwdErr
	}
	if chdirErr := os.Chdir(directory); chdirErr != nil {
		return chdirErr
	}
	defer func() { resultErr = errors.Join(resultErr, os.Chdir(cwd)) }()
	seed, seedErr := prepareSeed(ctx, directory)
	if seedErr != nil {
		return seedErr
	}
	file, openErr := os.Open(example) // #nosec G304 -- Explicit integration fixture.
	if openErr != nil {
		return openErr
	}
	top, decodeErr := topologytoml.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		return errors.Join(decodeErr, closeErr)
	}
	fmt.Fprintln(os.Stderr, "Provider matrix: native topology and lifecycle")
	if nativeErr := runNative(ctx, top, directory, image, domainImage, seed, result); nativeErr != nil {
		return nativeErr
	}
	fmt.Fprintln(os.Stderr, "Provider matrix: CLI topology")
	if cliErr := runCLI(ctx, binary, example, directory, image, domainImage, seed); cliErr != nil {
		return cliErr
	}
	result.Checks = append(result.Checks, "CLI all-provider topology, IOS-to-Shitnet ping, SIGINT cleanup")
	return nil
}

func prepareSeed(ctx context.Context, directory string) (string, error) {
	files := map[string]string{
		"user-data": `#!/bin/sh
set -eu
modprobe bridge
brctl addbr br0
brctl setfd br0 0
brctl stp br0 off
brctl addif br0 eth0
brctl addif br0 eth1
ip link set eth0 up
ip link set eth1 up
ip link set br0 up
`,
		"meta-data": `instance-id: provider-matrix
hostname: provider-matrix
network-interfaces: |
  auto lo
  iface lo inet loopback
  auto eth0
  iface eth0 inet manual
  auto eth1
  iface eth1 inet manual
`,
	}
	for name, contents := range files {
		if writeErr := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o644); writeErr != nil {
			return "", writeErr
		} // #nosec G306 -- Non-secret cloud-init fixture must be readable by the ISO builder.
	}
	seed := filepath.Join(directory, "seed.iso")
	command := exec.CommandContext(ctx, "xorrisofs", "-quiet", "-output", seed, "-volid", "cidata", "-joliet", "-rock", filepath.Join(directory, "user-data"), filepath.Join(directory, "meta-data")) // #nosec G204 -- Fixed builder with owned fixture paths.
	if output, commandErr := command.CombinedOutput(); commandErr != nil {
		return "", errs.Wrap("build guest seed", errs.ErrStart, string(output), commandErr)
	}
	return seed, nil
}

func runNative(ctx context.Context, top *topology.Topology, directory, image, domainImage, seed string, result *report) (resultErr error) {
	providers := make(map[string]container.ContainerProvider)
	for _, name := range []string{"docker", "podman", "containerd"} {
		provider, providerErr := local.SelectContainerProvider(ctx, name)
		if providerErr != nil {
			return providerErr
		}
		providers[name] = provider
	}
	hypervisor, hypervisorErr := libvirtprovider.New("qemu:///system")
	if hypervisorErr != nil {
		return hypervisorErr
	}
	defer func() { resultErr = errors.Join(resultErr, hypervisor.Close()) }()
	runtime, runtimeErr := native.New()
	if runtimeErr != nil {
		return runtimeErr
	}
	routers := &dynamips.Adapter{Runtime: runtime}
	defer func() { resultErr = errors.Join(resultErr, routers.Close(), runtime.Close()) }()
	endpoints := &shitnet.Adapter{}
	defer func() { resultErr = errors.Join(resultErr, endpoints.Close()) }()
	running, startErr := local.Start(ctx, top, local.Options{
		Containers: providers, Domains: &local.DomainOptions{Provider: hypervisor, Image: domainImage, SeedImage: seed},
		Dynamips: &local.DynamipsOptions{Provider: routers, StateRoot: directory, Image: image, ConsoleBasePort: 28200},
		Shitnet:  endpoints, BasePort: 27200,
	})
	if running != nil {
		defer func() {
			cleanupCtx, cancel := cleanup.NewContext(ctx)
			defer cancel()
			resultErr = errors.Join(resultErr, running.Close(cleanupCtx))
		}()
	}
	if startErr != nil {
		return startErr
	}
	for _, node := range top.Nodes {
		snapshot, reloadErr := running.Node(ctx, node.ID)
		if reloadErr != nil {
			return reloadErr
		}
		if snapshot.State != virt.NodeRunning {
			return errs.New("check provider state", errs.ErrStart, string(node.ID))
		}
		if spec, isContainer := node.Spec.(topology.ContainerSpec); isContainer {
			provider, providerErr := running.ContainerProvider(node.ID)
			if providerErr != nil {
				return providerErr
			}
			if provider.Info().Name != spec.Provider {
				return errs.New("check provider selection", errs.ErrUnsupported, spec.Provider)
			}
		}
	}
	connections, consoleErr := ios.Ready(ctx, []int{28200})
	if consoleErr != nil {
		return consoleErr
	}
	defer func() { resultErr = errors.Join(resultErr, ios.Close(connections)) }()
	if pingErr := pingNative(ctx, endpoints); pingErr != nil {
		return pingErr
	}
	if pingErr := pingIOS(ctx, connections[0]); pingErr != nil {
		return pingErr
	}
	result.Checks = append(result.Checks, "Docker, Podman, containerd, libvirt, Dynamips, Shitnet running", "bidirectional IOS/Shitnet ICMP through all providers")
	vm, nodeErr := running.Node(ctx, "vm")
	if nodeErr != nil {
		return nodeErr
	}
	if suspendErr := hypervisor.Suspend(ctx, vm.ID); suspendErr != nil {
		return suspendErr
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	probe, probeErr := endpoints.Ping(probeCtx, "pc", netip.MustParseAddr("10.77.0.1"), 1)
	cancel()
	if !errors.Is(probeErr, context.DeadlineExceeded) || probe.Received != 0 {
		return errs.Wrap("probe suspended guest", errs.ErrStart, "traffic must stop at the guest", probeErr)
	}
	if resumeErr := hypervisor.Resume(ctx, vm.ID); resumeErr != nil {
		return resumeErr
	}
	if pingErr := pingNative(ctx, endpoints); pingErr != nil {
		return pingErr
	}
	if restartErr := endpoints.Restart(ctx, "pc"); restartErr != nil {
		return restartErr
	}
	if pingErr := pingNative(ctx, endpoints); pingErr != nil {
		return pingErr
	}
	if pingErr := pingIOS(ctx, connections[0]); pingErr != nil {
		return pingErr
	}
	result.Checks = append(result.Checks, "guest suspend breaks path; resume restores it", "Shitnet restart restores bidirectional traffic")
	return nil
}

func pingNative(ctx context.Context, endpoints *shitnet.Adapter) error {
	readyCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		probeCtx, probeCancel := context.WithTimeout(readyCtx, 4*time.Second)
		result, probeErr := endpoints.Ping(probeCtx, "pc", netip.MustParseAddr("10.77.0.1"), 3)
		probeCancel()
		if probeErr == nil && result.Received == 3 {
			return nil
		}
		if readyCtx.Err() != nil {
			return errors.Join(readyCtx.Err(), probeErr)
		}
		if !errors.Is(probeErr, context.DeadlineExceeded) {
			return errs.Wrap("ping IOS", errs.ErrStart, "native probe", probeErr)
		}
	}
}

func pingIOS(ctx context.Context, connection *console.Connection) error {
	readyCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for readyCtx.Err() == nil {
		output, commandErr := ios.Command(readyCtx, connection, "ping 10.77.0.2 repeat 3 timeout 1")
		if commandErr != nil {
			return commandErr
		}
		if strings.Contains(output, "Success rate is 100 percent") {
			return nil
		}
	}
	return errs.Wrap("ping Shitnet", errs.ErrStart, "IOS probe", readyCtx.Err())
}

func runCLI(ctx context.Context, binary, example, directory, image, domainImage, seed string) (resultErr error) {
	command := exec.CommandContext(ctx, binary, "topology", "run", example, "--dynamips-image", image, "--domain-image", domainImage, "--domain-seed-image", seed, "--state-root", directory, "--base-port", "27200", "--console-base-port", "28200") // #nosec G204 -- Built project binary and explicit fixture paths.
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if startErr := command.Start(); startErr != nil {
		return startErr
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		if signalErr := command.Process.Signal(os.Interrupt); signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
			resultErr = errors.Join(resultErr, signalErr)
		}
		select {
		case waitErr := <-done:
			resultErr = errors.Join(resultErr, waitErr)
		case <-time.After(30 * time.Second):
			killErr := command.Process.Kill()
			resultErr = errors.Join(resultErr, killErr, <-done)
		}
	}()
	connections, consoleErr := ios.Ready(ctx, []int{28200})
	if consoleErr != nil {
		return consoleErr
	}
	defer func() { resultErr = errors.Join(resultErr, ios.Close(connections)) }()
	return pingIOS(ctx, connections[0])
}
