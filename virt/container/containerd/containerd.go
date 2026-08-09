package containerd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	containersspecs "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/content"
	dockerremotes "github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	mobyclient "github.com/moby/moby/client"
	imagespecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/tethux/tethux/virt"
	"github.com/tethux/tethux/virt/container"
	"github.com/tethux/tethux/virt/container/errs"
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

func (c *Containerd) Info() virt.ProviderInfo {
	return virt.ProviderInfo{
		Name:        "containerd",
		DisplayName: "Containerd",
		Kind:        virt.ProviderKindContainer,
		Capabilities: virt.Capabilities{
			Exec:    true,
			Logs:    true,
			Pause:   true,
			Console: true,
		},
	}
}

func (c *Containerd) Socket() string {
	return c.socket
}

func (c *Containerd) Close() error {
	if c.cli == nil {
		return nil
	}

	return c.cli.Close()
}

func WithSocket(socket string) Option {
	return func(c *config) {
		c.socketOverride = socket
	}
}

func New(opts ...Option) (*Containerd, error) {
	cfg := &config{}

	for _, opt := range opts {
		opt(cfg)
	}

	socket, err := resolveSocket(cfg)
	if err != nil {
		return nil, err
	}

	cli, err := containerd.New(socket)
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateClient,
			socket,
			err,
		)
	}

	return &Containerd{
		cli:    cli,
		socket: socket,
	}, nil
}

func resolveSocket(cfg *config) (string, error) {
	if cfg.socketOverride != "" {
		if err := checkSocket(cfg.socketOverride); err != nil {
			return "", errs.Wrap(
				"containerd",
				errs.ErrOverrideSocketNotAccessible,
				cfg.socketOverride,
				err,
			)
		}

		return cfg.socketOverride, nil
	}

	if value := os.Getenv("CONTAINERD_ADDRESS"); value != "" {
		if err := checkSocket(value); err == nil {
			return value, nil
		}
	}

	for _, candidate := range socketCandidates() {
		if err := checkSocket(candidate.socket); err == nil {
			return candidate.socket, nil
		}
	}

	return "", errs.New(
		"containerd",
		errs.ErrNoSocketFound,
		strings.Join(socketPaths(), ", "),
	)
}

type socketCandidate struct {
	label  string
	socket string
}

func socketCandidates() []socketCandidate {
	var candidates []socketCandidate

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(
			candidates,
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
		candidates = append(
			candidates,
			socketCandidate{
				label: "rootless/run-user",
				socket: fmt.Sprintf(
					"/run/user/%d/containerd/containerd.sock",
					uid,
				),
			},
			socketCandidate{
				label: "rootless/run-user-legacy",
				socket: fmt.Sprintf(
					"/run/user/%d/containerd-rootless/containerd.sock",
					uid,
				),
			},
		)
	}

	candidates = append(
		candidates,
		socketCandidate{
			label:  "rootful/run-containerd",
			socket: "/run/containerd/containerd.sock",
		},
		socketCandidate{
			label:  "rootful/var-run-containerd",
			socket: "/var/run/containerd/containerd.sock",
		},
	)

	return candidates
}

func socketPaths() []string {
	candidates := socketCandidates()

	paths := make([]string, 0, len(candidates))

	for _, candidate := range candidates {
		paths = append(paths, candidate.socket)
	}

	return paths
}

func checkSocket(addr string) error {
	path := addr

	if value, ok := strings.CutPrefix(addr, "unix://"); ok {
		path = value
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSocket == 0 {
		return errs.New(
			"containerd",
			errs.ErrNotASocket,
			path,
		)
	}

	return nil
}

func (c *Containerd) Pull(
	ctx context.Context,
	ref string,
	_ *mobyclient.ImagePullOptions,
) error {
	ctx = withNamespace(ctx)

	pullOpts := []containerd.RemoteOpt{
		containerd.WithPullUnpack,
	}

	if isLoopbackRegistry(ref) {
		pullOpts = append(
			pullOpts,
			containerd.WithResolver(
				dockerremotes.NewResolver(
					dockerremotes.ResolverOptions{
						PlainHTTP: true,
					},
				),
			),
		)
	}

	_, err := c.cli.Pull(ctx, ref, pullOpts...)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToPullImage,
			ref,
			err,
		)
	}

	return nil
}

