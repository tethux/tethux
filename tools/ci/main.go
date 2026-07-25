// tethux-ci runs repository tests, archives, test hosts, and deployments.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	ciframework "github.com/0xveya/tethux/internal/ci"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := dispatch(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tethux-ci: %v\n", err)
		os.Exit(exitCode(err))
	}
}

func dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("")
	}
	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:])
	case "archive":
		return archiveCommand(ctx, args[1:])
	case "host":
		return hostCommand(ctx, args[1:])
	case "topology":
		return topologyCommand(ctx, args[1:])
	case "deploy":
		return deployCommand(ctx, args[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		return usageError("unknown command " + args[0])
	}
}

func deployCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("deploy requires viewer or tunnel-token")
	}
	switch args[0] {
	case "tunnel-token":
		flags := flag.NewFlagSet("deploy tunnel-token", flag.ContinueOnError)
		envFile := flags.String("env-file", "/deployment/.env", "deployment environment file")
		network := flags.String("network", "tethux-ci-viewer", "viewer Docker network")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, "Cloudflare tunnel token: ")
		setEcho := func(enabled bool) {
			argument := "-echo"
			if enabled {
				argument = "echo"
			}
			command := exec.Command("stty", argument)
			command.Stdin = os.Stdin
			_ = command.Run()
		}
		setEcho(false)
		defer func() { setEcho(true); _, _ = fmt.Fprintln(os.Stderr) }()
		var token string
		if _, err := fmt.Fscanln(os.Stdin, &token); err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		if len(token) < 20 {
			return errors.New("token appears incomplete")
		}
		if err := os.MkdirAll(filepath.Dir(*envFile), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(*envFile, []byte("TUNNEL_TOKEN="+token+"\n"), 0o600); err != nil {
			return err
		}
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", "tethux-ci-viewer-tunnel").Run()
		command := exec.CommandContext(
			ctx, "docker", "run", "-d", "--name", "tethux-ci-viewer-tunnel",
			"--restart", "unless-stopped", "--network", *network, "--read-only",
			"--env-file", *envFile,
			"cloudflare/cloudflared:latest", "tunnel", "--no-autoupdate",
			"run",
		)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		return command.Run()
	case "viewer":
		flags := flag.NewFlagSet("deploy viewer", flag.ContinueOnError)
		source := flags.String("source", ".", "repository build context")
		image := flags.String("image", "tethux-ci-viewer:latest", "viewer image")
		network := flags.String("network", "tethux-ci-viewer", "dedicated Docker network")
		dataDir := flags.String("data-dir", "/var/cache/tethux-ci/viewer", "persistent viewer data directory")
		archiveDir := flags.String("archive-dir", "/var/cache/tethux-ci/archive", "read-only test archive directory")
		envFile := flags.String("env-file", "/deployment/.env", "Cloudflare tunnel environment file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if err := os.MkdirAll(*dataDir, 0o750); err != nil {
			return err
		}
		if info, err := os.Stat(*archiveDir); err != nil || !info.IsDir() {
			return fmt.Errorf("archive directory %s is unavailable", *archiveDir)
		}
		if err := os.Chown(*dataDir, 100, 101); err != nil {
			return fmt.Errorf("grant viewer data ownership: %w", err)
		}
		helperPath := filepath.Join(filepath.Dir(*envFile), "tethux-ci")
		helper := exec.CommandContext(ctx, "go", "build", "-o", helperPath, "./tools/ci")
		helper.Dir, helper.Stdout, helper.Stderr = *source, os.Stdout, os.Stderr
		if err := helper.Run(); err != nil {
			return fmt.Errorf("build NAS deployment helper: %w", err)
		}
		composeSource := filepath.Join(
			*source,
			"tools",
			"ci-results",
			"viewer",
			"compose.yaml",
		)
		composePath := filepath.Join(filepath.Dir(*envFile), "compose.yaml")
		composeContent, err := os.ReadFile(composeSource)
		if err != nil {
			return fmt.Errorf("read viewer compose file: %w", err)
		}
		if err := os.WriteFile(composePath, composeContent, 0o600); err != nil {
			return fmt.Errorf("install viewer compose file: %w", err)
		}
		run := func(arguments ...string) error {
			cmd := exec.CommandContext(ctx, "docker", arguments...)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			return cmd.Run()
		}
		dockerfile := filepath.Join(*source, "tools", "ci-results", "viewer", "Dockerfile")
		if err := run("build", "-f", dockerfile, "-t", *image, *source); err != nil {
			return fmt.Errorf("build viewer image: %w", err)
		}
		composeArgs := []string{"compose", "-f", composePath}
		if configured, err := tunnelConfigured(*envFile); err != nil {
			return err
		} else if configured {
			composeArgs = append(composeArgs, "--profile", "tunnel")
		} else {
			if _, err := fmt.Fprintln(os.Stdout, "viewer deployed; tunnel pending secure token installation"); err != nil {
				return err
			}
		}
		composeArgs = append(composeArgs, "up", "-d", "--force-recreate", "--remove-orphans")
		command := exec.CommandContext(ctx, "docker", composeArgs...)
		command.Env = append(
			os.Environ(),
			"TETHUX_VIEWER_IMAGE="+*image,
			"TETHUX_VIEWER_NETWORK="+*network,
			"TETHUX_VIEWER_DATA_DIR="+*dataDir,
			"TETHUX_VIEWER_ARCHIVE_DIR="+*archiveDir,
			"TETHUX_VIEWER_ENV_FILE="+*envFile,
		)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("apply viewer compose deployment: %w", err)
		}
		return nil
	default:
		return usageError("unknown deploy command " + args[0])
	}
}

