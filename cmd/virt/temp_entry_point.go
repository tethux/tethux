package virt

import (
	"context"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	"github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/0xveya/tethux/internal/libtethux/virt/container"
	"github.com/0xveya/tethux/internal/libtethux/virt/container/docker"
	"github.com/0xveya/tethux/internal/libtethux/virt/container/podman"
)

const testHostEnv = "TETHUX_VIRT_TEST_HOST"

func newProvider(provider, socket string) (container.ContainerProvider, error) {
	switch provider {
	case "podman":
		var opts []podman.Option
		if socket != "" {
			opts = append(opts, podman.WithSocket(socket))
		}
		return podman.New(opts...)
	case "docker":
		var opts []docker.Option
		if socket != "" {
			opts = append(opts, docker.WithSocket(socket))
		}
		return docker.New(opts...)
	case "containerd":
		return nil, fmt.Errorf("containerd provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider %q - choose podman, docker, or containerd", provider)
	}
}

func addProviderFlags(c *cobra.Command, provider, socket *string) {
	c.Flags().StringVarP(provider, "provider", "p", "podman", "container provider: podman, docker, containerd")
	c.Flags().StringVar(socket, "socket", "", "override provider socket path")
}

func smokeCmd() *cobra.Command {
	var (
		provider string
		socket   string
		host     string
		name     string
		image    string
		cmd      []string
	)

	c := &cobra.Command{
		Use:   "smoke",
		Short: "spin up a container, exec into it, then clean up",
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()
			if host != "" {
				return runRemoteSmoke(ctx, host, provider, socket, name, image, cmd)
			}

			p, err := newProvider(provider, socket)
			if err != nil {
				return err
			}

			fmt.Printf("[%s] pulling %s...\n", provider, image)
			if pullErr := p.Pull(ctx, image, nil); pullErr != nil {
				return pullErr
			}
			fmt.Println("pulled", image)

			_ = p.Delete(ctx, name)

			node, err := p.CreateContainer(ctx, &container.ContainerConfig{
				NodeConfig: virt.NodeConfig{
					Name:  name,
					Image: image + ":latest",
				},
				Cmd: cmd,
			})
			if err != nil {
				return err
			}
			fmt.Println("created", node.ID[:12], "name:", node.Name)

			defer func() {
				fmt.Println("cleaning up...")
				if deleteErr := p.DeleteContainer(ctx, node.ID, &client.ContainerRemoveOptions{Force: true}); deleteErr != nil {
					fmt.Println("cleanup error:", deleteErr)
				} else {
					fmt.Println("deleted", node.ID[:12])
				}
			}()

			if startErr := p.Start(ctx, node.ID); startErr != nil {
				return startErr
			}

			state, err := p.State(ctx, node.ID)
			if err != nil {
				return err
			}
			fmt.Println("state:", state)

			node, err = p.Inspect(ctx, node.ID, nil)
			if err != nil {
				return err
			}
			fmt.Printf("node: id=%s name=%s state=%s image=%s networks=%v\n",
				node.ID[:12], node.Name, node.State, node.ImageName, node.Networks)

			stdout, stderr, err := p.Exec(ctx, node.ID, []string{"echo", "meow"}, nil, nil)
			if err != nil {
				return err
			}
			fmt.Println("exec stdout:", string(stdout))
			if len(stderr) > 0 {
				fmt.Println("exec stderr:", string(stderr))
			}

			reader, err := p.Logs(ctx, node.ID, nil)
			if err != nil {
				return err
			}
			fmt.Println("logs:")
			_, _ = io.Copy(os.Stdout, reader)
			_ = reader.Close()
			fmt.Println()

			if err := p.SuspendContainer(ctx, node.ID, nil); err != nil {
				return fmt.Errorf("suspend: %w", err)
			}
			fmt.Println("suspended")

			if err := p.ResumeContainer(ctx, node.ID, nil); err != nil {
				return fmt.Errorf("resume: %w", err)
			}
			fmt.Println("resumed")

			if err := p.RestartContainer(ctx, node.ID, nil); err != nil {
				return fmt.Errorf("restart: %w", err)
			}
			fmt.Println("restarted")

			if err := p.StopContainer(ctx, node.ID, nil); err != nil {
				return fmt.Errorf("stop: %w", err)
			}
			fmt.Println("stopped")

			return nil
		},
	}

	addProviderFlags(c, &provider, &socket)
	c.Flags().StringVar(&host, "host", os.Getenv(testHostEnv), "SSH host for remote smoke test, or "+testHostEnv)
	c.Flags().StringVar(&name, "name", "tethux-smoke", "container name")
	c.Flags().StringVar(&image, "image", "alpine", "image to pull and run")
	c.Flags().StringSliceVar(&cmd, "cmd", []string{"sh", "-c", "echo meow && sleep 30"}, "command to run")

	return c
}