func isLoopbackRegistry(ref string) bool {
	host, _, ok := strings.Cut(ref, "/")
	if !ok {
		return false
	}

	hostname, _, splitErr := net.SplitHostPort(host)
	if splitErr != nil {
		hostname = host
	}

	if hostname == "localhost" {
		return true
	}

	address, err := netip.ParseAddr(
		strings.Trim(hostname, "[]"),
	)

	return err == nil && address.IsLoopback()
}

func (c *Containerd) CreateContainer(
	ctx context.Context,
	cfg *container.RuntimeConfig,
) (*container.ContainerNode, error) {
	if cfg == nil {
		return nil, errs.New(
			"containerd",
			errs.ErrInvalidConfig,
			"config is nil",
		)
	}

	if cfg.Name == "" {
		return nil, errs.New(
			"containerd",
			errs.ErrInvalidConfig,
			"name is required",
		)
	}

	imageRef := cfg.Image.String()
	if imageRef == "" {
		return nil, errs.New(
			"containerd",
			errs.ErrInvalidConfig,
			"image is required",
		)
	}

	ctx = withNamespace(ctx)

	img, err := c.cli.GetImage(ctx, imageRef)
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateContainer,
			imageRef,
			err,
		)
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(img),
	}

	if isRootlessSocket(c.socket) {
		specOpts, err = rootlessImageSpecOpts(ctx, img)
		if err != nil {
			return nil, errs.Wrap(
				"containerd",
				errs.ErrFailedToCreateContainer,
				cfg.Name,
				err,
			)
		}

		specOpts = append(
			specOpts,
			oci.WithCgroup(
				"user.slice:tethux:"+cfg.Name,
			),
			oci.WithLinuxNamespace(
				specs.LinuxNamespace{
					Type: specs.CgroupNamespace,
				},
			),
		)
	}

	if len(cfg.Entrypoint) > 0 {
		args := make(
			[]string,
			0,
			len(cfg.Entrypoint)+len(cfg.Cmd),
		)

		args = append(args, cfg.Entrypoint...)
		args = append(args, cfg.Cmd...)

		specOpts = append(
			specOpts,
			oci.WithProcessArgs(args...),
		)
	} else if len(cfg.Cmd) > 0 {
		specOpts = append(
			specOpts,
			oci.WithProcessArgs(cfg.Cmd...),
		)
	}

	if len(cfg.Env) > 0 {
		specOpts = append(
			specOpts,
			oci.WithEnv(cfg.Env),
		)
	}

	if cfg.Hostname != "" {
		specOpts = append(
			specOpts,
			oci.WithHostname(cfg.Hostname),
		)
	}

	if cfg.Privileged {
		specOpts = append(
			specOpts,
			oci.WithPrivileged,
		)
	}

	if len(cfg.CapAdd) > 0 {
		specOpts = append(
			specOpts,
			oci.WithAddedCapabilities(cfg.CapAdd),
		)
	}

	if len(cfg.CapDrop) > 0 {
		specOpts = append(
			specOpts,
			oci.WithDroppedCapabilities(cfg.CapDrop),
		)
	}

	if len(cfg.Labels) > 0 {
		specOpts = append(
			specOpts,
			oci.WithAnnotations(cfg.Labels),
		)
	}

	if len(cfg.Volumes) > 0 {
		mounts := make(
			[]specs.Mount,
			0,
			len(cfg.Volumes),
		)

		for _, volume := range cfg.Volumes {
			if volume.Source == "" {
				return nil, errs.New(
					"containerd",
					errs.ErrInvalidConfig,
					"volume source is empty",
				)
			}

			if volume.Target == "" {
				return nil, errs.New(
					"containerd",
					errs.ErrInvalidConfig,
					"volume target is empty",
				)
			}

			opts := []string{
				"rbind",
				"rw",
			}

			if volume.ReadOnly {
				opts = []string{
					"rbind",
					"ro",
				}
			}

			mounts = append(
				mounts,
				specs.Mount{
					Source:      volume.Source,
					Destination: volume.Target,
					Type:        "bind",
					Options:     opts,
				},
			)
		}

		specOpts = append(
			specOpts,
			oci.WithMounts(mounts),
		)
	}

	specOpts = append(
		specOpts,
		func(
			_ context.Context,
			_ oci.Client,
			_ *containersspecs.Container,
			spec *specs.Spec,
		) error {
			if cfg.NetworkMode != "host" {
				return nil
			}

			if spec.Linux == nil {
				return nil
			}

			spec.Linux.Namespaces = slices.DeleteFunc(
				spec.Linux.Namespaces,
				func(ns specs.LinuxNamespace) bool {
					return ns.Type == specs.NetworkNamespace
				},
			)

			return nil
		},
	)

	configMounts, err := writeNetworkFiles(cfg)
	if err != nil {
		return nil, err
	}

	if len(configMounts) > 0 {
		specOpts = append(
			specOpts,
			oci.WithMounts(configMounts),
		)
	}

	snapshotID := cfg.Name + "-snapshot"

	ctr, err := c.cli.NewContainer(
		ctx,
		cfg.Name,
		containerd.WithImage(img),
		containerd.WithNewSnapshot(
			snapshotID,
			img,
		),
		containerd.WithSnapshotter(
			defaults.DefaultSnapshotter,
		),
		containerd.WithNewSpec(specOpts...),
		containerd.WithContainerLabels(cfg.Labels),
	)
	if err != nil {
		_ = os.RemoveAll(configPath(cfg.Name))

		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateContainer,
			cfg.Name,
			err,
		)
	}

	return &container.ContainerNode{
		Node: virt.Node{
			ID:    ctr.ID(),
			Name:  cfg.Name,
			State: virt.NodeStopped,
		},
		ImageName: imageRef,
		Labels:    cfg.Labels,
	}, nil
}