func tunnelConfigured(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(content), "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && name == "TUNNEL_TOKEN" && len(strings.TrimSpace(value)) >= 20 {
			return true, nil
		}
	}
	return false, nil
}

func runCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("run requires a workflow")
	}
	name := args[0]
	flags := flag.NewFlagSet("run "+name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	rootDefault, _ := ciframework.RepositoryRoot()
	root := flags.String("root", rootDefault, "repository root")
	runtimeName := flags.String("runtime", envDefault("TETHUX_CONTAINER_RUNTIME", ""), "container runtime")
	provider := flags.String("provider", "all", "container provider")
	host := flags.String("host", "", "remote SSH target")
	hostA := flags.String("host-a", envDefault("TETHUX_LINK_HOST_A", "ci@10.0.0.100"), "first cross-host target")
	hostB := flags.String("host-b", envDefault("TETHUX_LINK_HOST_B", "ci@10.0.0.78"), "second cross-host target")
	jump := flags.String("jump-host", envDefault("TETHUX_SSH_JUMP", ""), "SSH jump host")
	device := flags.String("device", envDefault("TETHUX_DEVICE_ID", ""), "stable runner device ID")
	archiveEnabled := flags.Bool("archive", false, "write a Test Archive Format result")
	archiveRoot := flags.String("archive-root", envDefault("TETHUX_TEST_ARCHIVE_ROOT", ""), "archive root")
	dryRun := flags.Bool("dry-run", false, "print steps without running them")
	integration := flags.Bool("integration", false, "confirm privileged local integration")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("repository root could not be determined")
	}

	if name == "remote-laptop" {
		if *host == "" {
			return errors.New("--host is required")
		}
		return runRemoteLaptop(ctx, *root, *host, *jump, *runtimeName, *archiveRoot, *device, *archiveEnabled, *dryRun)
	}
	if name == "cross-laptop" {
		return runCrossLaptop(ctx, *root, *hostA, *hostB, *archiveRoot, *device, *archiveEnabled, *dryRun)
	}
	if name == "local" {
		if !*integration && os.Getenv("TETHUX_RUN_INTEGRATION") != "1" {
			return errors.New("local privileged integration requires --integration or TETHUX_RUN_INTEGRATION=1")
		}
		if *runtimeName == "" {
			return errors.New("--runtime must be docker or podman")
		}
		if !*dryRun {
			if err := checkFixtureRegistry(ctx); err != nil {
				return err
			}
		}
		name = "laptop"
		*archiveEnabled = true
	}

	workflow, err := workflowFor(name, *root, *runtimeName, *provider)
	if err != nil {
		return err
	}
	return executeWorkflow(ctx, workflow, executeOptions{
		Root: *root, Runtime: *runtimeName, Device: *device, Archive: *archiveEnabled,
		ArchiveRoot: *archiveRoot, DryRun: *dryRun,
	})
}