func runRemoteSmoke(ctx context.Context, host, provider, socket, name, image string, cmd []string) error {
	parts := []string{
		"sudo",
		"-n",
		"env",
		testHostEnv + "=",
		"tethux",
		"virt",
		"smoke",
		"--provider",
		provider,
		"--name",
		name,
		"--image",
		image,
	}
	if socket != "" {
		parts = append(parts, "--socket", socket)
	}
	for _, part := range cmd {
		parts = append(parts, "--cmd", part)
	}

	remote := shellJoin(parts)
	fmt.Printf("[%s] running remote smoke on %s\n", provider, host)
	// #nosec G204 -- this command is the explicit SSH transport for the --host smoke-test flag.
	ssh := osexec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", host, remote)
	ssh.Stdin = os.Stdin
	ssh.Stdout = os.Stdout
	ssh.Stderr = os.Stderr
	return ssh.Run()
}

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func listCmd() *cobra.Command {
	var provider, socket string

	c := &cobra.Command{
		Use:   "list",
		Short: "list all containers",
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			p, err := newProvider(provider, socket)
			if err != nil {
				return err
			}

			nodes, err := p.List(ctx)
			if err != nil {
				return err
			}

			if len(nodes) == 0 {
				fmt.Println("no containers")
				return nil
			}

			fmt.Printf("%-14s %-20s %s\n", "ID", "NAME", "STATE")
			for _, n := range nodes {
				fmt.Printf("%-14s %-20s %s\n", n.ID[:12], n.Name, n.State)
			}
			return nil
		},
	}

	addProviderFlags(c, &provider, &socket)
	return c
}

func pullCmd() *cobra.Command {
	var provider, socket string

	c := &cobra.Command{
		Use:   "pull <ref>",
		Short: "pull an image",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			p, err := newProvider(provider, socket)
			if err != nil {
				return err
			}

			fmt.Printf("[%s] pulling %s...\n", provider, args[0])
			if err := p.Pull(ctx, args[0], nil); err != nil {
				return err
			}
			fmt.Println("done")
			return nil
		},
	}

	addProviderFlags(c, &provider, &socket)
	return c
}

func logsCmd() *cobra.Command {
	var (
		provider   string
		socket     string
		follow     bool
		timestamps bool
		tail       string
	)

	c := &cobra.Command{
		Use:   "logs <container-id>",
		Short: "fetch logs from a container",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			p, err := newProvider(provider, socket)
			if err != nil {
				return err
			}

			reader, err := p.Logs(ctx, args[0], &client.ContainerLogsOptions{
				Follow:     follow,
				Timestamps: timestamps,
				Tail:       tail,
			})
			if err != nil {
				return err
			}
			defer reader.Close()

			_, _ = io.Copy(os.Stdout, reader)
			return nil
		},
	}

	addProviderFlags(c, &provider, &socket)
	c.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	c.Flags().BoolVarP(&timestamps, "timestamps", "t", false, "show timestamps")
	c.Flags().StringVar(&tail, "tail", "all", "number of lines from end")

	return c
}
