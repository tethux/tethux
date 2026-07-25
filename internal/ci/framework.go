// Package ci provides the declarative execution primitives used by repository
// automation. It intentionally does not parse command-line flags; tools/ci is
// the adapter between operators or CI providers and these reusable APIs.
package ci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Privilege string

const (
	PrivilegeUser Privilege = "user"
	PrivilegeRoot Privilege = "root"
)

type Output struct {
	Name string
	Path string
	Kind string
}

type Step struct {
	Name          string
	Command       string
	Args          []string
	Env           map[string]string
	Dir           string
	DependsOn     []string
	Privilege     Privilege
	Outputs       []Output
	CaptureStdout string
	Timeout       time.Duration
	Always        bool
	AllowMissing  bool
}

type Workflow struct {
	Name            string
	Description     string
	Steps           []Step
	Labels          map[string]string
	Archive         ArchiveMetadata
	ContinueOnError bool
}

type ArchiveMetadata struct {
	Workflow string
	DeviceID string
	Runtime  string
}

type StepResult struct {
	Name       string        `json:"name"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
	ExitCode   int           `json:"exit_code"`
	Error      string        `json:"error,omitempty"`
	Outputs    []Output      `json:"outputs,omitempty"`
}

type WorkflowResult struct {
	Name       string       `json:"name"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Steps      []StepResult `json:"steps"`
}

type Runner struct {
	Stdout  io.Writer
	Stderr  io.Writer
	DryRun  bool
	BaseEnv map[string]string
}

func NewRunner(stdout, stderr io.Writer) *Runner {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Runner{Stdout: stdout, Stderr: stderr}
}

func (r *Runner) Run(ctx context.Context, workflow Workflow) (WorkflowResult, error) {
	if err := ValidateWorkflow(workflow); err != nil {
		return WorkflowResult{}, err
	}
	result := WorkflowResult{Name: workflow.Name, StartedAt: time.Now().UTC()}
	completed := make(map[string]StepResult, len(workflow.Steps))
	pending := append([]Step(nil), workflow.Steps...)
	var workflowErr error
	for len(pending) > 0 {
		progress := false
		next := pending[:0]
		for _, step := range pending {
			if !dependenciesComplete(step.DependsOn, completed) {
				next = append(next, step)
				continue
			}
			progress = true
			if workflowErr != nil && !workflow.ContinueOnError && !step.Always {
				skipped := StepResult{Name: step.Name, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), ExitCode: -1, Error: "skipped after previous failure"}
				completed[step.Name] = skipped
				result.Steps = append(result.Steps, skipped)
				continue
			}
			stepResult, err := r.RunStep(ctx, step)
			completed[step.Name] = stepResult
			result.Steps = append(result.Steps, stepResult)
			if err != nil && workflowErr == nil {
				workflowErr = fmt.Errorf("step %s: %w", step.Name, err)
			}
		}
		if !progress {
			return result, fmt.Errorf("workflow %q has unresolved dependencies", workflow.Name)
		}
		pending = append([]Step(nil), next...)
	}
	result.FinishedAt = time.Now().UTC()
	return result, workflowErr
}

func (r *Runner) RunStep(ctx context.Context, step Step) (StepResult, error) {
	result := StepResult{Name: step.Name, StartedAt: time.Now().UTC(), Outputs: step.Outputs}
	stepCtx := ctx
	cancel := func() {}
	if step.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
	}
	defer cancel()

	command, args := step.Command, append([]string(nil), step.Args...)
	if step.Privilege == PrivilegeRoot && os.Geteuid() != 0 {
		args = append([]string{"-n", command}, args...)
		command = "sudo"
	}
	fmt.Fprintf(r.Stdout, "==> %s\n    %s %s\n", step.Name, command, strings.Join(args, " "))
	if r.DryRun {
		result.FinishedAt = time.Now().UTC()
		return result, nil
	}
	if _, err := exec.LookPath(command); err != nil && step.AllowMissing {
		fmt.Fprintf(r.Stdout, "    skipped: %s is not installed\n", command)
		result.FinishedAt = time.Now().UTC()
		return result, nil
	}
	cmd := exec.CommandContext(stepCtx, command, args...)
	cmd.Dir = step.Dir
	cmd.Env = mergedEnvironment(r.BaseEnv, step.Env)
	stdout := r.Stdout
	var capture *os.File
	if step.CaptureStdout != "" {
		if err := os.MkdirAll(filepath.Dir(step.CaptureStdout), 0o750); err != nil {
			return result, fmt.Errorf("create captured output directory: %w", err)
		}
		var err error
		capture, err = os.OpenFile(step.CaptureStdout, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
		if err != nil {
			return result, fmt.Errorf("open captured output: %w", err)
		}
		defer capture.Close()
		stdout = io.MultiWriter(r.Stdout, capture)
	}
	cmd.Stdout = stdout
	cmd.Stderr = r.Stderr
	err := cmd.Run()
	result.FinishedAt = time.Now().UTC()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	if err == nil {
		return result, nil
	}
	result.ExitCode = 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	if stepCtx.Err() != nil {
		err = stepCtx.Err()
	}
	result.Error = err.Error()
	return result, err
}

func ValidateWorkflow(workflow Workflow) error {
	if workflow.Name == "" {
		return errors.New("workflow name is required")
	}
	names := make(map[string]struct{}, len(workflow.Steps))
	for _, step := range workflow.Steps {
		if step.Name == "" || step.Command == "" {
			return errors.New("every workflow step requires a name and command")
		}
		if _, duplicate := names[step.Name]; duplicate {
			return fmt.Errorf("duplicate step %q", step.Name)
		}
		names[step.Name] = struct{}{}
	}
	for _, step := range workflow.Steps {
		for _, dependency := range step.DependsOn {
			if _, ok := names[dependency]; !ok {
				return fmt.Errorf("step %q depends on unknown step %q", step.Name, dependency)
			}
			if dependency == step.Name {
				return fmt.Errorf("step %q depends on itself", step.Name)
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	byName := make(map[string]Step, len(workflow.Steps))
	for _, step := range workflow.Steps {
		byName[step.Name] = step
	}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("workflow contains a dependency cycle at %q", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dependency := range byName[name].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for name := range names {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func dependenciesComplete(dependencies []string, completed map[string]StepResult) bool {
	for _, dependency := range dependencies {
		if _, ok := completed[dependency]; !ok {
			return false
		}
	}
	return true
}

func mergedEnvironment(base, overlay map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range base {
		values[key] = value
	}
	for key, value := range overlay {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func RepositoryRoot() (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find repository root: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(output))), nil
}
