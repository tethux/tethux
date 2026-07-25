package ci

import (
	"bytes"
	"context"
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
		"SECRET=not-for-root",
	})
	joined := strings.Join(args, " ")
	for _, expected := range []string{"-n env", "PATH=/nix/store/bin", "CGO_CFLAGS=", "LD_LIBRARY_PATH=", "go test ./..."} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %#v", expected, args)
		}
	}
	if strings.Contains(joined, "SECRET=") {
		t.Fatalf("unexpected environment leak in %#v", args)
	}
}
