package container

import (
	"context"
	"io"

	"github.com/moby/moby/client"
	"github.com/tethux/tethux/virt"
)

// ContainerProvider extends virt.Provider with OCI container operations.
type ContainerProvider interface {
	virt.Provider

	CreateContainer(
		ctx context.Context,
		cfg *RuntimeConfig,
	) (*ContainerNode, error)

	Pull(
		ctx context.Context,
		ref string,
		opts *client.ImagePullOptions,
	) error

	Exec(
		ctx context.Context,
		id string,
		cmd []string,
		execOpts *client.ExecCreateOptions,
		attachOpts *client.ExecAttachOptions,
	) (stdout, stderr []byte, err error)

	Logs(
		ctx context.Context,
		id string,
		opts *client.ContainerLogsOptions,
	) (io.ReadCloser, error)

	Inspect(
		ctx context.Context,
		id string,
		opts *client.ContainerInspectOptions,
	) (*ContainerNode, error)

	StartContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerStartOptions,
	) error

	StopContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerStopOptions,
	) error

	DeleteContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerRemoveOptions,
	) error

	RestartContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerRestartOptions,
	) error

	SuspendContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerPauseOptions,
	) error

	ResumeContainer(
		ctx context.Context,
		id string,
		opts *client.ContainerUnpauseOptions,
	) error
}
