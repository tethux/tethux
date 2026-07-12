package virt

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	libbridge "github.com/0xveya/tethux/internal/libtethux/bridge"
	libvirt "github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/0xveya/tethux/internal/libtethux/virt/container"
)

type linkEvent struct {
	Schema   string `json:"schema"`
	Host     string `json:"host"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Peer     string `json:"peer,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

func linkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link",
		Short: "connect provider-managed containers across two hosts with UDP bridges",
	}
	cmd.AddCommand(linkEndpointCmd(), linkTestCmd())
	return cmd
}

func linkEndpointCmd() *cobra.Command {
	var provider, socket, name, image, listen, remote, address, peer, hostIf, containerIf string
	var mtu int
	c := &cobra.Command{
		Use:   "endpoint",
		Short: "run one provider-managed container and its host UDP bridge",
		RunE: func(c *cobra.Command, _ []string) (resultErr error) {
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
			defer stop()
			p, providerErr := newProvider(provider, socket)
			if providerErr != nil {
				return providerErr
			}
			_ = p.Delete(ctx, name)
			if pullErr := p.Pull(ctx, image, nil); pullErr != nil {
				return pullErr
			}
			node, err := p.CreateContainer(ctx, &container.ContainerConfig{
				NodeConfig:  libvirt.NodeConfig{Name: name, Image: image},
				Cmd:         []string{"sh", "-c", "trap 'exit 0' TERM; while :; do sleep 1; done"},
				NetworkMode: "none",
				Labels:      map[string]string{"io.tethux.test": "cross-host-link"},
			})
			if err != nil {
				return err
			}
			defer func() {
				cleanupErr := p.DeleteContainer(context.WithoutCancel(ctx), node.ID, &client.ContainerRemoveOptions{Force: true})
				if resultErr == nil && cleanupErr != nil {
					resultErr = cleanupErr
				}
			}()
			if startErr := p.Start(ctx, node.ID); startErr != nil {
				return startErr
			}
			inspected, err := p.Inspect(ctx, node.ID, nil)
			if err != nil {
				return err
			}
			if inspected.PID == 0 {
				return errors.New("provider inspect returned no container PID")
			}
			bridge, err := libbridge.StartContainerBridge(&libbridge.ContainerBridgeOptions{
				PID:         int(inspected.PID),
				HostIf:      hostIf,
				ContainerIf: containerIf,
				Listen:      listen,
				Remote:      remote,
				MTU:         mtu,
				Immediate:   true,
			})
			if err != nil {
				return err
			}
			defer bridge.Close()
			if _, stderr, err := p.Exec(ctx, node.ID, []string{"ip", "addr", "add", address, "dev", containerIf}, nil, nil); err != nil {
				return fmt.Errorf("configure endpoint address (stderr %q): %w", stderr, err)
			}

			event := linkEvent{Schema: "tethux.cross-host-link/v1", Host: localHostname(), Provider: provider, Name: name, Address: address, Peer: peer, Status: "ready"}
			if err := json.NewEncoder(os.Stdout).Encode(event); err != nil {
				return err
			}
			if peer == "" {
				<-ctx.Done()
				return nil
			}
			for attempt := 0; attempt < 20; attempt++ {
				stdout, stderr, pingErr := p.Exec(ctx, node.ID, []string{"ping", "-c", "1", "-W", "1", peer}, nil, nil)
				if pingErr == nil {
					event.Status = "passed"
					event.Peer = peer
					return json.NewEncoder(os.Stdout).Encode(event)
				}
				if attempt == 19 {
					return fmt.Errorf("ping %s failed (stdout %q stderr %q): %w", peer, stdout, stderr, pingErr)
				}
				time.Sleep(500 * time.Millisecond)
			}
			return nil
		},
	}
	addProviderFlags(c, &provider, &socket)
	c.Flags().StringVar(&name, "name", "tethux-cross-link", "container name")
	c.Flags().StringVar(&image, "image", "docker.io/library/alpine:3.20", "container image")
	c.Flags().StringVar(&listen, "listen", "0.0.0.0:24000", "local UDP bridge address")
	c.Flags().StringVar(&remote, "remote", "", "remote UDP bridge address")
	c.Flags().StringVar(&address, "address", "10.88.0.1/24", "container interface address")
	c.Flags().StringVar(&peer, "peer", "", "optional peer IP to ping before exiting")
	c.Flags().StringVar(&hostIf, "host-if", "txcross0", "host-side veth interface")
	c.Flags().StringVar(&containerIf, "container-if", "tx0", "container-side veth interface")
	c.Flags().IntVar(&mtu, "mtu", 1500, "bridge MTU")
	_ = c.MarkFlagRequired("remote")
	return c
}

