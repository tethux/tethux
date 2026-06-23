//go:build ignore

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type config struct {
	runtime      string
	n            int
	basePort     int
	image        string
	mtu          int
	ifPrefix     string
	parallelJobs int
	pingCount    int
	pingTimeout  int
	ifTimeout    time.Duration
	keepLogs     bool
}

type node struct {
	index       int
	container   string
	hostIf      string
	containerIf string
	pid         string
	switchCmd   *exec.Cmd
	logFile     *os.File
}

func main() {
	log.SetFlags(0)

	cfg := parseFlags()
	if os.Geteuid() != 0 {
		log.Fatalf("this demo needs root for veth/raw sockets: sudo go run ./scripts/container-udp-topology.go --runtime %s --n %d", cfg.runtime, cfg.n)
	}

	if _, err := exec.LookPath(cfg.runtime); err != nil {
		log.Fatalf("%s is required: %v", cfg.runtime, err)
	}

	root, err := repoRoot()
	if err != nil {
		log.Fatal(err)
	}

	cleanupStaleDemoState(cfg)
	if err := ensureUDPPortsAvailable(cfg); err != nil {
		log.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().Unix()%1000000)
	bin := filepath.Join(os.TempDir(), "tethux-demo-"+suffix)
	nodes := makeNodes(cfg, suffix)

	exitCode := 0
	cleanup := func() {
		started := time.Now()
		stopSwitches(nodes)
		removeContainers(cfg, nodes)
		_ = os.Remove(bin)
		if elapsed := time.Since(started).Round(time.Second); elapsed > time.Second {
			log.Printf("cleanup: finished in %s", elapsed)
		}
	}
	defer func() {
		cleanup()
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	if err := phase("[1/5] building tethux and starting containers", func() error {
		if err := run(root, map[string]string{"GOCACHE": getenv("GOCACHE", filepath.Join(os.TempDir(), "gocache"))}, "go", "build", "-o", bin, "./cmd/tethux"); err != nil {
			return err
		}
		return parallel(cfg.parallelJobs, len(nodes), func(i int) error {
			n := &nodes[i]
			return runQuiet(root, nil, cfg.runtime, "run", "-d", "--name", n.container, "--rm", "--net=none", "--cap-add=NET_ADMIN", cfg.image, "sleep", "infinity")
		})
	}); err != nil {
		log.Printf("ERROR: %v", err)
		exitCode = 1
		return
	}

	if err := phase("[2/5] starting switches with local UDP remotes", func() error {
		for i := range nodes {
			pid, err := output(root, cfg.runtime, "inspect", "-f", "{{.State.Pid}}", nodes[i].container)
			if err != nil {
				return err
			}
			nodes[i].pid = strings.TrimSpace(pid)
		}

		for i := range nodes {
			cmd, err := startSwitch(root, bin, cfg, suffix, &nodes[i])
			if err != nil {
				return err
			}
			nodes[i].switchCmd = cmd
		}
		return nil
	}); err != nil {
		log.Printf("ERROR: %v", err)
		exitCode = 1
		return
	}

	if err := phase("[3/5] assigning container IPs", func() error {
		return parallel(cfg.parallelJobs, len(nodes), func(i int) error {
			return assignContainerIP(root, cfg, nodes[i])
		})
	}); err != nil {
		log.Printf("ERROR: %v", err)
		exitCode = 1
		return
	}

	fmt.Println("[4/5] topology")
	for _, n := range nodes {
		fmt.Printf("  %s:%s 10.77.0.%-3d <-> switch %-2d", n.container, n.containerIf, n.index, n.index)
		if n.index < cfg.n {
			fmt.Printf(" ==udp:%d/%d==", linkPortLeft(cfg, n.index), linkPortRight(cfg, n.index))
		}
		fmt.Println()
	}

	if err := phase("[5/5] proving first container reaches last container", func() error {
		return run(root, nil, cfg.runtime, "exec", nodes[0].container, "ping", "-c", fmt.Sprint(cfg.pingCount), "-W", fmt.Sprint(cfg.pingTimeout), fmt.Sprintf("10.77.0.%d", cfg.n))
	}); err != nil {
		log.Printf("ERROR: %v", err)
		exitCode = 1
		return
	}

	fmt.Printf("success: containers are networked through %d Go switches and local UDP remote pairs\n", cfg.n)
	fmt.Printf("switch logs: /tmp/tethux-switch-%s-*.log\n", suffix)
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.runtime, "runtime", getenv("RUNTIME", "podman"), "container runtime: podman or docker")
	flag.IntVar(&cfg.n, "n", getenvInt("N", 2), "switch and container count")
	flag.IntVar(&cfg.basePort, "base-port", getenvInt("BASE_PORT", 23000), "first local UDP link port")
	flag.StringVar(&cfg.image, "image", getenv("IMAGE", "alpine"), "container image")
	flag.IntVar(&cfg.mtu, "mtu", getenvInt("MTU", 1500), "link MTU")
	flag.StringVar(&cfg.ifPrefix, "container-if-prefix", getenv("CONTAINER_IF_PREFIX", "tx"), "container interface prefix")
	flag.IntVar(&cfg.parallelJobs, "parallel-jobs", getenvInt("PARALLEL_JOBS", 16), "max concurrent runtime operations")
	flag.IntVar(&cfg.pingCount, "ping-count", getenvInt("PING_COUNT", 2), "ping packet count")
	flag.IntVar(&cfg.pingTimeout, "ping-timeout", getenvInt("PING_TIMEOUT", 1), "ping timeout seconds")
	flag.DurationVar(&cfg.ifTimeout, "interface-timeout", getenvDuration("INTERFACE_TIMEOUT", 15*time.Second), "time to wait for each container interface")
	flag.BoolVar(&cfg.keepLogs, "keep-logs", true, "keep switch logs in /tmp")
	flag.Parse()

	if cfg.runtime != "podman" && cfg.runtime != "docker" {
		log.Fatal("--runtime must be podman or docker")
	}
	if cfg.n < 2 {
		log.Fatal("--n must be >= 2")
	}
	if len(cfg.ifPrefix) > 13 {
		log.Fatal("--container-if-prefix must be 13 characters or fewer")
	}
	if cfg.parallelJobs < 1 {
		log.Fatal("--parallel-jobs must be >= 1")
	}
	return cfg
}