// rootlessImageSpecOpts reads OCI image configuration directly because
// rootlesskit may hide the snapshot mount. runc applies UID/GID mappings
// when creating the task.
func rootlessImageSpecOpts(
	ctx context.Context,
	img containerd.Image,
) ([]oci.SpecOpts, error) {
	descriptor, err := img.Config(ctx)
	if err != nil {
		return nil, err
	}

	data, err := content.ReadBlob(
		ctx,
		img.ContentStore(),
		descriptor,
	)
	if err != nil {
		return nil, err
	}

	var image imagespecs.Image

	if err := json.Unmarshal(data, &image); err != nil {
		return nil, err
	}

	config := image.Config

	opts := []oci.SpecOpts{
		oci.WithEnv(config.Env),
	}

	args := make(
		[]string,
		0,
		len(config.Entrypoint)+len(config.Cmd),
	)

	args = append(args, config.Entrypoint...)
	args = append(args, config.Cmd...)

	if len(args) > 0 {
		opts = append(
			opts,
			oci.WithProcessArgs(args...),
		)
	}

	if config.WorkingDir != "" {
		opts = append(
			opts,
			oci.WithProcessCwd(config.WorkingDir),
		)
	}

	if config.User != "" {
		user, group, hasGroup := strings.Cut(
			config.User,
			":",
		)

		uid, parseErr := strconv.ParseUint(
			user,
			10,
			32,
		)
		if parseErr != nil {
			return nil, errs.New(
				"containerd",
				errs.ErrInvalidConfig,
				"rootless image user must be numeric: "+config.User,
			)
		}

		var gid uint64

		if hasGroup && group != "" {
			gid, parseErr = strconv.ParseUint(
				group,
				10,
				32,
			)
			if parseErr != nil {
				return nil, errs.New(
					"containerd",
					errs.ErrInvalidConfig,
					"rootless image group must be numeric: "+config.User,
				)
			}
		}

		opts = append(
			opts,
			oci.WithUIDGID(
				uint32(uid),
				uint32(gid),
			),
		)
	}

	return opts, nil
}

