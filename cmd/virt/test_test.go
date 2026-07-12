package virt

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEventWriterJSONSchema(t *testing.T) {
	var output bytes.Buffer
	writer, err := newEventWriterTo("json", "ci-laptop", &output)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.emit(testEvent{Provider: "containerd", Operation: "exec", Status: "passed"}); err != nil {
		t.Fatal(err)
	}

	var event testEvent
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("structured output is not JSON: %v", err)
	}
	if event.Schema != "tethux.provider-test/v1" {
		t.Fatalf("schema = %q", event.Schema)
	}
	if event.Host != "ci-laptop" || event.Provider != "containerd" || event.Operation != "exec" || event.Status != "passed" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Timestamp.IsZero() {
		t.Fatal("timestamp was not populated")
	}
}

func TestProviderTestDefaultsToTwoImages(t *testing.T) {
	if len(defaultTestImages) != 2 {
		t.Fatalf("default image count = %d, want 2", len(defaultTestImages))
	}
	if defaultTestImages[0] == defaultTestImages[1] {
		t.Fatal("provider test images must be distinct")
	}
}
