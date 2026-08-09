package moby

import (
	"bytes"
	"context"
	"io"
	"strings"

	mobycontainer "github.com/moby/moby/api/types/container"
	mobyclient "github.com/moby/moby/client"
	"github.com/tethux/tethux/internal/libtethux/virt"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/tethux/tethux/internal/libtethux/virt/container"
	"github.com/tethux/tethux/internal/libtethux/virt/container/errs"
)

type Client struct {
	cli    *mobyclient.Client
	name   string
	socket string
}

func (c *Client) Socket() string { return c.socket }

func New(name, socket string) (*Client, error) {
	cli, err := mobyclient.New(mobyclient.WithHost(socket))
	if err != nil {
		return nil, errs.Wrap(name, errs.ErrFailedToCreateClient, socket, err)
	}
	return &Client{cli: cli, name: name, socket: socket}, nil
}

func (c *Client) StartContainer(ctx context.Context, id string, opts *mobyclient.ContainerStartOptions) error {
	o := mobyclient.ContainerStartOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerStart(ctx, id, o)
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToStartContainer, id, err)
	}
	return nil
}

func (c *Client) StopContainer(ctx context.Context, id string, opts *mobyclient.ContainerStopOptions) error {
	o := mobyclient.ContainerStopOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerStop(ctx, id, o)
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToStopContainer, id, err)
	}
	return nil
}

func (c *Client) SuspendContainer(ctx context.Context, id string, opts *mobyclient.ContainerPauseOptions) error {
	o := mobyclient.ContainerPauseOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerPause(ctx, id, o)
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToSuspendContainer, id, err)
	}
	return nil
}

func (c *Client) ResumeContainer(ctx context.Context, id string, opts *mobyclient.ContainerUnpauseOptions) error {
	o := mobyclient.ContainerUnpauseOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerUnpause(ctx, id, o)
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToResumeContainer, id, err)
	}
	return nil
}

func (c *Client) DeleteContainer(ctx context.Context, id string, opts *mobyclient.ContainerRemoveOptions) error {
	o := mobyclient.ContainerRemoveOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerRemove(ctx, id, o)
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToDeleteContainer, id, err)
	}
	return nil
}

func (c *Client) State(ctx context.Context, id string) (virt.NodeState, error) {
	resp, err := c.cli.ContainerInspect(ctx, id, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return "", errs.Wrap(c.name, errs.ErrFailedToInspectContainer, id, err)
	}

	s := resp.Container.State
	switch {
	case s.Paused:
		return virt.NodeSuspended, nil
	case s.Restarting:
		return virt.NodeStarting, nil
	case s.Status == "running":
		return virt.NodeRunning, nil
	case s.Status == "created", s.Status == "exited", s.Status == "dead":
		return virt.NodeStopped, nil
	case s.Status == "removing":
		return virt.NodeStopping, nil
	default:
		return virt.NodeStopped, nil
	}
}

func (c *Client) RestartContainer(ctx context.Context, id string, opts *mobyclient.ContainerRestartOptions) error {
	o := mobyclient.ContainerRestartOptions{}
	if opts != nil {
		o = *opts
	}
	_, err := c.cli.ContainerRestart(ctx, id, o)
	return err
}

func (c *Client) CreateContainer(
	ctx context.Context,
	cfg *container.RuntimeConfig,
) (*container.ContainerNode, error) {
	if cfg == nil {
		return nil, errs.New(c.name, errs.ErrInvalidConfig, "config is nil")
	}
	image := cfg.Image.String()
	if image == "" {
		return nil, errs.New(c.name, errs.ErrInvalidConfig, "image is required")
	}
	binds := make([]string, 0, len(cfg.Volumes))

	for _, v := range cfg.Volumes {
		bind := v.Source + ":" + v.Target

		if v.ReadOnly {
			bind += ":ro"
		}

		binds = append(binds, bind)
	}

	resp, err := c.cli.ContainerCreate(
		ctx,
		mobyclient.ContainerCreateOptions{
			Name: cfg.Name,

			Config: &mobycontainer.Config{
				Image:      image,
				Cmd:        cfg.Cmd,
				Entrypoint: cfg.Entrypoint,
				Env:        cfg.Env,
				Labels:     cfg.Labels,
				Hostname:   cfg.Hostname,
			},

			HostConfig: &mobycontainer.HostConfig{
				Binds:       binds,
				CapAdd:      cfg.CapAdd,
				CapDrop:     cfg.CapDrop,
				Privileged:  cfg.Privileged,
				NetworkMode: mobycontainer.NetworkMode(cfg.NetworkMode),
				DNS:         cfg.DNS,
				ExtraHosts:  cfg.ExtraHosts,
			},
		},
	)
	if err != nil {
		return nil, errs.Wrap(
			c.name,
			errs.ErrFailedToCreateContainer,
			cfg.Name,
			err,
		)
	}

	return &container.ContainerNode{
		Node: virt.Node{
			ID:    resp.ID,
			Name:  cfg.Name,
			State: virt.NodeStopped,
		},
		ImageName: image,
		Labels:    cfg.Labels,
	}, nil
}

