package container

import (
	"context"
	"io"
	"net/netip"

	"github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/moby/moby/client"
)

type ContainerProvider interface {
	virt.Provider

	CreateContainer(ctx context.Context, cfg *ContainerConfig) (*ContainerNode, error)
	Pull(ctx context.Context, ref string, opts *client.ImagePullOptions) error
	Exec(ctx context.Context, id string, cmd []string, execOpts *client.ExecCreateOptions, attachOpts *client.ExecAttachOptions) (stdout, stderr []byte, err error)
	Logs(ctx context.Context, id string, opts *client.ContainerLogsOptions) (io.ReadCloser, error)
	Inspect(ctx context.Context, id string, opts *client.ContainerInspectOptions) (*ContainerNode, error)

	StartContainer(ctx context.Context, id string, opts *client.ContainerStartOptions) error
	StopContainer(ctx context.Context, id string, opts *client.ContainerStopOptions) error
	DeleteContainer(ctx context.Context, id string, opts *client.ContainerRemoveOptions) error
	RestartContainer(ctx context.Context, id string, opts *client.ContainerRestartOptions) error
	SuspendContainer(ctx context.Context, id string, opts *client.ContainerPauseOptions) error
	ResumeContainer(ctx context.Context, id string, opts *client.ContainerUnpauseOptions) error
	// needs cleanup and prune funcs
}

type ContainerConfig struct {
	virt.NodeConfig

	Registry string
	Tag      string
	Digest   string

	Entrypoint  []string
	Cmd         []string
	Env         []string
	Volumes     []VolumeMount
	CapAdd      []string
	CapDrop     []string
	Privileged  bool
	NetworkMode string

	Hostname   string
	DNS        []netip.Addr
	ExtraHosts []string

	Labels map[string]string
}

type VolumeMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ContainerNode struct {
	virt.Node

	PID       uint32
	ImageID   string
	ImageName string
	Labels    map[string]string
	Networks  []string
}
