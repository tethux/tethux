package virt

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
)

func TestEventWriterJSONSchema(t *testing.T) {
	var output bytes.Buffer
	writer, err := newEventWriterTo("json", "ci-laptop", &output)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.emit(&testEvent{Provider: "containerd", Operation: "exec", Status: "passed"}); err != nil {
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

func TestResolveProviderTestRunIDUsesCanonicalUUIDv7(t *testing.T) {
	runID, err := resolveTestRunID("019FE7E4-DAF2-7D54-9F77-980BC8619FCA")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "019fe7e4-daf2-7d54-9f77-980bc8619fca" {
		t.Fatalf("run ID was not canonicalized: %q", runID)
	}
	generated, err := resolveTestRunID("")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 36 || generated[14] != '7' {
		t.Fatalf("generated run ID is not UUIDv7: %q", generated)
	}
	if _, err := resolveTestRunID("550e8400-e29b-41d4-a716-446655440000"); err == nil {
		t.Fatal("expected UUIDv4 run ID to be rejected")
	}
}

func TestProviderResourceNameIsScopedToCompleteRunID(t *testing.T) {
	tests := []struct {
		name   string
		first  string
		second string
	}{
		{name: "different UUIDs", first: "019fe7e4-daf2-7d54-9f77-980bc8619fca", second: "019fe7e4-daf2-7d54-9f77-980bc8619fcb"},
		{name: "same readable prefix", first: "same-prefix-aaa", second: "same-prefix-bbb"},
		{name: "sanitization equivalent", first: "run/one", second: "run one"},
		{name: "invalid characters", first: "///", second: "???"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := providerResourceName("docker", tt.first, 0)
			second := providerResourceName("docker", tt.second, 0)
			if first == second {
				t.Fatalf("resource names from distinct runs collide: %q", first)
			}
			if len(first) > 63 {
				t.Fatalf("resource name exceeds the portable 63-character limit: %q", first)
			}
			if !regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`).MatchString(first) {
				t.Fatalf("resource name is not container-runtime safe: %q", first)
			}
		})
	}
}
