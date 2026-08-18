package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	ciframework "github.com/tethux/tethux/internal/ci"
)

const repositoryStepTimeout = 30 * time.Minute

func repositoryTaskCommand(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return usageError("task requires one of format, check-format, lint, test, build, or check")
	}
	root, err := ciframework.RepositoryRoot()
	if err != nil {
		return err
	}
	workflow, err := repositoryWorkflow(root, args[0])
	if err != nil {
		return err
	}
	_, err = ciframework.NewRunner(os.Stdout, os.Stderr).Run(ctx, workflow)
	return err
}

func repositoryWorkflow(root, name string) (ciframework.Workflow, error) {
	goFiles, err := repositoryFiles(root, ".go")
	if err != nil {
		return ciframework.Workflow{}, err
	}
	builder := repositoryWorkflowBuilder{
		root: root, bin: filepath.Join(root, "bin"), goFiles: goFiles,
	}

	var steps []ciframework.Step
	switch name {
	case "format":
		steps, _ = builder.format(nil)
	case "check-format":
		steps, _ = builder.checkFormat(nil)
	case "lint":
		steps, _ = builder.lint(nil)
	case "test":
		steps, _ = builder.test(nil)
	case "build":
		steps, _ = builder.build(nil)
	case "check":
		var tail []string
		var group []ciframework.Step
		group, tail = builder.checkFormat(tail)
		steps = append(steps, group...)
		group, tail = builder.build(tail)
		steps = append(steps, group...)
		group, tail = builder.lint(tail)
		steps = append(steps, group...)
		group, _ = builder.test(tail)
		steps = append(steps, group...)
	default:
		return ciframework.Workflow{}, usageError(fmt.Sprintf("unknown repository task %q", name))
	}

	return ciframework.Workflow{
		Name:        "repository-" + name,
		Description: "monorepo " + name,
		Steps:       steps,
	}, nil
}

type repositoryWorkflowBuilder struct {
	root    string
	bin     string
	goFiles []string
}

func (builder *repositoryWorkflowBuilder) format(after []string) (steps []ciframework.Step, tail []string) {
	steps = make([]ciframework.Step, 0, 3)
	steps = append(
		steps,
		builder.step("goimports", builder.root, "goimports", append([]string{"-w"}, builder.goFiles...), after),
		builder.step("gofumpt", builder.root, "gofumpt", append([]string{"-w"}, builder.goFiles...), []string{"goimports"}),
		builder.step("go-mod-tidy", builder.root, "go", []string{"mod", "tidy"}, []string{"gofumpt"}),
	)
	return steps, []string{"go-mod-tidy"}
}

func (builder *repositoryWorkflowBuilder) checkFormat(after []string) (steps []ciframework.Step, tail []string) {
	goimports := builder.step("goimports-check", builder.root, "goimports", append([]string{"-l"}, builder.goFiles...), after)
	goimports.RequireEmptyStdout = true
	gofumpt := builder.step("gofumpt-check", builder.root, "gofumpt", append([]string{"-l"}, builder.goFiles...), []string{"goimports-check"})
	gofumpt.RequireEmptyStdout = true
	steps = []ciframework.Step{
		goimports,
		gofumpt,
		builder.step("go-mod-check", builder.root, "go", []string{"mod", "tidy", "-diff"}, []string{"gofumpt-check"}),
	}
	return steps, []string{"go-mod-check"}
}

func (builder *repositoryWorkflowBuilder) lint(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("repolint", builder.root, "go", []string{"run", "./tools/repolint", "."}, after),
		builder.step("go-lint", builder.root, "golangci-lint", []string{"run", "-c", ".golangci.yml"}, []string{"repolint"}),
	}
	return steps, []string{"go-lint"}
}

func (builder *repositoryWorkflowBuilder) test(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("go-test", builder.root, "go", []string{"test", "-tags", "debug", "./..."}, after),
	}
	return steps, []string{"go-test"}
}

func (builder *repositoryWorkflowBuilder) build(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("bin-directory", builder.root, "mkdir", []string{"-p", builder.bin}, after),
		builder.step("build-tethux", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux"), "./cmd/tethux"}, []string{"bin-directory"}),
		builder.step("build-bridge", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux-bridge"), "./cmd/bridge/main"}, []string{"build-tethux"}),
		builder.step("build-virt", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux-virt"), "./cmd/virt/main"}, []string{"build-bridge"}),
		builder.step("build-ci", builder.root, "go", []string{"build", "-o", filepath.Join(builder.bin, "tethux-ci"), "./tools/ci"}, []string{"build-virt"}),
		builder.step("link-bridge", builder.bin, "ln", []string{"-sf", "tethux", "bridge"}, []string{"build-ci"}),
		builder.step("link-virt", builder.bin, "ln", []string{"-sf", "tethux", "virt"}, []string{"link-bridge"}),
	}
	return steps, []string{"link-virt"}
}

func (builder *repositoryWorkflowBuilder) step(name, dir, command string, args, after []string) ciframework.Step {
	return ciframework.Step{
		Name:      name,
		Command:   command,
		Args:      args,
		Dir:       dir,
		DependsOn: after,
		Timeout:   repositoryStepTimeout,
	}
}

func repositoryFiles(root, extension string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".jj" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "dagger.gen.go" {
			return nil
		}
		if !entry.IsDir() && filepath.Ext(path) == extension {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, relative)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}