func repoRoot() (string, error) {
	out, err := output("", "git", "rev-parse", "--show-toplevel")
	if err == nil {
		return strings.TrimSpace(out), nil
	}
	wd, wdErr := os.Getwd()
	if wdErr != nil {
		return "", errors.Join(err, wdErr)
	}
	return wd, nil
}

func makeNodes(cfg config, suffix string) []node {
	nodes := make([]node, cfg.n)
	for i := 1; i <= cfg.n; i++ {
		nodes[i-1] = node{
			index:       i,
			container:   fmt.Sprintf("tethux-demo-%s-%02d", suffix, i),
			hostIf:      fmt.Sprintf("tx%s%02d", suffix, i),
			containerIf: fmt.Sprintf("%s%02d", cfg.ifPrefix, i),
		}
	}
	return nodes
}

func startSwitch(root, bin string, cfg config, suffix string, n *node) (*exec.Cmd, error) {
	args := []string{"bridge", "container", "--pid", n.pid, "--interface-mode", "create-veth", "--host-if", n.hostIf, "--container-if", n.containerIf, "--mtu", fmt.Sprint(cfg.mtu)}
	if n.index > 1 {
		left := n.index - 1
		args = append(args, "--port", fmt.Sprintf("id=sw%d-left,scheme=udp,listen=127.0.0.1:%d,remote=127.0.0.1:%d,mtu=%d", n.index, linkPortRight(cfg, left), linkPortLeft(cfg, left), cfg.mtu))
	}
	if n.index < cfg.n {
		args = append(args, "--port", fmt.Sprintf("id=sw%d-right,scheme=udp,listen=127.0.0.1:%d,remote=127.0.0.1:%d,mtu=%d", n.index, linkPortLeft(cfg, n.index), linkPortRight(cfg, n.index), cfg.mtu))
	}

	logFile, err := os.Create(filepath.Join(os.TempDir(), fmt.Sprintf("tethux-switch-%s-%d.log", suffix, n.index)))
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	n.logFile = logFile
	return cmd, nil
}