func isRootlessSocket(socket string) bool {
	return strings.Contains(
		socket,
		"/run/user/",
	)
}

func (c *Containerd) Start(
	ctx context.Context,
	id string,
) error {
	return c.StartContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) Stop(
	ctx context.Context,
	id string,
) error {
	return c.StopContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) Suspend(
	ctx context.Context,
	id string,
) error {
	return c.SuspendContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) Resume(
	ctx context.Context,
	id string,
) error {
	return c.ResumeContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) Delete(
	ctx context.Context,
	id string,
) error {
	return c.DeleteContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) Restart(
	ctx context.Context,
	id string,
) error {
	return c.RestartContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) StartContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerStartOptions,
) error {
	ctx = withNamespace(ctx)

	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStartContainer,
			id,
			err,
		)
	}

	task, err := ctr.Task(ctx, nil)

	if errdefs.IsNotFound(err) {
		if mkdirErr := os.MkdirAll(
			filepath.Dir(logPath(id)),
			0o700,
		); mkdirErr != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToPrepareIO,
				id,
				mkdirErr,
			)
		}

		task, err = ctr.NewTask(
			ctx,
			cio.LogFile(logPath(id)),
		)
	}

	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStartContainer,
			id,
			err,
		)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	if status.Status == containerd.Running {
		return nil
	}

	if status.Status == containerd.Paused {
		if resumeErr := task.Resume(ctx); resumeErr != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStartContainer,
				id,
				resumeErr,
			)
		}

		return nil
	}

	if status.Status != containerd.Created {
		if _, deleteErr := task.Delete(ctx); deleteErr != nil &&
			!errdefs.IsNotFound(deleteErr) {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStartContainer,
				id,
				deleteErr,
			)
		}

		task, err = ctr.NewTask(
			ctx,
			cio.LogFile(logPath(id)),
		)
		if err != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStartContainer,
				id,
				err,
			)
		}
	}

	if err := task.Start(ctx); err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStartContainer,
			id,
			err,
		)
	}

	return nil
}

func (c *Containerd) StopContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerStopOptions,
) error {
	ctx = withNamespace(ctx)

	ctr, err := c.cli.LoadContainer(ctx, id)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStopContainer,
			id,
			err,
		)
	}

	task, err := ctr.Task(ctx, nil)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStopContainer,
			id,
			err,
		)
	}

	status, err := task.Status(ctx)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	if status.Status == containerd.Paused {
		if err := task.Resume(ctx); err != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStopContainer,
				id,
				err,
			)
		}

		status.Status = containerd.Running
	}

	switch status.Status {
	case containerd.Running:
		wait, waitErr := task.Wait(ctx)
		if waitErr != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStopContainer,
				id,
				waitErr,
			)
		}

		if err := task.Kill(
			ctx,
			syscall.SIGTERM,
		); err != nil {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToStopContainer,
				id,
				err,
			)
		}

		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		select {
		case <-wait:

		case <-timer.C:
			if err := task.Kill(
				ctx,
				syscall.SIGKILL,
			); err != nil {
				return errs.Wrap(
					"containerd",
					errs.ErrFailedToStopContainer,
					id,
					err,
				)
			}

			select {
			case <-wait:
			case <-ctx.Done():
				return ctx.Err()
			}

		case <-ctx.Done():
			return ctx.Err()
		}

	case containerd.Stopped:
		// Nothing to signal.
	}

	if _, err := task.Delete(ctx); err != nil &&
		!errdefs.IsNotFound(err) {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToStopContainer,
			id,
			err,
		)
	}

	return nil
}

func (c *Containerd) SuspendContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerPauseOptions,
) error {
	task, err := c.loadTask(ctx, id)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToSuspendContainer,
			id,
			err,
		)
	}

	status, err := task.Status(
		withNamespace(ctx),
	)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	if status.Status == containerd.Paused {
		return nil
	}

	if status.Status != containerd.Running {
		return errs.New(
			"containerd",
			errs.ErrFailedToSuspendContainer,
			"container is not running",
		)
	}

	if err := task.Pause(
		withNamespace(ctx),
	); err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToSuspendContainer,
			id,
			err,
		)
	}

	return nil
}

