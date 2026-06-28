package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xveya/tethux/internal/libtethux/virt/container/errs"
	mobyimpl "github.com/0xveya/tethux/internal/libtethux/virt/container/moby"
)

type Option func(*config)

type config struct {
	socketOverride string
}

func WithSocket(s string) Option {
	return func(c *config) { c.socketOverride = s }
}

type Docker struct {
	*mobyimpl.Client
}

func (d *Docker) Socket() string { return d.Client.Socket() }

func New(opts ...Option) (*Docker, error) {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	socket, err := resolveSocket(cfg)
	if err != nil {
		return nil, err
	}

	cli, err := mobyimpl.New("docker", socket)
	if err != nil {
		return nil, err
	}

	return &Docker{Client: cli}, nil
}

type socketCandidate struct {
	label  string
	socket string
}

func resolveSocket(cfg *config) (string, error) {
	if cfg.socketOverride != "" {
		if err := checkSocket(cfg.socketOverride); err != nil {
			return "", fmt.Errorf("docker: %w: %q err: %w", errs.ErrOverrideSocketNotAccessible, cfg.socketOverride, err)
		}
		return cfg.socketOverride, nil
	}

	if val := os.Getenv("DOCKER_HOST"); val != "" {
		if err := checkSocket(val); err == nil {
			return val, nil
		}
	}

	for _, candidate := range socketCandidates() {
		if err := checkSocket(candidate.socket); err == nil {
			return candidate.socket, nil
		}
	}

	return "", fmt.Errorf("docker: %w; tried %v - is docker running?", errs.ErrNoSockerFound, socketPaths())
}

func socketCandidates() []socketCandidate {
	var candidates []socketCandidate

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, socketCandidate{
			label:  "rootless/XDG_RUNTIME_DIR",
			socket: "unix://" + filepath.Join(xdg, "docker.sock"),
		})
	}

	if uid := os.Getuid(); uid > 0 {
		candidates = append(candidates, socketCandidate{
			label:  "rootless/run-user",
			socket: fmt.Sprintf("unix:///run/user/%d/docker.sock", uid),
		})
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			socketCandidate{
				label:  "rootless/docker-desktop",
				socket: "unix://" + filepath.Join(home, ".docker", "run", "docker.sock"),
			},
			socketCandidate{
				label:  "rootless/docker-desktop-legacy",
				socket: "unix://" + filepath.Join(home, ".docker", "desktop", "docker.sock"),
			},
		)
	}

	candidates = append(candidates,
		socketCandidate{label: "rootful/var-run-docker", socket: "unix:///var/run/docker.sock"},
		socketCandidate{label: "rootful/run-docker", socket: "unix:///run/docker.sock"},
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
		return fmt.Errorf("%q exists but is %w", path, errs.ErrNotASocket)
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
