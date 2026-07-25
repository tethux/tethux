package ci

import (
	"bytes"
	"context"
	"runtime"
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
}