func (c *Containerd) ResumeContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerUnpauseOptions,
) error {
	task, err := c.loadTask(ctx, id)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToResumeContainer,
			id,
			err,
		)
	}

	status, err := task.Status(
		withNamespace(ctx),
	)
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	if status.Status == containerd.Running {
		return nil
	}

	if status.Status != containerd.Paused {
		return errs.New(
			"containerd",
			errs.ErrFailedToResumeContainer,
			"container is not paused",
		)
	}

	if err := task.Resume(
		withNamespace(ctx),
	); err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToResumeContainer,
			id,
			err,
		)
	}

	return nil
}

func (c *Containerd) RestartContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerRestartOptions,
) error {
	if err := c.StopContainer(
		ctx,
		id,
		nil,
	); err != nil {
		return err
	}

	return c.StartContainer(
		ctx,
		id,
		nil,
	)
}

func (c *Containerd) DeleteContainer(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerRemoveOptions,
) error {
	ctx = withNamespace(ctx)

	ctr, err := c.cli.LoadContainer(ctx, id)
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToDeleteContainer,
			id,
			err,
		)
	}

	if task, taskErr := ctr.Task(ctx, nil); taskErr == nil {
		if _, deleteErr := task.Delete(
			ctx,
			containerd.WithProcessKill,
		); deleteErr != nil &&
			!errdefs.IsNotFound(deleteErr) {
			return errs.Wrap(
				"containerd",
				errs.ErrFailedToDeleteContainer,
				id,
				deleteErr,
			)
		}
	} else if !errdefs.IsNotFound(taskErr) {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToDeleteContainer,
			id,
			taskErr,
		)
	}

	if err := ctr.Delete(
		ctx,
		containerd.WithSnapshotCleanup,
	); err != nil {
		return errs.Wrap(
			"containerd",
			errs.ErrFailedToDeleteContainer,
			id,
			err,
		)
	}

	_ = os.Remove(logPath(id))
	_ = os.RemoveAll(configPath(id))

	return nil
}

func (c *Containerd) State(
	ctx context.Context,
	id string,
) (virt.NodeState, error) {
	task, err := c.loadTask(ctx, id)

	if errdefs.IsNotFound(err) {
		return virt.NodeStopped, nil
	}

	if err != nil {
		return "", errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	status, err := task.Status(
		withNamespace(ctx),
	)
	if err != nil {
		return "", errs.Wrap(
			"containerd",
			errs.ErrFailedToReadState,
			id,
			err,
		)
	}

	return mapState(status.Status), nil
}

func (c *Containerd) Reload(
	ctx context.Context,
	id string,
) (*virt.Node, error) {
	node, err := c.Inspect(
		ctx,
		id,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &node.Node, nil
}

func (c *Containerd) List(
	ctx context.Context,
) ([]*virt.Node, error) {
	ctx = withNamespace(ctx)

	containers, err := c.cli.Containers(ctx)
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToListContainers,
			"",
			err,
		)
	}

	nodes := make(
		[]*virt.Node,
		0,
		len(containers),
	)

	for _, ctr := range containers {
		state, stateErr := c.State(
			ctx,
			ctr.ID(),
		)
		if stateErr != nil {
			return nil, stateErr
		}

		nodes = append(
			nodes,
			&virt.Node{
				ID:    ctr.ID(),
				Name:  ctr.ID(),
				State: state,
			},
		)
	}

	return nodes, nil
}

