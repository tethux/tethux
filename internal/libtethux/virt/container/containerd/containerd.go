package containerd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/0xveya/tethux/internal/libtethux/virt/container"
	"github.com/0xveya/tethux/internal/libtethux/virt/container/errs"
	containersspecs "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	mobyclient "github.com/moby/moby/client"
)

var _ container.ContainerProvider = (*Containerd)(nil)

const namespace = "default"

var execSequence atomic.Uint64

type Option func(*config)

type config struct {
	socketOverride string
}

type Containerd struct {
	cli    *containerd.Client
	socket string
}

func (c *Containerd) Socket() string { return c.socket }

func WithSocket(s string) Option {
	return func(c *config) { c.socketOverride = s }
}

func checkSocket(addr string) error {
	path := addr
	if after, ok := strings.CutPrefix(addr, "unix://"); ok {
		path = after
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%q exists but is not a socket", path)
	}
	return nil
}

func New(opts ...Option) (*Containerd, error) {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	socket, err := resolveSocket(cfg)
	if err != nil {
		return nil, err
	}

	cli, err := containerd.New(socket)
	if err != nil {
		return nil, fmt.Errorf("containerd: %w: %q: %w", errs.ErrFailedToCreateClent, socket, err)
	}

	return &Containerd{cli: cli, socket: socket}, nil
}

func resolveSocket(cfg *config) (string, error) {
	if cfg.socketOverride != "" {
		if err := checkSocket(cfg.socketOverride); err != nil {
			return "", fmt.Errorf("containerd: %w: %q err: %w", errs.ErrOverrideSocketNotAccessible, cfg.socketOverride, err)
		}
		return cfg.socketOverride, nil
	}

	if val := os.Getenv("CONTAINERD_ADDRESS"); val != "" {
		if err := checkSocket(val); err == nil {
			return val, nil
		}
	}

	for _, candidate := range socketCandidates() {
		if err := checkSocket(candidate.socket); err == nil {
			return candidate.socket, nil
		}
	}

	return "", fmt.Errorf("containerd: %w; tried %v — is containerd running?", errs.ErrNoSockerFound, socketPaths())
}

type socketCandidate struct {
	label  string
	socket string
}

func socketCandidates() []socketCandidate {
	var candidates []socketCandidate

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates,
			socketCandidate{
				label:  "rootless/XDG_RUNTIME_DIR",
				socket: filepath.Join(xdg, "containerd", "containerd.sock"),
			},
			socketCandidate{
				label:  "rootless/XDG_RUNTIME_DIR-legacy",
				socket: filepath.Join(xdg, "containerd-rootless", "containerd.sock"),
			},
		)
	}

	if uid := os.Getuid(); uid > 0 {
		candidates = append(candidates,
			socketCandidate{
				label:  "rootless/run-user",
				socket: fmt.Sprintf("/run/user/%d/containerd/containerd.sock", uid),
			},
			socketCandidate{
				label:  "rootless/run-user-legacy",
				socket: fmt.Sprintf("/run/user/%d/containerd-rootless/containerd.sock", uid),
			},
		)
	}

	candidates = append(candidates,
		socketCandidate{label: "rootful/run-containerd", socket: "/run/containerd/containerd.sock"},
		socketCandidate{label: "rootful/var-run-containerd", socket: "/var/run/containerd/containerd.sock"},
	)

	return candidates
}

func socketPaths() []string {
	candidates := socketCandidates()
	paths := make([]string, 0, len(candidates))
	for _, c := range candidates {
		paths = append(paths, c.socket)
	}
	return paths
}

func (c *Containerd) Pull(ctx context.Context, ref string, _ *mobyclient.ImagePullOptions) error {
	ctx = withNamespace(ctx)
	_, err := c.cli.Pull(ctx, ref, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToPullImage, ref, err)
	}
	return nil
}