func (c *Client) Pull(ctx context.Context, ref string, opts *mobyclient.ImagePullOptions) error {
	o := mobyclient.ImagePullOptions{}
	if opts != nil {
		o = *opts
	}
	// progress output can be added when the caller needs it.
	resp, err := c.cli.ImagePull(ctx, ref, mobyclient.ImagePullOptions{
		All:           o.All,
		RegistryAuth:  o.RegistryAuth,
		PrivilegeFunc: o.PrivilegeFunc,
		Platforms:     o.Platforms,
	})
	if err != nil {
		return errs.Wrap(c.name, errs.ErrFailedToPullImage, ref, err)
	}
	return resp.Wait(ctx)
}

func (c *Client) Exec(ctx context.Context, id string, cmd []string, execOpts *mobyclient.ExecCreateOptions, attachOpts *mobyclient.ExecAttachOptions) (stdout, stderr []byte, err error) {
	eo := mobyclient.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}
	if execOpts != nil {
		execOpts.Cmd = cmd
		execOpts.AttachStdout = true
		execOpts.AttachStderr = true
		eo = *execOpts
	}

	ao := mobyclient.ExecAttachOptions{}
	if attachOpts != nil {
		ao = *attachOpts
	}

	exec, err := c.cli.ExecCreate(ctx, id, eo)
	if err != nil {
		return nil, nil, errs.Wrap(c.name, errs.ErrFailedToCreateExec, id, err)
	}

	resp, err := c.cli.ExecAttach(ctx, exec.ID, ao)
	if err != nil {
		return nil, nil, errs.Wrap(c.name, errs.ErrFailedToAttachExec, id, err)
	}
	defer resp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Conn); err != nil {
		return nil, nil, errs.Wrap(c.name, errs.ErrFailedToStdCopy, id, err)
	}

	stdout = stdoutBuf.Bytes()
	stderr = stderrBuf.Bytes()

	return stdout, stderr, nil
}

func (c *Client) Logs(ctx context.Context, id string, opts *mobyclient.ContainerLogsOptions) (io.ReadCloser, error) {
	o := mobyclient.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}
	if opts != nil {
		opts.ShowStdout = true
		opts.ShowStderr = true
		o = *opts
	}
	resp, err := c.cli.ContainerLogs(ctx, id, o)
	if err != nil {
		return nil, errs.Wrap(c.name, errs.ErrFailedToLogs, id, err)
	}
	return resp, nil
}

func (c *Client) Inspect(ctx context.Context, id string, opts *mobyclient.ContainerInspectOptions) (*container.ContainerNode, error) {
	o := mobyclient.ContainerInspectOptions{}
	if opts != nil {
		o = *opts
	}
	resp, err := c.cli.ContainerInspect(ctx, id, o)
	if err != nil {
		return nil, errs.Wrap(c.name, errs.ErrFailedToInspectContainer, id, err)
	}

	var networks []string
	for name := range resp.Container.NetworkSettings.Networks {
		networks = append(networks, name)
	}

	return &container.ContainerNode{
		Node: virt.Node{
			ID:    resp.Container.ID,
			Name:  strings.TrimPrefix(resp.Container.Name, "/"),
			State: mapState(resp.Container.State),
		},
		PID:       uint32(resp.Container.State.Pid), // #nosec G115 -- runtime PIDs are non-negative on successful inspect.
		ImageID:   resp.Container.Image,
		ImageName: resp.Container.Config.Image,
		Labels:    resp.Container.Config.Labels,
		Networks:  networks,
	}, nil
}

func (c *Client) Start(ctx context.Context, id string) error {
	return c.StartContainer(ctx, id, nil)
}

func (c *Client) Stop(ctx context.Context, id string) error {
	return c.StopContainer(ctx, id, nil)
}

func (c *Client) Suspend(ctx context.Context, id string) error {
	return c.SuspendContainer(ctx, id, nil)
}

func (c *Client) Resume(ctx context.Context, id string) error {
	return c.ResumeContainer(ctx, id, nil)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return c.DeleteContainer(ctx, id, nil)
}

func (c *Client) Restart(ctx context.Context, id string) error {
	return c.RestartContainer(ctx, id, nil)
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) List(ctx context.Context) ([]*virt.Node, error) {
	result, err := c.cli.ContainerList(ctx, mobyclient.ContainerListOptions{All: true})
	if err != nil {
		return nil, errs.Wrap(c.name, errs.ErrFailedToListContainers, "", err)
	}
	var nodes []*virt.Node
	for i := range result.Items {
		name := ""
		if len(result.Items[i].Names) > 0 {
			name = strings.TrimPrefix(result.Items[i].Names[0], "/")
		}
		nodes = append(nodes, &virt.Node{
			ID:    result.Items[i].ID,
			Name:  name,
			State: mapState(result.Items[i].State),
		})
	}
	return nodes, nil
}

func (c *Client) Reload(ctx context.Context, id string) (*virt.Node, error) {
	node, err := c.Inspect(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return &node.Node, nil
}