func (c *Containerd) Inspect(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerInspectOptions,
) (*container.ContainerNode, error) {
	ctx = withNamespace(ctx)

	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToInspectContainer,
			id,
			err,
		)
	}

	info, err := ctr.Info(ctx)
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToInspectContainer,
			id,
			err,
		)
	}

	state, err := c.State(ctx, id)
	if err != nil {
		return nil, err
	}

	imageID := ""

	if image, imageErr := ctr.Image(ctx); imageErr == nil {
		imageID = image.Target().Digest.String()
	}

	var pid uint32

	if task, taskErr := ctr.Task(ctx, nil); taskErr == nil {
		pid = task.Pid()
	}

	return &container.ContainerNode{
		Node: virt.Node{
			ID:    id,
			Name:  id,
			State: state,
		},
		PID:       pid,
		ImageID:   imageID,
		ImageName: info.Image,
		Labels:    info.Labels,
	}, nil
}

func (c *Containerd) Exec(
	ctx context.Context,
	id string,
	cmd []string,
	_ *mobyclient.ExecCreateOptions,
	_ *mobyclient.ExecAttachOptions,
) (
	stdoutBytes []byte,
	stderrBytes []byte,
	resultErr error,
) {
	if len(cmd) == 0 {
		return nil, nil, errs.New(
			"containerd",
			errs.ErrInvalidConfig,
			"exec command is empty",
		)
	}

	ctx = withNamespace(ctx)

	task, err := c.loadTask(ctx, id)
	if err != nil {
		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateExec,
			id,
			err,
		)
	}

	taskSpec, err := task.Spec(ctx)
	if err != nil {
		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateExec,
			id,
			err,
		)
	}

	if taskSpec.Process == nil {
		return nil, nil, errs.New(
			"containerd",
			errs.ErrFailedToCreateExec,
			"container process spec is nil",
		)
	}

	process := *taskSpec.Process

	process.Args = slices.Clone(cmd)
	process.Terminal = false

	if process.Cwd == "" {
		process.Cwd = "/"
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	execID := fmt.Sprintf(
		"tethux-%d-%d",
		time.Now().UnixNano(),
		execSequence.Add(1),
	)

	fifoDir := filepath.Join(
		stateDir(),
		"fifo",
	)

	if mkdirErr := os.MkdirAll(
		fifoDir,
		0o700,
	); mkdirErr != nil {
		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToPrepareIO,
			id,
			mkdirErr,
		)
	}

	proc, err := task.Exec(
		ctx,
		execID,
		&process,
		cio.NewCreator(
			cio.WithFIFODir(fifoDir),
			cio.WithStreams(
				nil,
				&stdout,
				&stderr,
			),
		),
	)
	if err != nil {
		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateExec,
			id,
			err,
		)
	}

	wait, err := proc.Wait(ctx)
	if err != nil {
		_, _ = proc.Delete(ctx)

		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateExec,
			id,
			err,
		)
	}

	if err := proc.Start(ctx); err != nil {
		_, _ = proc.Delete(ctx)

		return nil, nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToAttachExec,
			id,
			err,
		)
	}

	var status containerd.ExitStatus

	select {
	case status = <-wait:

	case <-ctx.Done():
		_ = proc.Kill(
			withNamespace(context.Background()),
			syscall.SIGKILL,
		)

		_, _ = proc.Delete(
			withNamespace(context.Background()),
		)

		return stdout.Bytes(),
			stderr.Bytes(),
			ctx.Err()
	}

	_, deleteErr := proc.Delete(ctx)
	if deleteErr != nil {
		return stdout.Bytes(),
			stderr.Bytes(),
			errs.Wrap(
				"containerd",
				errs.ErrExecFailed,
				id,
				deleteErr,
			)
	}

	code, _, resultErr := status.Result()
	if resultErr != nil {
		return stdout.Bytes(),
			stderr.Bytes(),
			resultErr
	}

	if code != 0 {
		return stdout.Bytes(),
			stderr.Bytes(),
			errs.New(
				"containerd",
				errs.ErrExecFailed,
				id+" exited with status "+
					strconv.FormatUint(
						uint64(code),
						10,
					),
			)
	}

	return stdout.Bytes(),
		stderr.Bytes(),
		nil
}