func (c *Containerd) CreateContainer(ctx context.Context, cfg *container.ContainerConfig) (*container.ContainerNode, error) {
	if cfg == nil {
		return nil, fmt.Errorf("containerd: create: cfg is nil")
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("containerd: create: name is required")
	}
	ctx = withNamespace(ctx)

	img, err := c.cli.GetImage(ctx, cfg.Image)
	if err != nil {
		return nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToCreateContainer, cfg.Image, err)
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(img),
	}

	if len(cfg.Entrypoint) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(append(cfg.Entrypoint, cfg.Cmd...)...))
	} else if len(cfg.Cmd) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(cfg.Cmd...))
	}

	if len(cfg.Env) > 0 {
		specOpts = append(specOpts, oci.WithEnv(cfg.Env))
	}

	if cfg.Hostname != "" {
		specOpts = append(specOpts, oci.WithHostname(cfg.Hostname))
	}

	if cfg.Privileged {
		specOpts = append(specOpts, oci.WithPrivileged)
	}

	if len(cfg.CapAdd) > 0 {
		specOpts = append(specOpts, oci.WithAddedCapabilities(cfg.CapAdd))
	}

	if len(cfg.CapDrop) > 0 {
		specOpts = append(specOpts, oci.WithDroppedCapabilities(cfg.CapDrop))
	}

	if len(cfg.Labels) > 0 {
		specOpts = append(specOpts, oci.WithAnnotations(cfg.Labels))
	}

	// volumes as bind mounts
	if len(cfg.Volumes) > 0 {
		mounts := make([]specs.Mount, len(cfg.Volumes))
		for i, v := range cfg.Volumes {
			opts := []string{"rbind", "rw"}
			if v.ReadOnly {
				opts = []string{"rbind", "ro"}
			}
			mounts[i] = specs.Mount{
				Source:      v.Source,
				Destination: v.Target,
				Type:        "bind",
				Options:     opts,
			}
		}
		specOpts = append(specOpts, oci.WithMounts(mounts))
	}

	// Containerd itself does not configure CNI for direct client-created tasks.
	// Host mode is still useful for the topology runner, while isolated mode
	// gets the runtime's default network namespace.
	specOpts = append(specOpts, func(ctx context.Context, _ oci.Client, _ *containersspecs.Container, s *specs.Spec) error {
		if cfg.NetworkMode == "host" {
			s.Linux.Namespaces = slices.DeleteFunc(s.Linux.Namespaces, func(ns specs.LinuxNamespace) bool {
				return ns.Type == specs.NetworkNamespace
			})
		}
		return nil
	})

	configMounts, err := writeNetworkFiles(cfg)
	if err != nil {
		return nil, err
	}
	if len(configMounts) > 0 {
		specOpts = append(specOpts, oci.WithMounts(configMounts))
	}

	snapshotID := cfg.Name + "-snapshot"

	ctr, err := c.cli.NewContainer(ctx, cfg.Name,
		containerd.WithImage(img),
		containerd.WithNewSnapshot(snapshotID, img),
		containerd.WithSnapshotter(defaults.DefaultSnapshotter),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(cfg.Labels),
	)
	if err != nil {
		return nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToCreateContainer, cfg.Name, err)
	}

	return &container.ContainerNode{
		Node: virt.Node{
			ID:    ctr.ID(),
			Name:  cfg.Name,
			State: virt.NodeStopped,
		},
		ImageName: cfg.Image,
		Labels:    cfg.Labels,
	}, nil
}

func (c *Containerd) Name() string { return "containerd" }

func (c *Containerd) Create(ctx context.Context, cfg *virt.NodeConfig) (*virt.Node, error) {
	if cfg == nil {
		return nil, fmt.Errorf("containerd: create: cfg is nil")
	}
	node, err := c.CreateContainer(ctx, &container.ContainerConfig{NodeConfig: *cfg})
	if err != nil {
		return nil, err
	}
	return &node.Node, nil
}

func (c *Containerd) Start(ctx context.Context, id string) error {
	return c.StartContainer(ctx, id, nil)
}

func (c *Containerd) Stop(ctx context.Context, id string) error {
	return c.StopContainer(ctx, id, nil)
}

func (c *Containerd) Suspend(ctx context.Context, id string) error {
	return c.SuspendContainer(ctx, id, nil)
}

func (c *Containerd) Resume(ctx context.Context, id string) error {
	return c.ResumeContainer(ctx, id, nil)
}

func (c *Containerd) Delete(ctx context.Context, id string) error {
	return c.DeleteContainer(ctx, id, nil)
}

func (c *Containerd) Restart(ctx context.Context, id string) error {
	return c.RestartContainer(ctx, id, nil)
}

func (c *Containerd) StartContainer(ctx context.Context, id string, _ *mobyclient.ContainerStartOptions) error {
	ctx = withNamespace(ctx)
	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStartContainer, id, err)
	}

	task, err := ctr.Task(ctx, nil)
	if errdefs.IsNotFound(err) {
		if mkdirErr := os.MkdirAll(filepath.Dir(logPath(id)), 0o700); mkdirErr != nil {
			return fmt.Errorf("containerd: prepare logs %q: %w", id, mkdirErr)
		}
		task, err = ctr.NewTask(ctx, cio.LogFile(logPath(id)))
	}
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStartContainer, id, err)
	}
	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("containerd: status before start %q: %w", id, err)
	}
	if status.Status == containerd.Running {
		return nil
	}
	if status.Status != containerd.Created {
		if _, deleteErr := task.Delete(ctx); deleteErr != nil && !errdefs.IsNotFound(deleteErr) {
			return fmt.Errorf("containerd: replace stopped task %q: %w", id, deleteErr)
		}
		task, err = ctr.NewTask(ctx, cio.LogFile(logPath(id)))
		if err != nil {
			return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStartContainer, id, err)
		}
	}
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStartContainer, id, err)
	}
	return nil
}

