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
	sqlFiles, err := repositorySQLFiles(root)
	if err != nil {
		return ciframework.Workflow{}, err
	}

	builder := repositoryWorkflowBuilder{
		root:     root,
		web:      filepath.Join(root, "tools", "ci-results", "viewer", "frontend"),
		bin:      filepath.Join(root, "bin"),
		goFiles:  goFiles,
		sqlFiles: sqlFiles,
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
	case "test-web":
		steps = []ciframework.Step{builder.webDependencies("web-dependencies", nil), builder.webTest([]string{"web-dependencies"})}
	case "build":
		steps, _ = builder.build(nil)
	case "build-web":
		steps = []ciframework.Step{builder.webDependencies("web-dependencies", nil), builder.webBuild([]string{"web-dependencies"})}
	case "install-web":
		steps = []ciframework.Step{builder.webDependencies("web-dependencies", nil)}
	case "check":
		var tail []string
		var group []ciframework.Step
		group, tail = builder.checkFormat(tail)
		steps = append(steps, group...)
		group, tail = builder.lint(tail)
		steps = append(steps, group...)
		group, tail = builder.test(tail)
		steps = append(steps, group...)
		group, _ = builder.build(tail)
		steps = append(steps, group...)
		steps = shareWebDependencies(steps)
	default:
		return ciframework.Workflow{}, usageError(fmt.Sprintf("unknown repository task %q", name))
	}

	return ciframework.Workflow{
		Name:        "repository-" + name,
		Description: "monorepo " + name,
		Steps:       steps,
	}, nil
}

func shareWebDependencies(steps []ciframework.Step) []ciframework.Step {
	const shared = "web-dependencies"
	aliases := map[string]string{}
	result := make([]ciframework.Step, 0, len(steps))
	for index := range steps {
		step := steps[index]
		if filepath.Base(step.Dir) == "frontend" && step.Command == "bun" && len(step.Args) > 0 && step.Args[0] == "install" {
			aliases[step.Name] = shared
			if len(aliases) == 1 {
				step.Name = shared
				result = append(result, step)
			}
			continue
		}
		result = append(result, step)
	}
	for index := range result {
		for dependency := range result[index].DependsOn {
			if replacement, ok := aliases[result[index].DependsOn[dependency]]; ok {
				result[index].DependsOn[dependency] = replacement
			}
		}
	}
	return result
}

type repositoryWorkflowBuilder struct {
	root     string
	web      string
	bin      string
	goFiles  []string
	sqlFiles []string
}

func (builder *repositoryWorkflowBuilder) format(after []string) (steps []ciframework.Step, tail []string) {
	steps = make([]ciframework.Step, 0, 5+len(builder.sqlFiles))
	steps = append(steps,
		builder.step("goimports", builder.root, "goimports", append([]string{"-w"}, builder.goFiles...), after),
		builder.step("gofumpt", builder.root, "gofumpt", append([]string{"-w"}, builder.goFiles...), []string{"goimports"}),
		builder.step("go-mod-tidy", builder.root, "go", []string{"mod", "tidy"}, []string{"gofumpt"}),
	)
	previous := "go-mod-tidy"
	for index, path := range builder.sqlFiles {
		name := fmt.Sprintf("sql-%03d", index+1)
		steps = append(steps, builder.step(name, builder.root, "sleek", []string{path}, []string{previous}))
		previous = name
	}
	steps = append(steps,
		builder.webDependencies("format-web-dependencies", []string{previous}),
		builder.step("web-format", builder.web, "bun", []string{"run", "format"}, []string{"format-web-dependencies"}),
	)
	return steps, []string{"web-format"}
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
		builder.webDependencies("check-web-dependencies", []string{"go-mod-check"}),
		builder.step("web-format-check", builder.web, "bun", []string{"run", "format:check"}, []string{"check-web-dependencies"}),
	}
	return steps, []string{"web-format-check"}
}

func (builder *repositoryWorkflowBuilder) lint(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("assert-lint", builder.root, "go", []string{"run", "./tools/assertlint", "."}, after),
		builder.step("go-lint", builder.root, "golangci-lint", []string{"run", "-c", ".golangci.yml"}, []string{"assert-lint"}),
		builder.webDependencies("lint-web-dependencies", []string{"go-lint"}),
		builder.step("web-lint", builder.web, "bun", []string{"run", "lint"}, []string{"lint-web-dependencies"}),
	}
	return steps, []string{"web-lint"}
}

func (builder *repositoryWorkflowBuilder) test(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("go-test", builder.root, "go", []string{"test", "-tags", "debug", "./..."}, after),
		builder.webDependencies("test-web-dependencies", []string{"go-test"}),
		builder.webTest([]string{"test-web-dependencies"}),
	}
	return steps, []string{"web-test"}
}

func (builder *repositoryWorkflowBuilder) build(after []string) (steps []ciframework.Step, tail []string) {
	steps = []ciframework.Step{
		builder.step("bin-directory", builder.root, "mkdir", []string{"-p", builder.bin}, after),
		builder.webDependencies("build-web-dependencies", []string{"bin-directory"}),
		builder.webBuild([]string{"build-web-dependencies"}),
		builder.step("build-tethux", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux"), "./cmd/tethux"}, []string{"web-build"}),
		builder.step("build-bridge", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux-bridge"), "./cmd/bridge/main"}, []string{"build-tethux"}),
		builder.step("build-virt", builder.root, "go", []string{"build", "-tags", "debug", "-o", filepath.Join(builder.bin, "tethux-virt"), "./cmd/virt/main"}, []string{"build-bridge"}),
		builder.step("build-results", builder.root, "go", []string{"build", "-o", filepath.Join(builder.bin, "ci-results"), "./tools/ci-results"}, []string{"build-virt"}),
		builder.step("build-ci", builder.root, "go", []string{"build", "-o", filepath.Join(builder.bin, "tethux-ci"), "./tools/ci"}, []string{"build-results"}),
		builder.step("link-bridge", builder.bin, "ln", []string{"-sf", "tethux", "bridge"}, []string{"build-ci"}),
		builder.step("link-virt", builder.bin, "ln", []string{"-sf", "tethux", "virt"}, []string{"link-bridge"}),
	}
	return steps, []string{"link-virt"}
}

func (builder *repositoryWorkflowBuilder) webDependencies(name string, after []string) ciframework.Step {
	return builder.step(name, builder.web, "bun", []string{"install", "--frozen-lockfile"}, after)
}

func (builder *repositoryWorkflowBuilder) webTest(after []string) ciframework.Step {
	return builder.step("web-test", builder.web, "bun", []string{"run", "check"}, after)
}

func (builder *repositoryWorkflowBuilder) webBuild(after []string) ciframework.Step {
	return builder.step("web-build", builder.web, "bun", []string{"run", "build"}, after)
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

func repositorySQLFiles(root string) ([]string, error) {
	var paths []string
	for _, pattern := range []string{"internal/ciresults/db/queries/*.sql", "internal/ciresults/db/migrations/*.sql"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", pattern, err)
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	return paths, nil
}