func linkTestCmd() *cobra.Command {
	var hostA, hostB, providerA, providerB, binary, image string
	c := &cobra.Command{
		Use:   "test",
		Short: "prove a container on host A can ping a container on host B",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, cancel := context.WithCancel(c.Context())
			defer cancel()
			bArgs := remoteEndpointArgs(binary, providerB, "tethux-cross-b", image, "0.0.0.0:24000", hostAddress(hostA)+":24000", "10.88.0.2/24", "10.88.0.1", "txcrossb")
			bCmd := sshCommand(ctx, hostB, bArgs)
			stdout, pipeErr := bCmd.StdoutPipe()
			if pipeErr != nil {
				return pipeErr
			}
			bCmd.Stderr = os.Stderr
			if cmdStartErr := bCmd.Start(); cmdStartErr != nil {
				return cmdStartErr
			}
			ready := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				announced := false
				for scanner.Scan() {
					line := scanner.Text()
					fmt.Println("[host-b]", line)
					if !announced && strings.Contains(line, `"status":"ready"`) {
						announced = true
						ready <- nil
					}
				}
				if !announced {
					ready <- errors.Join(scanner.Err(), errors.New("host B endpoint exited before ready"))
				}
			}()
			select {
			case readyErr := <-ready:
				if readyErr != nil {
					return readyErr
				}
			case <-time.After(90 * time.Second):
				return errors.New("timed out waiting for host B endpoint")
			case <-c.Context().Done():
				return c.Context().Err()
			}

			aArgs := remoteEndpointArgs(binary, providerA, "tethux-cross-a", image, "0.0.0.0:24000", hostAddress(hostB)+":24000", "10.88.0.1/24", "10.88.0.2", "txcrossa")
			aCmd := sshCommand(c.Context(), hostA, aArgs)
			aCmd.Stdout = os.Stdout
			aCmd.Stderr = os.Stderr
			runErr := aCmd.Run()
			bWaitErr := bCmd.Wait()
			cancel()
			return errors.Join(runErr, bWaitErr)
		},
	}
	c.Flags().StringVar(&hostA, "host-a", "ci@10.0.0.100", "SSH host for endpoint A")
	c.Flags().StringVar(&hostB, "host-b", "ci@10.0.0.78", "SSH host for endpoint B")
	c.Flags().StringVar(&providerA, "provider-a", "docker", "container provider on host A")
	c.Flags().StringVar(&providerB, "provider-b", "podman", "container provider on host B")
	c.Flags().StringVar(&binary, "remote-binary", "tethux", "tethux binary path on both hosts")
	c.Flags().StringVar(&image, "image", "docker.io/library/alpine:3.20", "container image")
	return c
}

func remoteEndpointArgs(binary, provider, name, image, listen, remote, address, peer, hostIf string) []string {
	args := []string{"sudo", "-n", "env", "PATH=/run/current-system/sw/bin:/usr/bin:/bin", binary, "virt", "link", "endpoint", "--provider", provider, "--name", name, "--image", image, "--listen", listen, "--remote", remote, "--address", address, "--host-if", hostIf}
	if peer != "" {
		args = append(args, "--peer", peer)
	}
	return args
}

func hostAddress(host string) string {
	if _, address, ok := strings.Cut(host, "@"); ok {
		return address
	}
	return host
}

func sshCommand(ctx context.Context, host string, remoteArgs []string) *osexec.Cmd {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=/tmp/tethux-ci-known-hosts",
		host,
		shellJoin(remoteArgs),
	}
	return osexec.CommandContext(ctx, "ssh", args...) // #nosec G204 -- host and remote command are explicit CLI inputs.
}
