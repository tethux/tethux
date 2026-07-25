package main

import (
	"slices"
	"testing"
)

func TestLaptopWorkflowOnlyRunsHostIntegration(t *testing.T) {
	workflow, err := workflowFor("laptop", t.TempDir(), "docker", "all")
	if err != nil {
		t.Fatal(err)
	}

	if len(workflow.Steps) != 4 {
		t.Fatalf("expected one CLI build and three host integration steps, got %d", len(workflow.Steps))
	}
	for _, step := range workflow.Steps {
		switch step.Name {
		case "build-cli", "bridge", "provider", "topology":
		default:
			t.Fatalf("laptop workflow contains duplicated repository check %q", step.Name)
		}
	}
	bridge := workflow.Steps[1]
	if len(bridge.DependsOn) != 1 || bridge.DependsOn[0] != "build-cli" {
		t.Fatalf("bridge step does not depend on the CLI build: %#v", bridge.DependsOn)
	}
	topology := workflow.Steps[3]
	if !slices.Contains(topology.Args, "--interface-timeout") || !slices.Contains(topology.Args, "45s") {
		t.Fatalf("topology step lacks the host startup allowance: %#v", topology.Args)
	}
}