func (c *Containerd) StopContainer(ctx context.Context, id string, _ *mobyclient.ContainerStopOptions) error {
	ctx = withNamespace(ctx)
	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStopContainer, id, err)
	}
	task, err := ctr.Task(ctx, nil)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStopContainer, id, err)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return fmt.Errorf("containerd: status before stop %q: %w", id, err)
	}
	if status.Status == containerd.Paused {
		if err := task.Resume(ctx); err != nil {
			return fmt.Errorf("containerd: resume before stop %q: %w", id, err)
		}
	}
	if status.Status == containerd.Running {
		wait, waitErr := task.Wait(ctx)
		if waitErr != nil {
			return fmt.Errorf("containerd: wait before stop %q: %w", id, waitErr)
		}
		if err := task.Kill(ctx, syscall.SIGTERM); err != nil {
			return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToStopContainer, id, err)
		}
		select {
		case <-wait:
		case <-time.After(10 * time.Second):
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				return fmt.Errorf("containerd: force stop %q: %w", id, err)
			}
			<-wait
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if _, err := task.Delete(ctx); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("containerd: delete stopped task %q: %w", id, err)
	}
	return nil
}

func (c *Containerd) SuspendContainer(ctx context.Context, id string, _ *mobyclient.ContainerPauseOptions) error {
	task, err := c.loadTask(ctx, id)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToSuspendContainer, id, err)
	}
	if err := task.Pause(withNamespace(ctx)); err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToSuspendContainer, id, err)
	}
	return nil
}

func (c *Containerd) ResumeContainer(ctx context.Context, id string, _ *mobyclient.ContainerUnpauseOptions) error {
	task, err := c.loadTask(ctx, id)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToResumeContainer, id, err)
	}
	if err := task.Resume(withNamespace(ctx)); err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToResumeContainer, id, err)
	}
	return nil
}

func (c *Containerd) RestartContainer(ctx context.Context, id string, _ *mobyclient.ContainerRestartOptions) error {
	if err := c.Stop(ctx, id); err != nil {
		return err
	}
	return c.Start(ctx, id)
}

func (c *Containerd) DeleteContainer(ctx context.Context, id string, _ *mobyclient.ContainerRemoveOptions) error {
	ctx = withNamespace(ctx)
	ctr, err := c.cli.LoadContainer(ctx, id)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToDeleteContainer, id, err)
	}
	if task, taskErr := ctr.Task(ctx, nil); taskErr == nil {
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	} else if !errdefs.IsNotFound(taskErr) {
		return fmt.Errorf("containerd: load task for delete %q: %w", id, taskErr)
	}
	if err := ctr.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToDeleteContainer, id, err)
	}
	_ = os.Remove(logPath(id))
	_ = os.RemoveAll(configPath(id))
	return nil
}

func (c *Containerd) State(ctx context.Context, id string) (virt.NodeState, error) {
	task, err := c.loadTask(ctx, id)
	if errdefs.IsNotFound(err) {
		return virt.NodeStopped, nil
	}
	if err != nil {
		return "", fmt.Errorf("containerd: state %q: %w", id, err)
	}
	status, err := task.Status(withNamespace(ctx))
	if err != nil {
		return "", fmt.Errorf("containerd: state %q: %w", id, err)
	}
	return mapState(status.Status), nil
}

func (c *Containerd) Reload(ctx context.Context, id string) (*virt.Node, error) {
	node, err := c.Inspect(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return &node.Node, nil
}

func (c *Containerd) List(ctx context.Context) ([]*virt.Node, error) {
	ctx = withNamespace(ctx)
	containers, err := c.cli.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("containerd: list: %w", err)
	}
	nodes := make([]*virt.Node, 0, len(containers))
	for _, ctr := range containers {
		state, stateErr := c.State(ctx, ctr.ID())
		if stateErr != nil {
			return nil, stateErr
		}
		nodes = append(nodes, &virt.Node{ID: ctr.ID(), Name: ctr.ID(), State: state})
	}
	return nodes, nil
}