func (c *Containerd) Logs(
	ctx context.Context,
	id string,
	_ *mobyclient.ContainerLogsOptions,
) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	file, err := os.Open(logPath(id))
	if err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToLogs,
			id,
			err,
		)
	}

	return file, nil
}

func (c *Containerd) loadTask(
	ctx context.Context,
	id string,
) (containerd.Task, error) {
	ctx = withNamespace(ctx)

	ctr, err := c.cli.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	return ctr.Task(ctx, nil)
}

func withNamespace(
	ctx context.Context,
) context.Context {
	return namespaces.WithNamespace(
		ctx,
		namespace,
	)
}

func mapState(
	status containerd.ProcessStatus,
) virt.NodeState {
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
	if runtimeDir := os.Getenv(
		"XDG_RUNTIME_DIR",
	); runtimeDir != "" {
		return filepath.Join(
			runtimeDir,
			"tethux",
			"containerd",
		)
	}

	return filepath.Join(
		os.TempDir(),
		"tethux-containerd",
	)
}

func logPath(id string) string {
	return filepath.Join(
		stateDir(),
		"logs",
		filepath.Base(id)+".log",
	)
}

func configPath(id string) string {
	return filepath.Join(
		stateDir(),
		"config",
		filepath.Base(id),
	)
}

func writeNetworkFiles(
	cfg *container.RuntimeConfig,
) ([]specs.Mount, error) {
	dir := configPath(cfg.Name)

	// Prevent stale hosts/resolv.conf files from a previous failed
	// creation attempt.
	if err := os.RemoveAll(dir); err != nil {
		return nil, errs.Wrap(
			"containerd",
			errs.ErrFailedToCreateContainer,
			cfg.Name,
			err,
		)
	}

	var mounts []specs.Mount

	if len(cfg.ExtraHosts) > 0 {
		if err := os.MkdirAll(
			dir,
			0o700,
		); err != nil {
			return nil, errs.Wrap(
				"containerd",
				errs.ErrFailedToCreateContainer,
				cfg.Name,
				err,
			)
		}

		var configText strings.Builder

		configText.WriteString(
			"127.0.0.1 localhost\n" +
				"::1 localhost\n",
		)

		for _, entry := range cfg.ExtraHosts {
			host, address, ok := strings.Cut(
				entry,
				":",
			)

			if !ok ||
				host == "" ||
				address == "" {
				return nil, errs.New(
					"containerd",
					errs.ErrInvalidConfig,
					"extra host "+entry+
						" must be host:address",
				)
			}

			configText.WriteString(address)
			configText.WriteByte(' ')
			configText.WriteString(host)
			configText.WriteByte('\n')
		}

		path := filepath.Join(
			dir,
			"hosts",
		)

		if err := os.WriteFile(
			path,
			[]byte(configText.String()),
			0o600,
		); err != nil {
			return nil, errs.Wrap(
				"containerd",
				errs.ErrFailedToCreateContainer,
				cfg.Name,
				err,
			)
		}

		mounts = append(
			mounts,
			specs.Mount{
				Source:      path,
				Destination: "/etc/hosts",
				Type:        "bind",
				Options: []string{
					"rbind",
					"ro",
				},
			},
		)
	}

	if len(cfg.DNS) > 0 {
		if err := os.MkdirAll(
			dir,
			0o700,
		); err != nil {
			return nil, errs.Wrap(
				"containerd",
				errs.ErrFailedToCreateContainer,
				cfg.Name,
				err,
			)
		}

		var configText strings.Builder

		for _, address := range cfg.DNS {
			fmt.Fprintf(
				&configText,
				"nameserver %s\n",
				address,
			)
		}

		path := filepath.Join(
			dir,
			"resolv.conf",
		)

		if err := os.WriteFile(
			path,
			[]byte(configText.String()),
			0o600,
		); err != nil {
			return nil, errs.Wrap(
				"containerd",
				errs.ErrFailedToCreateContainer,
				cfg.Name,
				err,
			)
		}

		mounts = append(
			mounts,
			specs.Mount{
				Source:      path,
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Options: []string{
					"rbind",
					"ro",
				},
			},
		)
	}

	return mounts, nil
}
