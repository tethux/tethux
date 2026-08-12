package main

import (
	"slices"
	"strings"
	"testing"

	ciframework "github.com/tethux/tethux/internal/ci"
)

func TestLaptopWorkflowRunsContainerIntegration(t *testing.T) {
	workflow, err := workflowFor("laptop", t.TempDir(), "docker", "all", "test-run")
	if err != nil {
		t.Fatal(err)
	}

	wanted := map[string]bool{
		"build-cli": false,
		"bridge":    false,
		"provider":  false,
		"topology":  false,
	}
	for _, step := range workflow.Steps {
		if _, ok := wanted[step.Name]; ok {
			wanted[step.Name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("laptop workflow is missing host integration step %q", name)
		}
	}
	bridge := workflowStep(t, &workflow, "bridge")
	if len(bridge.DependsOn) != 1 || bridge.DependsOn[0] != "build-cli" {
		t.Fatalf("bridge step does not depend on the CLI build: %#v", bridge.DependsOn)
	}
	topology := workflowStep(t, &workflow, "topology")
	if !slices.Contains(topology.Args, "--interface-timeout") || !slices.Contains(topology.Args, "45s") {
		t.Fatalf("topology step lacks the host startup allowance: %#v", topology.Args)
	}
}

func TestBridgeWorkflowBuildsCLIForConformanceTest(t *testing.T) {
	workflow, err := workflowFor("bridge", t.TempDir(), "", "", "test-run")
	if err != nil {
		t.Fatal(err)
	}

	build := workflowStep(t, &workflow, "build-cli")
	bridge := workflowStep(t, &workflow, "bridge")
	if len(bridge.DependsOn) != 1 || bridge.DependsOn[0] != build.Name {
		t.Fatalf("bridge dependencies = %#v, want build-cli", bridge.DependsOn)
	}
	if !slices.Contains(bridge.Args, "--tethux") {
		t.Fatalf("bridge arguments do not provide the CLI binary: %#v", bridge.Args)
	}
}

func TestRepositoryCheckIsOneDeclarativeWorkflow(t *testing.T) {
	workflow, err := repositoryWorkflow(tethuxRoot(t), "check")
	if err != nil {
		t.Fatal(err)
	}
	if err := ciframework.ValidateWorkflow(workflow); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"goimports-check", "go-lint", "go-test", "web-test", "web-build", "build-ci"} {
		step := workflowStep(t, &workflow, name)
		if step.Command == "mise" || step.Timeout == 0 {
			t.Fatalf("step %q is not a complete command declaration: %+v", name, step)
		}
	}
	if !workflowStep(t, &workflow, "goimports-check").RequireEmptyStdout {
		t.Fatal("goimports check does not declare its stdout contract")
	}
}

func tethuxRoot(t *testing.T) string {
	t.Helper()
	root, err := ciframework.RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestResolveRunIDCanonicalizesBeforeWorkflowConstruction(t *testing.T) {
	runID, err := resolveRunID("019FE7E4-DAF2-7D54-9F77-980BC8619FCA")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "019fe7e4-daf2-7d54-9f77-980bc8619fca" {
		t.Fatalf("run ID was not canonicalized: %q", runID)
	}

	generated, err := resolveRunID("")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 36 || generated[14] != '7' {
		t.Fatalf("generated run ID is not UUIDv7: %q", generated)
	}
}

func TestRemoteLaptopExplicitlyMarksNonInteractiveCIRunner(t *testing.T) {
	args := remoteLaptopArgs("/tmp/tethux-ci-deadbeef", "docker", "laptop-100")
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"env -C /tmp/tethux-ci-deadbeef",
		"-c env TETHUX_CI_RUNNER=1 go run ./tools/ci run laptop",
		"--runtime docker",
		"--device laptop-100",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("remote command %q does not contain %q", joined, expected)
		}
	}
}

func workflowStep(t *testing.T, workflow *ciframework.Workflow, name string) *ciframework.Step {
	t.Helper()
	for index := range workflow.Steps {
		if workflow.Steps[index].Name == name {
			return &workflow.Steps[index]
		}
	}
	t.Fatalf("workflow is missing step %q", name)
	return nil
}
