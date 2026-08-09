package ingest

import (
	"testing"

	"github.com/tethux/tethux/internal/ciresults/ingest/archiveformat"
)

func TestNormalizeTestAttemptsPreservesDuplicateResults(t *testing.T) {
	tests := []archiveformat.TestResult{
		{TestID: "event/pull", Attempt: 1},
		{TestID: "event/pull", Attempt: 1},
		{TestID: "event/pull", Attempt: 3},
		{TestID: "other", Attempt: 0},
	}
	normalized := normalizeTestAttempts(tests)
	want := []int64{1, 2, 3, 1}
	for index, attempt := range want {
		if normalized[index].Attempt != attempt {
			t.Fatalf("result %d attempt = %d, want %d", index, normalized[index].Attempt, attempt)
		}
	}
	if tests[1].Attempt != 1 {
		t.Fatal("normalization mutated its input")
	}
}
