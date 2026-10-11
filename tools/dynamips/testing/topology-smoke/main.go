// Command topology-smoke verifies real IOS routing, lifecycle, storage, and CLI cleanup.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	native "github.com/tethux/dynamips-downgrade"
	"github.com/tethux/dynamips-downgrade/console"
	"github.com/tethux/tethux/topology/local"
	topologytoml "github.com/tethux/tethux/topology/toml"
	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/emulator/dynamips"
	"github.com/tethux/tethux/virt/emulator/dynamips/errs"
)

var promptPattern = regexp.MustCompile(`(?:^|[\r\n])([A-Za-z][A-Za-z0-9_.-]*(?:\([^()\r\n]*\))?[>#])\s*$`)

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
	image := flag.String("image", defaultImage(), "local C7200 IOS image")
	binary := flag.String("tethux", "", "built tethux CLI")
	example := flag.String("example", "topology/examples/dynamips-pair.toml", "two-router TOML example")
	timeout := flag.Duration("timeout", 6*time.Minute, "total integration timeout")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	result := report{Name: "dynamips-topology"}
	started := time.Now()
	runErr := run(ctx, *image, *binary, *example, &result)
	result.Passed, result.Seconds = runErr == nil, time.Since(started).Seconds()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		runErr = errors.Join(runErr, encodeErr)
	}
	return runErr
}

func defaultImage() string {
	if image := os.Getenv("TETHUX_DYNAMIPS_IMAGE"); image != "" {
		return image
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "tethux", "images", "cisco", "c7200-a3jk9s-mz.124-25g.bin")
}

