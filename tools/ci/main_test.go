package main

import (
	"slices"
	"strings"
	"testing"

	ciframework "github.com/tethux/tethux/internal/ci"
)

func TestLaptopWorkflowRunsContainerAndHypervisorHostIntegration(t *testing.T) {
	workflow, err := workflowFor("laptop", t.TempDir(), "docker", "all", "test-run")
	if err != nil {
		t.Fatal(err)
	}

	wanted := map[string]bool{
		"build-cli":    false,
		"bridge":       false,
		"provider":     false,
		"topology":     false,
		"qemu-version": false,
		"virsh-list":   false,
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

func TestLaptopWorkflowScopesHostInterfacesToCompleteRunID(t *testing.T) {
	first, err := workflowFor("laptop", t.TempDir(), "docker", "all", "same-prefix-aaa")
	if err != nil {
		t.Fatal(err)
	}
	second, err := workflowFor("laptop", t.TempDir(), "docker", "all", "same-prefix-bbb")
	if err != nil {
		t.Fatal(err)
	}

	firstArgs := workflowStep(t, &first, "dummy-add").Args
	secondArgs := workflowStep(t, &second, "dummy-add").Args
	if slices.Equal(firstArgs, secondArgs) {
		t.Fatalf("distinct runs share dummy interface arguments: %#v", firstArgs)
	}
	if len(firstArgs) < 3 || len(firstArgs[2]) > 15 || strings.Contains(firstArgs[2], "tethux-dummy0") {
		t.Fatalf("dummy interface is not safely run-scoped: %#v", firstArgs)
	}
	for _, stepName := range []string{"dummy-address", "dummy-delete"} {
		if !slices.Contains(workflowStep(t, &first, stepName).Args, firstArgs[2]) {
			t.Fatalf("%s does not use scoped dummy interface %q", stepName, firstArgs[2])
		}
	}

	tapArgs := workflowStep(t, &first, "tap-add").Args
	if len(tapArgs) < 4 || len(tapArgs[3]) > 15 || strings.Contains(tapArgs[3], "tethux-tap0") {
		t.Fatalf("TAP interface is not safely run-scoped: %#v", tapArgs)
	}
	if !slices.Contains(workflowStep(t, &first, "tap-delete").Args, tapArgs[3]) {
		t.Fatalf("tap-delete does not use scoped TAP interface %q", tapArgs[3])
	}
}

func TestHypervisorWorkflowScopesHostInterfacesToCompleteRunID(t *testing.T) {
	first, err := workflowFor("hypervisors", t.TempDir(), "", "", "same-prefix-aaa")
	if err != nil {
		t.Fatal(err)
	}
	second, err := workflowFor("hypervisors", t.TempDir(), "", "", "same-prefix-bbb")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(workflowStep(t, &first, "dummy-add").Args, workflowStep(t, &second, "dummy-add").Args) {
		t.Fatal("standalone hypervisor workflows share host interface names")
	}
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
