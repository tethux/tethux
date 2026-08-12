package ci

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidateWorkflowRejectsCycle(t *testing.T) {
	workflow := Workflow{Name: "cycle", Steps: []Step{
		{Name: "a", Command: "true", DependsOn: []string{"b"}},
		{Name: "b", Command: "true", DependsOn: []string{"a"}},
	}}
	if err := ValidateWorkflow(workflow); err == nil {
		t.Fatal("expected dependency cycle to fail")
	}
}

func TestRunnerPreservesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix helper")
	}
	runner := NewRunner(&bytes.Buffer{}, &bytes.Buffer{})
	result, err := runner.RunStep(context.Background(), Step{
		Name: "failure", Command: "sh", Args: []string{"-c", "exit 17"}, Timeout: time.Second,
	})
	if err == nil || result.ExitCode != 17 {
		t.Fatalf("expected exit 17, got result=%+v err=%v", result, err)
	}
}

func TestRunnerEnforcesEmptyStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix helper")
	}
	runner := NewRunner(&bytes.Buffer{}, &bytes.Buffer{})
	result, err := runner.RunStep(context.Background(), Step{
		Name: "format-check", Command: "sh", Args: []string{"-c", "printf changed.go"},
		RequireEmptyStdout: true,
	})
	if err == nil || result.ExitCode != 0 || !strings.Contains(err.Error(), "changed.go") {
		t.Fatalf("expected an output-contract failure, got result=%+v err=%v", result, err)
	}
}

func TestRunnerAcceptsConfiguredExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix helper")
	}
	runner := NewRunner(&bytes.Buffer{}, &bytes.Buffer{})
	result, err := runner.RunStep(context.Background(), Step{
		Name: "version-probe", Command: "sh", Args: []string{"-c", "exit 1"},
		AllowedExitCodes: []int{1},
	})
	if err != nil {
		t.Fatalf("configured exit code should pass: result=%+v err=%v", result, err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
}

func TestRunnerDoesNotLetAllowedExitCodeMaskTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix helper")
	}
	runner := NewRunner(&bytes.Buffer{}, &bytes.Buffer{})
	result, err := runner.RunStep(context.Background(), Step{
		Name: "timeout", Command: "sh", Args: []string{"-c", "sleep 5"},
		Timeout: 10 * time.Millisecond, AllowedExitCodes: []int{-1},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout was masked: result=%+v err=%v", result, err)
	}
}

func TestMergedEnvironmentOverlayWins(t *testing.T) {
	env := mergedEnvironment(map[string]string{"TETHUX_TEST_VALUE": "base"}, map[string]string{"TETHUX_TEST_VALUE": "step"})
	found := false
	for _, value := range env {
		if value == "TETHUX_TEST_VALUE=step" {
			found = true
		}
	}
	if !found {
		t.Fatal("step environment did not override base")
	}
}

func TestRemoteArgsDoNotUseShell(t *testing.T) {
	remote := Remote{Target: "ci@example", JumpHost: "jump@example"}
	args, err := remote.SSHArgs("hostname")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) == 0 || args[len(args)-1] != "hostname" {
		t.Fatalf("unexpected args: %#v", args)
	}
	for _, option := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "UpdateHostKeys=no"} {
		if !slices.Contains(args, option) {
			t.Fatalf("missing SSH transport option %q in %#v", option, args)
		}
	}
}

func TestRootCommandPreservesOnlyToolchainEnvironment(t *testing.T) {
	args := rootCommandArgs("go", []string{"test", "./..."}, []string{
		"PATH=/nix/store/bin",
		"CGO_CFLAGS=-I/nix/store/libpcap/include",
		"LD_LIBRARY_PATH=/nix/store/libpcap/lib",
		"TETHUX_RUN_ID=019fe7e4-daf2-7d54-9f77-980bc8619fca",
		"SECRET=not-for-root",
	})
	joined := strings.Join(args, " ")
	for _, expected := range []string{"-n env", "PATH=/nix/store/bin", "CGO_CFLAGS=", "LD_LIBRARY_PATH=", "TETHUX_RUN_ID=019fe7e4-daf2-7d54-9f77-980bc8619fca", "go test ./..."} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %#v", expected, args)
		}
	}
	if strings.Contains(joined, "SECRET=") {
		t.Fatalf("unexpected environment leak in %#v", args)
	}
}

func TestRootCopyCommandIsInteractiveAndShellQuoted(t *testing.T) {
	command := rootCopyCommand(
		"/tmp/repository with spaces",
		"go",
		[]string{"run", "./tools/example", "value with spaces"},
		[]string{"PATH=/nix/store/bin", "SECRET=not-for-root"},
	)
	for _, expected := range []string{
		"cd '/tmp/repository with spaces' && sudo env",
		"PATH=/nix/store/bin",
		"'value with spaces'",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("copy command %q does not contain %q", command, expected)
		}
	}
	if strings.Contains(command, "-n") {
		t.Fatalf("copy command must allow interactive sudo: %q", command)
	}
	if strings.Contains(command, "SECRET=") {
		t.Fatalf("copy command leaked unrelated environment: %q", command)
	}
}

func TestIsCIUsesExplicitRunnerMarker(t *testing.T) {
	t.Setenv(CIEnvironmentVariable, "")
	if IsCI() {
		t.Fatal("local environment detected as CI")
	}
	t.Setenv(CIEnvironmentVariable, "1")
	if !IsCI() {
		t.Fatal("explicit tethux CI runner was not detected")
	}
}
