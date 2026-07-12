package podman

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xveya/tethux/internal/libtethux/virt/container/errs"
	mobyimpl "github.com/0xveya/tethux/internal/libtethux/virt/container/moby"
)

// uncomment to check at compile time if methods are missing
// var _ container.ContainerProvider = (*Podman)(nil)

type Option func(*config)

type config struct {
	socketOverride string
}

func WithSocket(s string) Option {
	return func(c *config) { c.socketOverride = s }
}

type Podman struct {
	*mobyimpl.Client
}

func (p *Podman) Socket() string { return p.Client.Socket() }

func New(opts ...Option) (*Podman, error) {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	socket, err := resolveSocket(cfg)
	if err != nil {
		return nil, err
	}

	cli, err := mobyimpl.New("podman", socket)
	if err != nil {
		return nil, err
	}

	return &Podman{Client: cli}, nil
}

type socketCandidate struct {
	label  string
	socket string
}

func resolveSocket(cfg *config) (string, error) {
	if cfg.socketOverride != "" {
		if err := checkSocket(cfg.socketOverride); err != nil {
			return "", errs.Wrap("podman", errs.ErrOverrideSocketNotAccessible, cfg.socketOverride, err)
		}
		return cfg.socketOverride, nil
	}

	for _, env := range []string{"CONTAINER_HOST", "DOCKER_HOST"} {
		if val := os.Getenv(env); val != "" {
			if err := checkSocket(val); err == nil {
				return val, nil
			}
		}
	}

	for _, candidate := range socketCandidates() {
		if err := checkSocket(candidate.socket); err == nil {
			return candidate.socket, nil
		}
	}

	return "", errs.New("podman", errs.ErrNoSocketFound, strings.Join(socketPaths(), ", "))
}

func socketCandidates() []socketCandidate {
	var candidates []socketCandidate

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, socketCandidate{
			label:  "rootless/XDG_RUNTIME_DIR",
			socket: "unix://" + filepath.Join(xdg, "podman", "podman.sock"),
		})
	}

	if uid := os.Getuid(); uid > 0 {
		candidates = append(candidates, socketCandidate{
			label:  "rootless/run-user",
			socket: fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", uid),
		})
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			socketCandidate{
				label:  "rootless/podman-machine-qemu",
				socket: "unix://" + filepath.Join(home, ".local", "share", "containers", "podman", "machine", "qemu", "podman.sock"),
			},
			socketCandidate{
				label:  "rootless/podman-machine-default",
				socket: "unix://" + filepath.Join(home, ".local", "share", "containers", "podman", "machine", "default", "podman.sock"),
			},
		)
	}

	candidates = append(candidates,
		socketCandidate{label: "rootful/run-podman", socket: "unix:///run/podman/podman.sock"},
		socketCandidate{label: "rootful/var-run-podman", socket: "unix:///var/run/podman/podman.sock"},
	)

	return candidates
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
		return errs.New("podman", errs.ErrNotASocket, path)
	}
	return nil
}

func socketPaths() []string {
	candidates := socketCandidates()
	paths := make([]string, 0, len(candidates))
	for _, c := range candidates {
		paths = append(paths, c.socket)
	}
	return paths
}