func run(ctx context.Context, image, binary, example string, result *report) (resultErr error) {
	if binary == "" {
		return errs.New("test Dynamips", errs.ErrConfig, "--tethux is required")
	}
	for _, path := range []*string{&image, &binary, &example} {
		absolute, pathErr := filepath.Abs(*path)
		if pathErr != nil {
			return pathErr
		}
		*path = absolute
	}
	if _, statErr := os.Stat(image); statErr != nil {
		return errs.Wrap("open IOS image; set TETHUX_DYNAMIPS_IMAGE", errs.ErrConfig, image, statErr)
	}
	stateRoot, directoryErr := os.MkdirTemp("", "tethux-dynamips-")
	if directoryErr != nil {
		return directoryErr
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(stateRoot)) }()
	workingDirectory, workingDirectoryErr := os.Getwd()
	if workingDirectoryErr != nil {
		return workingDirectoryErr
	}
	if changeDirectoryErr := os.Chdir(stateRoot); changeDirectoryErr != nil {
		return changeDirectoryErr
	}
	defer func() { resultErr = errors.Join(resultErr, os.Chdir(workingDirectory)) }()
	phase("CLI topology")
	cliArgs := []string{"topology", "run", example, "--dynamips-image", image, "--state-root", stateRoot, "--base-port", "27100", "--console-base-port", "28100"}
	if cliErr := runCLI(ctx, binary, cliArgs, []int{28100, 28101}, true); cliErr != nil {
		return cliErr
	}
	result.Checks = append(result.Checks, "CLI topology, bidirectional IOS ping, graceful cleanup")
	phase("CLI router")
	startup := filepath.Join(stateRoot, "startup.cfg")
	if writeErr := os.WriteFile(startup, []byte("hostname CLIRouter\nno ip domain-lookup\nend\n"), 0o600); writeErr != nil {
		return writeErr
	}
	virtArgs := []string{"virt", "dynamips", "run", "--image", image, "--name", "cli-router", "--state-root", stateRoot, "--startup-config", startup, "--console=false", "--console-port", "28102"}
	if cliErr := runCLI(ctx, binary, virtArgs, []int{28102}, false); cliErr != nil {
		return cliErr
	}
	result.Checks = append(result.Checks, "CLI router boot and cleanup")
	file, openErr := os.Open(example) // #nosec G304 -- Explicit integration fixture path.
	if openErr != nil {
		return openErr
	}
	top, decodeErr := topologytoml.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		return errors.Join(decodeErr, closeErr)
	}
	runtime, runtimeErr := native.New()
	if runtimeErr != nil {
		return runtimeErr
	}
	adapter := &dynamips.Adapter{Runtime: runtime}
	defer func() { resultErr = errors.Join(resultErr, adapter.Close(), runtime.Close()) }()
	phase("native topology and lifecycle")
	running, startErr := local.Start(ctx, top, local.Options{BasePort: 27100, Dynamips: &local.DynamipsOptions{
		Provider: adapter, StateRoot: stateRoot, Image: image, ConsoleBasePort: 28100,
	}})
	if startErr != nil {
		return startErr
	}
	defer func() { resultErr = errors.Join(resultErr, running.Close(context.WithoutCancel(ctx))) }()
	connections, connectionErr := readyConsoles(ctx, []int{28100, 28101})
	if connectionErr != nil {
		return connectionErr
	}
	defer closeConsoles(connections)
	if pingErr := pingPair(ctx, connections); pingErr != nil {
		return pingErr
	}
	nodes, listErr := adapter.List(ctx)
	if listErr != nil || len(nodes) != 2 {
		return errors.Join(listErr, errs.New("list routers", errs.ErrLifecycle, "expected two routers"))
	}
	if reload, reloadErr := adapter.Reload(ctx, "r1"); reloadErr != nil || reload.State != virt.NodeRunning {
		return errors.Join(reloadErr, errs.New("reload router", errs.ErrLifecycle, "expected running"))
	}
	if suspendErr := adapter.Suspend(ctx, "r1"); suspendErr != nil {
		return suspendErr
	}
	if stateErr := expectState(ctx, adapter, "r1", virt.NodeSuspended); stateErr != nil {
		return stateErr
	}
	if resumeErr := adapter.Resume(ctx, "r1"); resumeErr != nil {
		return resumeErr
	}
	if stateErr := expectState(ctx, adapter, "r1", virt.NodeRunning); stateErr != nil {
		return stateErr
	}
	for _, input := range []string{"configure terminal", "hostname PersistedR1", "end", "write memory"} {
		if _, commandErr := command(ctx, connections[0], input); commandErr != nil {
			return commandErr
		}
	}
	if restartErr := adapter.Restart(ctx, "r1"); restartErr != nil {
		return restartErr
	}
	_ = connections[0].Close()
	reopened, reopenErr := readyConsoles(ctx, []int{28100})
	if reopenErr != nil {
		return reopenErr
	}
	connections[0] = reopened[0]
	output, versionErr := command(ctx, connections[0], "show version")
	if versionErr != nil || !strings.Contains(output, "PersistedR1") {
		return errors.Join(versionErr, errs.New("verify saved NVRAM", errs.ErrLifecycle, output))
	}
	if pingErr := pingPair(ctx, connections); pingErr != nil {
		return pingErr
	}
	if stopErr := adapter.Stop(ctx, "r1"); stopErr != nil {
		return stopErr
	}
	if stateErr := expectState(ctx, adapter, "r1", virt.NodeStopped); stateErr != nil {
		return stateErr
	}
	if stopErr := adapter.Stop(ctx, "r1"); stopErr != nil {
		return stopErr
	}
	result.Checks = append(result.Checks, "native state/list/reload, pause/resume/restart/stop, saved NVRAM, ping after restart")
	return nil
}

func expectState(ctx context.Context, adapter *dynamips.Adapter, id string, want virt.NodeState) error {
	state, stateErr := adapter.State(ctx, id)
	if stateErr != nil {
		return stateErr
	}
	if state != want {
		return errs.New("verify router state", errs.ErrLifecycle, string(state)+" expected "+string(want))
	}
	return nil
}

func runCLI(ctx context.Context, binary string, args []string, ports []int, ping bool) (resultErr error) {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	process := exec.CommandContext(childCtx, binary, args...) // #nosec G204 -- Explicit built CLI path and structured integration arguments.
	process.Stdout, process.Stderr = os.Stderr, os.Stderr
	if startErr := process.Start(); startErr != nil {
		return startErr
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			resultErr = errors.Join(resultErr, process.Wait())
		}
	}()
	connections, readyErr := readyConsoles(ctx, ports)
	if readyErr != nil {
		return readyErr
	}
	defer closeConsoles(connections)
	if ping {
		if pingErr := pingPair(ctx, connections); pingErr != nil {
			return pingErr
		}
	} else {
		output, commandErr := command(ctx, connections[0], "show version")
		if commandErr != nil || !strings.Contains(output, "Cisco IOS Software") {
			return errors.Join(commandErr, errs.New("verify CLI IOS boot", errs.ErrLifecycle, output))
		}
	}
	if signalErr := process.Process.Signal(os.Interrupt); signalErr != nil {
		return signalErr
	}
	waitErr := process.Wait()
	waited = true
	return waitErr
}