type executeOptions struct {
	Root, Runtime, Device, ArchiveRoot string
	Archive                            bool
	DryRun                             bool
}

func executeWorkflow(ctx context.Context, workflow ciframework.Workflow, options executeOptions) error {
	var writer *ciframework.ArchiveWriter
	var err error
	started := time.Now().UTC()
	stdout := io.Writer(os.Stdout)
	if options.Archive {
		writer, err = ciframework.NewArchiveWriter(ciframework.ArchiveOptions{
			Root: options.ArchiveRoot, Repository: options.Root, Workflow: workflow.Name,
			DeviceID: options.Device, Runtime: options.Runtime, StartedAt: started,
		})
		if err != nil {
			return err
		}
		for index := range workflow.Steps {
			step := &workflow.Steps[index]
			if step.CaptureStdout != "" {
				step.CaptureStdout = filepath.Join(writer.ArtifactDir(), filepath.Base(step.CaptureStdout))
			}
		}
		logFile, openErr := os.OpenFile(filepath.Join(writer.LogDir(), "runner.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
		if openErr != nil {
			return openErr
		}
		defer logFile.Close()
		stdout = io.MultiWriter(os.Stdout, logFile)
	}
	runner := ciframework.NewRunner(stdout, io.MultiWriter(os.Stderr, stdout))
	runner.DryRun = options.DryRun
	if writer != nil {
		runner.BaseEnv = map[string]string{
			"TETHUX_CI_ARCHIVE_DIR": writer.Stage,
			"TETHUX_RESULTS_DIR":    writer.ArtifactDir(),
			"TETHUX_RUN_ID":         writer.Options.RunID,
		}
	}
	result, runErr := runner.Run(ctx, workflow)
	if writer != nil && !options.DryRun {
		writer.Options.FinishedAt = time.Now().UTC()
		writer.Options.CommandErr = runErr
		resultPath := filepath.Join(writer.ConfigDir(), "workflow.json")
		if err := writeJSON(resultPath, result); err != nil {
			return errors.Join(runErr, err)
		}
		archivePath, finalizeErr := writer.Finalize(ctx)
		if finalizeErr != nil {
			return errors.Join(runErr, finalizeErr)
		}
		fmt.Fprintf(os.Stdout, "test archive: %s\n", archivePath)
		if host := os.Getenv("TETHUX_ARCHIVE_NAS_HOST"); host != "" {
			if err := publishArchive(ctx, archivePath, host, envDefault("TETHUX_NAS_ARCHIVE_ROOT", "/var/cache/tethux-ci/archive")); err != nil {
				return errors.Join(runErr, err)
			}
		}
	}
	return runErr
}

func workflowFor(name, root, runtimeName, provider string) (ciframework.Workflow, error) {
	registry, err := ciframework.DefaultRegistry(root)
	if err != nil {
		return ciframework.Workflow{}, err
	}
	if workflow, ok := registry.Workflow(name); ok {
		if name == "provider" {
			workflow.Steps[0].Args = []string{"run", "./cmd/tethux", "virt", "test", "--provider", provider, "--output", "json"}
		}
		if name == "topology" {
			if runtimeName == "" || runtimeName == "all" {
				base := workflow.Steps[0]
				base.Name = "topology-podman"
				base.Args = []string{"run", "./tools/bridge/example/container-udp", "--runtime", "podman"}
				docker := base
				docker.Name = "topology-docker"
				docker.Args = []string{"run", "./tools/bridge/example/container-udp", "--runtime", "docker"}
				workflow.Steps = []ciframework.Step{base, docker}
			} else {
				workflow.Steps[0].Args = []string{"run", "./tools/bridge/example/container-udp", "--runtime", runtimeName}
			}
		}
		return workflow, nil
	}
	if name != "laptop" {
		return ciframework.Workflow{}, fmt.Errorf("unknown workflow %q", name)
	}
	if runtimeName != "docker" && runtimeName != "podman" {
		return ciframework.Workflow{}, errors.New("laptop workflow requires --runtime docker or podman")
	}
	normal, _ := registry.Workflow("normal")
	providers, _ := registry.Workflow("provider")
	topology, _ := registry.Workflow("topology")
	backends, _ := registry.Workflow("bridge")
	steps := append([]ciframework.Step(nil), normal.Steps...)
	for _, group := range [][]ciframework.Step{backends.Steps, providers.Steps, topology.Steps} {
		for _, step := range group {
			step.DependsOn = []string{"build"}
			if step.Name == "topology" {
				step.Args = []string{"run", "./tools/bridge/example/container-udp", "--runtime", runtimeName}
			}
			steps = append(steps, step)
		}
	}
	return ciframework.Workflow{Name: "laptop-" + runtimeName, Description: "complete test host integration", Steps: steps}, nil
}

func archiveCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("archive requires run, finalize, publish, or inventory")
	}
	switch args[0] {
	case "run":
		flags := flag.NewFlagSet("archive run", flag.ContinueOnError)
		workflow := flags.String("workflow", "", "workflow name")
		root := flags.String("archive-root", envDefault("TETHUX_TEST_ARCHIVE_ROOT", ""), "archive root")
		device := flags.String("device", envDefault("TETHUX_DEVICE_ID", ""), "device ID")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		command := flags.Args()
		if *workflow == "" || len(command) == 0 {
			return errors.New("archive run requires --workflow and a command after flags")
		}
		repository, err := ciframework.RepositoryRoot()
		if err != nil {
			return err
		}
		return executeWorkflow(ctx, ciframework.Workflow{Name: *workflow, Steps: []ciframework.Step{{
			Name: "command", Command: command[0], Args: command[1:], Dir: repository,
		}}}, executeOptions{Root: repository, Device: *device, Archive: true, ArchiveRoot: *root})
	case "finalize":
		flags := flag.NewFlagSet("archive finalize", flag.ContinueOnError)
		stage := flags.String("stage", "", "existing .partial stage directory")
		workflow := flags.String("workflow", "", "workflow name")
		revision := flags.String("revision", "", "source revision")
		runID := flags.String("run-id", "", "archive UUID")
		device := flags.String("device", envDefault("TETHUX_DEVICE_ID", ""), "device ID")
		runtimeName := flags.String("runtime", envDefault("TETHUX_CONTAINER_RUNTIME", ""), "runtime name")
		failed := flags.Bool("failed", false, "record the wrapped command as failed")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *stage == "" || *workflow == "" || *revision == "" || *runID == "" {
			return errors.New("archive finalize requires --stage, --workflow, --revision, and --run-id")
		}
		repository, err := ciframework.RepositoryRoot()
		if err != nil {
			return err
		}
		var commandErr error
		if *failed {
			commandErr = errors.New("provider reported a failed command")
		}
		writer, err := ciframework.OpenArchiveWriter(*stage, ciframework.ArchiveOptions{
			Repository: repository, Workflow: *workflow, Revision: *revision, RunID: *runID,
			DeviceID: *device, Runtime: *runtimeName, FinishedAt: time.Now().UTC(), CommandErr: commandErr,
		})
		if err != nil {
			return err
		}
		path, err := writer.Finalize(ctx)
		if err == nil {
			_, _ = fmt.Fprintln(os.Stdout, path)
		}
		return err
	case "publish":
		flags := flag.NewFlagSet("archive publish", flag.ContinueOnError)
		host := flags.String("host", envDefault("TETHUX_ARCHIVE_NAS_HOST", ""), "NAS SSH target")
		remoteRoot := flags.String("remote-root", envDefault("TETHUX_NAS_ARCHIVE_ROOT", "/var/cache/tethux-ci/archive"), "remote archive root")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *host == "" || len(flags.Args()) != 1 {
			return errors.New("archive publish requires --host and one archive path")
		}
		return publishArchive(ctx, flags.Args()[0], *host, *remoteRoot)
	case "inventory":
		flags := flag.NewFlagSet("archive inventory", flag.ContinueOnError)
		host := flags.String("host", "nas", "NAS SSH target")
		remoteRoot := flags.String("remote-root", "/var/cache/tethux-ci/archive", "remote archive root")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return inventoryArchives(ctx, *host, *remoteRoot, os.Stdout)
	default:
		return usageError("unknown archive command " + args[0])
	}
}

func hostCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("host requires discover, audit, or install")
	}
	switch args[0] {
	case "discover":
		flags := flag.NewFlagSet("host discover", flag.ContinueOnError)
		subnet := flags.String("subnet", "", "IPv4 /24 prefix, for example 10.0.0")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return discoverHosts(ctx, *subnet, os.Stdout)
	case "audit":
		flags := flag.NewFlagSet("host audit", flag.ContinueOnError)
		target := flags.String("host", envDefault("HOST", ""), "SSH target")
		jump := flags.String("jump-host", envDefault("TETHUX_SSH_JUMP", ""), "SSH jump host")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return auditHost(ctx, *target, *jump, os.Stdout)
	case "install":
		return installHost(ctx, args[1:])
	default:
		return usageError("unknown host command " + args[0])
	}
}

func topologyCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "container-udp" {
		return usageError("topology requires container-udp")
	}
	flags := flag.NewFlagSet("topology container-udp", flag.ContinueOnError)
	runtimeName := flags.String("runtime", "podman", "docker or podman")
	count := flags.Int("n", 4, "container count")
	parallel := flags.Int("parallel-jobs", 4, "parallel jobs")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	root, err := ciframework.RepositoryRoot()
	if err != nil {
		return err
	}
	step := ciframework.Step{
		Name: "container-udp", Command: "go",
		Args: []string{"run", "./tools/bridge/example/container-udp", "--runtime", *runtimeName, "--n", strconv.Itoa(*count), "--parallel-jobs", strconv.Itoa(*parallel)},
		Dir:  root, Privilege: ciframework.PrivilegeRoot,
	}
	_, err = ciframework.NewRunner(os.Stdout, os.Stderr).Run(ctx, ciframework.Workflow{Name: "container-udp", Steps: []ciframework.Step{step}})
	return err
}

