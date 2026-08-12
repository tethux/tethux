// Tethux CI runs repository and physical-host workflows through Dagger.
package main

import (
	"context"
	"strings"

	"dagger/tethux/internal/dagger"
)

type Tethux struct{}

func (m *Tethux) Format(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.repositoryTask(source, "check-format").Stdout(ctx)
}

func (m *Tethux) Build(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.repositoryTask(source, "build").Stdout(ctx)
}

func (m *Tethux) Lint(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.repositoryTask(source, "lint").Stdout(ctx)
}

func (m *Tethux) Test(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.repositoryTask(source, "test").Stdout(ctx)
}

// Normal runs the complete unprivileged repository gate.
func (m *Tethux) Normal(ctx context.Context, source *dagger.Directory) (string, error) {
	outputs := make([]string, 0, 4)
	for _, task := range []string{"check-format", "build", "lint", "test"} {
		output, err := m.repositoryTask(source, task).Stdout(ctx)
		if err != nil {
			return "", err
		}
		outputs = append(outputs, output)
	}
	return strings.Join(outputs, "\n"), nil
}

// Laptop100 runs the Docker workflow on the physical laptop.
func (m *Tethux) Laptop100(
	ctx context.Context,
	source *dagger.Directory,
	sshKey *dagger.Secret,
	knownHosts *dagger.Secret,
) (string, error) {
	return m.remote(source, sshKey, knownHosts).
		WithExec(nixDevelop("go", "run", "./tools/ci", "run", "remote-laptop",
			"--host", "ci@10.0.0.100", "--runtime", "docker", "--device", "laptop-100")).
		Stdout(ctx)
}

// Laptop78 runs the Podman workflow on the physical laptop.
func (m *Tethux) Laptop78(
	ctx context.Context,
	source *dagger.Directory,
	sshKey *dagger.Secret,
	knownHosts *dagger.Secret,
) (string, error) {
	return m.remote(source, sshKey, knownHosts).
		WithExec(nixDevelop("go", "run", "./tools/ci", "run", "remote-laptop",
			"--host", "ci@10.0.0.78", "--runtime", "podman", "--device", "laptop-78")).
		Stdout(ctx)
}

// CrossLaptop verifies the managed link between both physical laptops.
func (m *Tethux) CrossLaptop(
	ctx context.Context,
	source *dagger.Directory,
	sshKey *dagger.Secret,
	knownHosts *dagger.Secret,
) (string, error) {
	return m.remote(source, sshKey, knownHosts).
		WithExec(nixDevelop("go", "run", "./tools/ci", "run", "cross-laptop", "--device", "cross-laptop")).
		Stdout(ctx)
}

func (m *Tethux) repositoryTask(source *dagger.Directory, task string) *dagger.Container {
	return m.base(source).WithExec(nixDevelop("go", "run", "./tools/ci", "task", task))
}

func (m *Tethux) base(source *dagger.Directory) *dagger.Container {
	return dag.Container().
		From("nixos/nix:2.32.4").
		WithMountedDirectory("/src", source).
		WithEnvVariable("TETHUX_REPOSITORY_ROOT", "/src").
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("tethux-go-build")).
		WithMountedCache("/root/go/pkg/mod", dag.CacheVolume("tethux-go-mod")).
		WithWorkdir("/src")
}

func (m *Tethux) remote(
	source *dagger.Directory,
	sshKey *dagger.Secret,
	knownHosts *dagger.Secret,
) *dagger.Container {
	return m.base(source).
		WithMountedSecret("/root/.ssh/id_rsa", sshKey).
		WithMountedSecret("/root/.ssh/known_hosts", knownHosts)
}

func nixDevelop(command ...string) []string {
	args := []string{"nix", "develop", ".#ci", "--extra-experimental-features", "nix-command flakes", "-c"}
	return append(args, command...)
}