func assignContainerIP(root string, cfg config, n node) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ifTimeout)
	defer cancel()

	addrArgs := []string{"exec", n.container, "ip", "addr", "add", fmt.Sprintf("10.77.0.%d/24", n.index), "dev", n.containerIf}
	for {
		err := runQuietContext(ctx, root, nil, cfg.runtime, "exec", n.container, "ip", "link", "show", n.containerIf)
		if err == nil {
			err = runQuietContext(ctx, root, nil, cfg.runtime, addrArgs...)
			if err == nil {
				return nil
			}
		}
		if ctx.Err() != nil {
			return fmt.Errorf("timed out assigning 10.77.0.%d/24 to %s in %s", n.index, n.containerIf, n.container)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func stopSwitches(nodes []node) {
	if len(nodes) == 0 {
		return
	}
	log.Printf("cleanup: stopping %d switch processes", len(nodes))
	for _, n := range nodes {
		if n.switchCmd != nil && n.switchCmd.Process != nil {
			_ = n.switchCmd.Process.Signal(syscall.SIGTERM)
		}
	}

	deadline := time.After(2 * time.Second)
	for _, n := range nodes {
		if n.switchCmd == nil || n.switchCmd.Process == nil {
			continue
		}
		done := make(chan struct{})
		go func(cmd *exec.Cmd, logFile *os.File) {
			_ = cmd.Wait()
			if logFile != nil {
				_ = logFile.Close()
			}
			close(done)
		}(n.switchCmd, n.logFile)

		select {
		case <-done:
		case <-deadline:
			_ = n.switchCmd.Process.Kill()
		}
	}

	for _, n := range nodes {
		if n.switchCmd != nil && n.switchCmd.Process != nil {
			_ = n.switchCmd.Process.Kill()
		}
	}
}

func cleanupStaleDemoState(cfg config) {
	log.Print("preflight: cleaning stale tethux demo state")
	_ = runQuiet("", nil, "pkill", "-TERM", "-f", `/tmp/tethux-demo-[0-9]+ bridge container`)
	time.Sleep(2 * time.Second)
	_ = runQuiet("", nil, "pkill", "-KILL", "-f", `/tmp/tethux-demo-[0-9]+ bridge container`)

	names, err := output("", cfg.runtime, "ps", "-a", "--format", "{{.Names}}")
	if err == nil {
		for _, name := range strings.Fields(names) {
			if strings.HasPrefix(name, "tethux-demo-") {
				args := []string{"rm", "-f"}
				if cfg.runtime == "podman" {
					args = append(args, "--time", "0")
				}
				args = append(args, name)
				_ = runQuiet("", nil, cfg.runtime, args...)
			}
		}
	}

	deleteStaleHostLinks()
}

func deleteStaleHostLinks() {
	out, err := output("", "ip", "-o", "link", "show")
	if err != nil {
		return
	}

	demoLink := regexp.MustCompile(`^\d+:\s+(tx\d{8})(?:@|:)`)
	for _, line := range strings.Split(out, "\n") {
		matches := demoLink.FindStringSubmatch(line)
		if len(matches) == 2 {
			_ = runQuiet("", nil, "ip", "link", "delete", matches[1])
		}
	}
}

func ensureUDPPortsAvailable(cfg config) error {
	lastPort := cfg.basePort + (cfg.n-2)*2 + 1
	for port := cfg.basePort; port <= lastPort; port++ {
		conn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return fmt.Errorf("UDP port %d is still in use after stale cleanup: %w", port, err)
		}
		_ = conn.Close()
	}

	return nil
}

func removeContainers(cfg config, nodes []node) {
	if len(nodes) == 0 {
		return
	}
	log.Printf("cleanup: removing %d containers", len(nodes))
	_ = parallel(cfg.parallelJobs, len(nodes), func(i int) error {
		args := []string{"rm", "-f"}
		if cfg.runtime == "podman" {
			args = append(args, "--time", "0")
		}
		args = append(args, nodes[i].container)
		_ = runQuiet("", nil, cfg.runtime, args...)
		return nil
	})
}

func parallel(limit, total int, fn func(int) error) error {
	sem := make(chan struct{}, limit)
	errs := make(chan error, total)
	var wg sync.WaitGroup

	for i := 0; i < total; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			errs <- fn(i)
		}(i)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func phase(label string, fn func() error) error {
	fmt.Println(label)
	started := time.Now()
	if err := fn(); err != nil {
		return err
	}
	fmt.Printf("%s completed in %s\n", label[:5], time.Since(started).Round(time.Second))
	return nil
}

func linkPortLeft(cfg config, linkIndex int) int {
	return cfg.basePort + (linkIndex-1)*2
}

func linkPortRight(cfg config, linkIndex int) int {
	return cfg.basePort + (linkIndex-1)*2 + 1
}

func run(dir string, env map[string]string, name string, args ...string) error {
	return runContext(context.Background(), dir, env, name, args...)
}

func runContext(ctx context.Context, dir string, env map[string]string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd.Run()
}

func runQuiet(dir string, env map[string]string, name string, args ...string) error {
	return runQuietContext(context.Background(), dir, env, name, args...)
}

func runQuietContext(ctx context.Context, dir string, env map[string]string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd.Run()
}

func output(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(os.Getenv(key), "%d", &value); err == nil {
		return value
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}
