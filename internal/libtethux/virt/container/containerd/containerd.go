package containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0xveya/tethux/internal/libtethux/virt"
	"github.com/0xveya/tethux/internal/libtethux/virt/container"
	"github.com/0xveya/tethux/internal/libtethux/virt/container/errs"
	containersspecs "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	containerd "github.com/containerd/containerd/v2/client"
	mobyclient "github.com/moby/moby/client"
)

// uncomment to check at compile time if methods are missing
// var _ container.ContainerProvider = (*Containerd)(nil)

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
		candidates = append(candidates, socketCandidate{
			label:  "rootless/XDG_RUNTIME_DIR",
			socket: filepath.Join(xdg, "containerd-rootless", "containerd.sock"),
		})
	}

	if uid := os.Getuid(); uid > 0 {
		candidates = append(candidates, socketCandidate{
			label:  "rootless/run-user",
			socket: fmt.Sprintf("/run/user/%d/containerd-rootless/containerd.sock", uid),
		})
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
	ctx = namespaces.WithNamespace(ctx, "default")
	_, err := c.cli.Pull(ctx, ref, containerd.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("containerd: %w %q: %w", errs.ErrFailedToPullImage, ref, err)
	}
	return nil
}

func (c *Containerd) CreateContainer(ctx context.Context, cfg *container.ContainerConfig) (*container.ContainerNode, error) {
	ctx = namespaces.WithNamespace(ctx, "default")

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

	// network mode + DNS via raw spec edit
	specOpts = append(specOpts, func(ctx context.Context, _ oci.Client, _ *containersspecs.Container, s *specs.Spec) error {
		if cfg.NetworkMode == "host" {
			s.Linux.Namespaces = slices.DeleteFunc(s.Linux.Namespaces, func(ns specs.LinuxNamespace) bool {
				return ns.Type == specs.NetworkNamespace
			})
		}

		// extra hosts -> appended to /etc/hosts via mount
		if len(cfg.ExtraHosts) > 0 {
			var hosts strings.Builder
			for _, h := range cfg.ExtraHosts {
				hosts.WriteString(h)
				hosts.WriteString("\n")
			}
			s.Mounts = append(s.Mounts, specs.Mount{
				Destination: "/etc/hosts",
				Type:        "bind",
				Source:      "/etc/hosts",
				Options:     []string{"rbind", "rw"},
			})
			// note: extra hosts are appended at task start via exec or prestart hook
			// full injection requires a tmpfile written before task creation
		}

		// DNS -> write resolv.conf content
		if len(cfg.DNS) > 0 {
			var resolv strings.Builder
			for _, addr := range cfg.DNS {
				resolv.WriteString("nameserver ")
				resolv.WriteString(addr.String())
				resolv.WriteString("\n")
			}
			// same pattern - needs tmpfile, see Start() for injection
			_ = resolv.String()
		}

		return nil
	})

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