func (c *Containerd) Inspect(ctx context.Context, id string, _ *mobyclient.ContainerInspectOptions) (*container.ContainerNode, error) {
	ctx = withNamespace(ctx)
	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToInspectContainer, id, err)
	}
	info, err := ctr.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToInspectContainer, id, err)
	}
	state, err := c.State(ctx, id)
	if err != nil {
		return nil, err
	}
	imageID := ""
	if image, imageErr := ctr.Image(ctx); imageErr == nil {
		imageID = image.Target().Digest.String()
	}
	return &container.ContainerNode{
		Node:      virt.Node{ID: id, Name: id, State: state},
		ImageID:   imageID,
		ImageName: info.Image,
		Labels:    info.Labels,
	}, nil
}

func (c *Containerd) Exec(ctx context.Context, id string, cmd []string, _ *mobyclient.ExecCreateOptions, _ *mobyclient.ExecAttachOptions) (stdoutBytes, stderrBytes []byte, resultErr error) {
	ctx = withNamespace(ctx)
	task, err := c.loadTask(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToCreateExec, id, err)
	}
	taskSpec, err := task.Spec(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("containerd: exec spec %q: %w", id, err)
	}
	process := *taskSpec.Process
	process.Args = slices.Clone(cmd)
	process.Terminal = false
	if process.Cwd == "" {
		process.Cwd = "/"
	}
	var stdout, stderr bytes.Buffer
	execID := fmt.Sprintf("tethux-%d-%d", time.Now().UnixNano(), execSequence.Add(1))
	proc, err := task.Exec(ctx, execID, &process, cio.NewCreator(cio.WithStreams(nil, &stdout, &stderr)))
	if err != nil {
		return nil, nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToCreateExec, id, err)
	}
	wait, err := proc.Wait(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("containerd: exec wait %q: %w", id, err)
	}
	if err := proc.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToAttachExec, id, err)
	}
	status := <-wait
	_, deleteErr := proc.Delete(ctx)
	if deleteErr != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("containerd: delete exec %q: %w", id, deleteErr)
	}
	code, _, resultErr := status.Result()
	if resultErr != nil {
		return stdout.Bytes(), stderr.Bytes(), resultErr
	}
	if code != 0 {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("containerd: exec %q exited with status %d", id, code)
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

func (c *Containerd) Logs(_ context.Context, id string, _ *mobyclient.ContainerLogsOptions) (io.ReadCloser, error) {
	f, err := os.Open(logPath(id))
	if err != nil {
		return nil, fmt.Errorf("containerd: logs %q: %w", id, err)
	}
	return f, nil
}

func (c *Containerd) loadTask(ctx context.Context, id string) (containerd.Task, error) {
	ctx = withNamespace(ctx)
	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}
	return ctr.Task(ctx, nil)
}

func withNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, namespace)
}

func mapState(status containerd.ProcessStatus) virt.NodeState {
	switch status {
	case containerd.Running:
		return virt.NodeRunning
	case containerd.Created:
		return virt.NodeStarting
	case containerd.Paused, containerd.Pausing:
		return virt.NodeSuspended
	default:
		return virt.NodeStopped
	}
}

func stateDir() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "tethux", "containerd")
	}
	return filepath.Join(os.TempDir(), "tethux-containerd")
}

func logPath(id string) string { return filepath.Join(stateDir(), "logs", filepath.Base(id)+".log") }

func configPath(id string) string { return filepath.Join(stateDir(), "config", filepath.Base(id)) }

func writeNetworkFiles(cfg *container.ContainerConfig) ([]specs.Mount, error) {
	var mounts []specs.Mount
	dir := configPath(cfg.Name)
	if len(cfg.ExtraHosts) > 0 {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("containerd: create host config: %w", err)
		}
		content := "127.0.0.1 localhost\n::1 localhost\n"
		for _, entry := range cfg.ExtraHosts {
			host, address, ok := strings.Cut(entry, ":")
			if !ok {
				return nil, fmt.Errorf("containerd: invalid extra host %q (expected host:address)", entry)
			}
			content += address + " " + host + "\n"
		}
		path := filepath.Join(dir, "hosts")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return nil, fmt.Errorf("containerd: write hosts: %w", err)
		}
		mounts = append(mounts, specs.Mount{Source: path, Destination: "/etc/hosts", Type: "bind", Options: []string{"rbind", "ro"}})
	}
	if len(cfg.DNS) > 0 {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("containerd: create DNS config: %w", err)
		}
		var content strings.Builder
		for _, address := range cfg.DNS {
			fmt.Fprintf(&content, "nameserver %s\n", address)
		}
		path := filepath.Join(dir, "resolv.conf")
		if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
			return nil, fmt.Errorf("containerd: write resolv.conf: %w", err)
		}
		mounts = append(mounts, specs.Mount{Source: path, Destination: "/etc/resolv.conf", Type: "bind", Options: []string{"rbind", "ro"}})
	}
	return mounts, nil
}