func runRemoteLaptop(ctx context.Context, root, target, jump, runtimeName, archiveRoot, device string, archive, dryRun bool) error {
	if runtimeName != "docker" && runtimeName != "podman" {
		return errors.New("remote laptop requires --runtime docker or podman")
	}
	revision, err := commandOutput(root, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remoteDir := "/tmp/tethux-ci-" + revision[:12]
	remote := ciframework.Remote{Target: target, JumpHost: jump}
	if dryRun {
		fmt.Printf("stage %s at %s:%s and run laptop-%s\n", root, target, remoteDir, runtimeName)
		return nil
	}
	_ = remote.Run(ctx, io.Discard, io.Discard, "rm", "-rf", remoteDir)
	if err := remote.Run(ctx, os.Stdout, os.Stderr, "mkdir", "-p", remoteDir); err != nil {
		return err
	}
	defer func() {
		_ = remote.Run(context.WithoutCancel(ctx), io.Discard, io.Discard, "rm", "-rf", remoteDir)
	}()
	if err := streamRepository(ctx, remote, root, remoteDir); err != nil {
		return err
	}
	remoteArgs := []string{"nix", "develop", ".#integration", "--extra-experimental-features", "nix-command flakes", "-c", "go", "run", "./tools/ci", "run", "laptop", "--runtime", runtimeName}
	if device != "" {
		remoteArgs = append(remoteArgs, "--device", device)
	}
	if !archive {
		return remote.Run(ctx, os.Stdout, os.Stderr, append([]string{"env", "-C", remoteDir}, remoteArgs...)...)
	}

	started := time.Now().UTC()
	writer, err := ciframework.NewArchiveWriter(ciframework.ArchiveOptions{
		Root: archiveRoot, Repository: root, Workflow: "remote-laptop-" + runtimeName,
		DeviceID: device, Runtime: runtimeName, StartedAt: started,
	})
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(writer.LogDir(), "runner.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer logFile.Close()
	output := io.MultiWriter(os.Stdout, logFile)
	runErr := remote.Run(ctx, output, io.MultiWriter(os.Stderr, logFile), append([]string{"env", "-C", remoteDir}, remoteArgs...)...)
	copyErr := remote.CopyFrom(ctx, remoteDir+"/results/current/artifacts/.", writer.ArtifactDir(), output, os.Stderr)
	writer.Options.FinishedAt = time.Now().UTC()
	writer.Options.CommandErr = errors.Join(runErr, copyErr)
	if err := writeJSON(filepath.Join(writer.ConfigDir(), "remote.json"), map[string]any{
		"target": target, "jump_host": jump, "workspace": remoteDir, "runtime": runtimeName,
	}); err != nil {
		return errors.Join(runErr, copyErr, err)
	}
	archivePath, finalizeErr := writer.Finalize(ctx)
	if finalizeErr == nil {
		fmt.Fprintf(os.Stdout, "test archive: %s\n", archivePath)
	}
	return errors.Join(runErr, copyErr, finalizeErr)
}

func runCrossLaptop(ctx context.Context, root, hostA, hostB, archiveRoot, device string, archive, dryRun bool) error {
	workflow := ciframework.Workflow{Name: "cross-laptop", Steps: []ciframework.Step{{
		Name: "cross-link", Command: "go", Dir: root, Privilege: ciframework.PrivilegeUser,
		Args:          []string{"run", "./cmd/tethux", "virt", "link", "test", "--host-a", hostA, "--host-b", hostB, "--provider-a", "docker", "--provider-b", "podman"},
		CaptureStdout: filepath.Join(root, "results", "current", "artifacts", "cross-link.jsonl"),
		Timeout:       45 * time.Minute,
	}}}
	return executeWorkflow(ctx, workflow, executeOptions{Root: root, Device: device, Archive: archive, ArchiveRoot: archiveRoot, DryRun: dryRun})
}

func streamRepository(ctx context.Context, remote ciframework.Remote, root, destination string) error {
	tarCmd := exec.CommandContext(ctx, "tar", "--exclude=.git", "--exclude=.jj", "--exclude=bin", "--exclude=results", "-czf", "-", ".")
	tarCmd.Dir = root
	pipe, err := tarCmd.StdoutPipe()
	if err != nil {
		return err
	}
	sshArgs, err := remote.SSHArgs("tar", "-xzf", "-", "-C", destination)
	if err != nil {
		return err
	}
	sshCmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	sshCmd.Stdin, sshCmd.Stdout, sshCmd.Stderr = pipe, os.Stdout, os.Stderr
	if err := tarCmd.Start(); err != nil {
		return err
	}
	if err := sshCmd.Run(); err != nil {
		_ = tarCmd.Wait()
		return err
	}
	return tarCmd.Wait()
}

func publishArchive(ctx context.Context, archivePath, host, remoteRoot string) error {
	content, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`).MatchString(remoteRoot) || strings.Contains(remoteRoot, "..") {
		return errors.New("unsafe remote archive root")
	}
	relative := filepath.Join(filepath.Base(filepath.Dir(filepath.Dir(archivePath))), filepath.Base(filepath.Dir(archivePath)), filepath.Base(archivePath))
	remoteFinal := filepath.ToSlash(filepath.Join(remoteRoot, relative))
	remotePartial := remoteFinal + ".partial"
	remote := ciframework.Remote{Target: host}
	if err := remote.Run(ctx, os.Stdout, os.Stderr, "mkdir", "-p", filepath.Dir(remoteFinal)); err != nil {
		return err
	}
	if err := remote.CopyTo(ctx, archivePath, remotePartial, os.Stdout, os.Stderr); err != nil {
		return err
	}
	var output bytes.Buffer
	if err := remote.Run(ctx, &output, os.Stderr, "sha256sum", remotePartial); err != nil {
		return err
	}
	expected := fmt.Sprintf("%x", sha256.Sum256(content))
	if !strings.HasPrefix(output.String(), expected) {
		return errors.New("remote archive checksum mismatch")
	}
	if err := remote.Run(ctx, os.Stdout, os.Stderr, "mv", remotePartial, remoteFinal); err != nil {
		return err
	}
	localMarker := archivePath + ".done"
	if err := os.WriteFile(localMarker, []byte(expected+"\n"), 0o600); err != nil {
		return err
	}
	remoteMarkerPartial := remoteFinal + ".done.partial"
	if err := remote.CopyTo(ctx, localMarker, remoteMarkerPartial, os.Stdout, os.Stderr); err != nil {
		return err
	}
	if err := remote.Run(ctx, os.Stdout, os.Stderr, "mv", remoteMarkerPartial, remoteFinal+".done"); err != nil {
		return err
	}
	fmt.Printf("published test archive: %s:%s\n", host, remoteFinal)
	return nil
}

func inventoryArchives(ctx context.Context, host, remoteRoot string, output io.Writer) error {
	remote := ciframework.Remote{Target: host}
	fmt.Fprintf(output, "# NAS test archives\n\nRoot: `%s:%s`\n\n", host, remoteRoot)
	return remote.Run(ctx, output, os.Stderr, "find", remoteRoot, "-type", "f", "-name", "*.tar.zst", "-print")
}

func discoverHosts(ctx context.Context, subnet string, output io.Writer) error {
	if subnet == "" {
		value, err := commandOutput("", "ip", "-4", "-o", "route", "show", "scope", "link")
		if err != nil {
			return err
		}
		match := regexp.MustCompile(`\b(\d+\.\d+\.\d+)\.\d+/\d+`).FindStringSubmatch(value)
		if len(match) < 2 {
			return errors.New("could not infer a local IPv4 subnet; pass --subnet")
		}
		subnet = match[1]
	}
	for suffix := 1; suffix < 255; suffix++ {
		address := fmt.Sprintf("%s.%d", subnet, suffix)
		connection, err := (&net.Dialer{Timeout: 35 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort(address, "22"))
		if err == nil {
			_ = connection.Close()
			_, _ = fmt.Fprintln(output, address)
		}
	}
	return ctx.Err()
}

func auditHost(ctx context.Context, target, jump string, output io.Writer) error {
	if target == "" {
		return errors.New("--host is required")
	}
	remote := ciframework.Remote{Target: target, JumpHost: jump}
	commands := [][]string{
		{"hostname"},
		{"uname", "-a"},
		{"ip", "-brief", "addr"},
		{"lsblk", "-o", "NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS,MODEL"},
		{"systemd-detect-virt"},
		{"go", "version"},
		{"docker", "--version"},
		{"podman", "--version"},
		{"ctr", "version"},
	}
	var auditErr error
	for _, command := range commands {
		fmt.Fprintf(output, "\n== %s ==\n", strings.Join(command, " "))
		if err := remote.Run(ctx, output, os.Stderr, command...); err != nil {
			fmt.Fprintf(output, "unavailable: %v\n", err)
			auditErr = errors.Join(auditErr, err)
		}
	}
	return auditErr
}

func installHost(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("host install", flag.ContinueOnError)
	target := flags.String("host", "", "target SSH host")
	flakeHost := flags.String("flake-host", "", "Nix flake host")
	disk := flags.String("disk", envDefault("TETHUX_INSTALL_DISK", ""), "whole target disk")
	jump := flags.String("jump-host", envDefault("TETHUX_SSH_JUMP", ""), "SSH jump host")
	expectSize := flags.Int64("expect-size", envInt64("TETHUX_EXPECT_DISK_SIZE_BYTES"), "expected disk size in bytes")
	expectVirt := flags.String("expect-virtualization", envDefault("TETHUX_EXPECT_VIRTUALIZATION", ""), "expected virtualization")
	yes := flags.Bool("yes", false, "confirm destructive installation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *target == "" || *flakeHost == "" || !regexp.MustCompile(`^/dev/(sd[a-z]+|vd[a-z]+|nvme\d+n\d+|mmcblk\d+)$`).MatchString(*disk) {
		return errors.New("--host, --flake-host, and a safe whole --disk are required")
	}
	remote := ciframework.Remote{Target: *target, JumpHost: *jump}
	var kind, size, virtualization bytes.Buffer
	if err := remote.Run(ctx, &kind, os.Stderr, "lsblk", "-dnro", "TYPE", *disk); err != nil {
		return err
	}
	if strings.TrimSpace(kind.String()) != "disk" {
		return fmt.Errorf("refusing %s because it is not a whole disk", *disk)
	}
	if err := remote.Run(ctx, &size, os.Stderr, "blockdev", "--getsize64", *disk); err != nil {
		return err
	}
	actualSize, err := strconv.ParseInt(strings.TrimSpace(size.String()), 10, 64)
	if err != nil {
		return err
	}
	_ = remote.Run(ctx, &virtualization, io.Discard, "systemd-detect-virt")
	if *expectSize > 0 && actualSize != *expectSize {
		return fmt.Errorf("disk size is %d, expected %d", actualSize, *expectSize)
	}
	if *expectVirt != "" && strings.TrimSpace(virtualization.String()) != *expectVirt {
		return fmt.Errorf("virtualization is %q, expected %q", strings.TrimSpace(virtualization.String()), *expectVirt)
	}
	fmt.Printf("DESTRUCTIVE INSTALL\nTarget: %s\nDisk: %s (%d bytes)\nFlake: %s\n", *target, *disk, actualSize, *flakeHost)
	if !*yes {
		return errors.New("refusing destructive installation without --yes")
	}
	root, err := ciframework.RepositoryRoot()
	if err != nil {
		return err
	}
	installHost := *flakeHost
	if !strings.HasSuffix(installHost, "-install") {
		installHost += "-install"
	}
	commandArgs := []string{"run", "github:nix-community/nixos-anywhere", "--", "--flake", root + "#" + installHost, "--option", "pure-eval", "false", "--build-on", "remote"}
	if *jump != "" {
		commandArgs = append(commandArgs, "--ssh-option", "ProxyJump="+*jump)
	}
	commandArgs = append(commandArgs, *target)
	cmd := exec.CommandContext(ctx, "nix", commandArgs...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TETHUX_INSTALL_DISK="+*disk)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func checkFixtureRegistry(ctx context.Context) error {
	registry := envDefault("TETHUX_FIXTURE_REGISTRY", "http://127.0.0.1:5000")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, registry+"/v2/", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("fixture registry unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("fixture registry returned %s", response.Status)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(value)
	return errors.Join(encodeErr, file.Close())
}

func commandOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string) int64 {
	value, _ := strconv.ParseInt(os.Getenv(key), 10, 64)
	return value
}

type usageErr struct{ message string }

func (e usageErr) Error() string { return e.message }
func usageError(message string) error {
	if message != "" {
		message += "\n"
	}
	return usageErr{message + "run `tethux-ci help` for usage"}
}

func exitCode(err error) int {
	var value usageErr
	if errors.As(err, &value) {
		return 2
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func printUsage(output io.Writer) {
	_, _ = fmt.Fprintln(output, `usage: tethux-ci GROUP COMMAND [flags]

groups:
  run       normal, laptop, local, remote-laptop, cross-laptop,
            provider, topology, bridge, hypervisors
  archive   run, finalize, publish, inventory
  host      discover, audit, install
  topology  container-udp

All commands use the standard library flag parser. Flags override environment
defaults. Use -h after a command for its complete flag list.`)
}
