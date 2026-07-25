package main

import "testing"

func TestLaptopWorkflowOnlyRunsHostIntegration(t *testing.T) {
	workflow, err := workflowFor("laptop", t.TempDir(), "docker", "all")
	if err != nil {
		t.Fatal(err)
	}

	if len(workflow.Steps) != 3 {
		t.Fatalf("expected three host integration steps, got %d", len(workflow.Steps))
	}
	for _, step := range workflow.Steps {
		switch step.Name {
		case "bridge", "provider", "topology":
		default:
			t.Fatalf("laptop workflow contains duplicated repository check %q", step.Name)
		}
	}
}