func readyConsoles(ctx context.Context, ports []int) ([]*console.Connection, error) {
	connections := make([]*console.Connection, 0, len(ports))
	for _, port := range ports {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		var connection *console.Connection
		for {
			conn, dialErr := console.Dial(ctx, address)
			if dialErr == nil {
				connection = conn
				break
			}
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				closeConsoles(connections)
				return nil, errors.Join(ctx.Err(), dialErr)
			case <-timer.C:
			}
		}
		connections = append(connections, connection)
		prompt, _, promptErr := readPrompt(ctx, connection, true, "")
		if promptErr != nil {
			closeConsoles(connections)
			return nil, promptErr
		}
		if strings.HasSuffix(prompt, ">") {
			if _, enableErr := command(ctx, connection, "enable"); enableErr != nil {
				closeConsoles(connections)
				return nil, enableErr
			}
		}
		if _, pagingErr := command(ctx, connection, "terminal length 0"); pagingErr != nil {
			closeConsoles(connections)
			return nil, pagingErr
		}
	}
	return connections, nil
}

func closeConsoles(connections []*console.Connection) {
	for _, connection := range connections {
		_ = connection.Close()
	}
}

func command(ctx context.Context, connection *console.Connection, input string) (string, error) {
	if _, writeErr := io.WriteString(connection, input+"\r"); writeErr != nil {
		return "", writeErr
	}
	_, output, readErr := readPrompt(ctx, connection, false, input)
	if readErr != nil {
		return output, readErr
	}
	if strings.Contains(output, "% Invalid") || strings.Contains(output, "% Incomplete") || strings.Contains(output, "% Error") {
		return output, errs.New("IOS command "+input, errs.ErrLifecycle, output)
	}
	return output, nil
}

func readPrompt(ctx context.Context, connection *console.Connection, boot bool, input string) (prompt, transcript string, resultErr error) {
	deadline := time.Now().Add(90 * time.Second)
	var output bytes.Buffer
	buffer := make([]byte, 4096)
	lastNudge := time.Time{}
	for time.Now().Before(deadline) {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", output.String(), contextErr
		}
		if deadlineErr := connection.SetDeadline(time.Now().Add(time.Second)); deadlineErr != nil {
			return "", output.String(), deadlineErr
		}
		count, readErr := connection.Read(buffer)
		output.Write(buffer[:count])
		text := output.String()
		if output.Len() > 2<<20 {
			return "", text, errs.New("read IOS prompt", errs.ErrLifecycle, "console exceeded 2 MiB")
		}
		if match := promptPattern.FindStringSubmatch(text); match != nil && (input == "" || strings.Contains(text, input)) {
			return match[1], text, nil
		}
		if boot && time.Since(lastNudge) > 3*time.Second {
			reply := "\r"
			if strings.Contains(strings.ToLower(text), "initial configuration dialog") {
				reply = "no\r"
			}
			if strings.Contains(strings.ToLower(text), "terminate autoinstall") {
				reply = "yes\r"
			}
			if _, writeErr := io.WriteString(connection, reply); writeErr != nil {
				return "", text, writeErr
			}
			lastNudge = time.Now()
		}
		var timeout net.Error
		if readErr != nil && (!errors.As(readErr, &timeout) || !timeout.Timeout()) {
			return "", text, readErr
		}
	}
	return "", output.String(), errs.New("read IOS prompt", errs.ErrLifecycle, "timed out: "+output.String())
}

func pingPair(ctx context.Context, connections []*console.Connection) error {
	for index, target := range []string{"10.77.0.2", "10.77.0.1"} {
		passed := false
		for attempt := 0; attempt < 3; attempt++ {
			output, commandErr := command(ctx, connections[index], "ping "+target+" repeat 3 timeout 1")
			if commandErr != nil {
				return commandErr
			}
			if strings.Contains(output, "Success rate is 100 percent") {
				passed = true
				break
			}
		}
		if !passed {
			return errs.New("ping router", errs.ErrLifecycle, target+" did not reply to every packet")
		}
	}
	return nil
}

func phase(name string) { fmt.Fprintln(os.Stderr, "Dynamips integration:", name) }
