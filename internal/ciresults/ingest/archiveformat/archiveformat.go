package archiveformat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Project       Project           `json:"project"`
	Source        *Source           `json:"source,omitempty"`
	Git           Git               `json:"git"`
	Timing        Timing            `json:"timing"`
	Runner        Runner            `json:"runner"`
	Software      Software          `json:"software"`
	Environment   map[string]any    `json:"environment,omitempty"`
	Summary       Summary           `json:"summary"`
	Files         []ArchiveFile     `json:"files"`
	Labels        map[string]string `json:"labels,omitempty"` // TODO: make this typed instead of a map so stuff will be filterable by test level like integration nw mode etc
}

type Software struct {
	GoVersion                 string            `json:"go_version,omitempty"`
	TestRunnerVersion         string            `json:"test_runner_version,omitempty"`
	ProjectBinaryVersion      string            `json:"project_binary_version,omitempty"`
	UBridgeReplacementVersion string            `json:"ubridge_replacement_version,omitempty"`
	ContainerRuntimes         ContainerRuntimes `json:"container_runtime,omitempty"`
}

type ContainerRuntime string

const (
	ContainerRuntimeDocker     ContainerRuntime = "docker"
	ContainerRuntimePodman     ContainerRuntime = "podman"
	ContainerRuntimeContainerd ContainerRuntime = "containerd"
)

func (r ContainerRuntime) Valid() bool {
	switch r {
	case ContainerRuntimeDocker,
		ContainerRuntimePodman,
		ContainerRuntimeContainerd:
		return true
	default:
		return false
	}
}

type ContainerRuntimes []ContainerRuntime

func (r *ContainerRuntime) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("container runtime must be a string: %w", err)
	}

	runtime := ContainerRuntime(value)
	if runtime != "" && !runtime.Valid() {
		return fmt.Errorf(
			"unsupported container runtime %q; expected docker, podman, or containerd",
			value,
		)
	}

	*r = runtime
	return nil
}

func (r *ContainerRuntimes) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("container_runtime must be a string: %w", err)
	}

	if value == "" {
		*r = nil
		return nil
	}

	parts := strings.Split(value, "+")
	runtimes := make(ContainerRuntimes, 0, len(parts))
	seen := make(map[ContainerRuntime]struct{}, len(parts))

	for _, part := range parts {
		runtime := ContainerRuntime(strings.TrimSpace(part))

		if !runtime.Valid() {
			return fmt.Errorf(
				"unsupported container runtime %q; expected docker, podman, or containerd",
				part,
			)
		}

		if _, duplicate := seen[runtime]; duplicate {
			return fmt.Errorf("duplicate container runtime %q", runtime)
		}

		seen[runtime] = struct{}{}
		runtimes = append(runtimes, runtime)
	}

	*r = runtimes
	return nil
}

func (r ContainerRuntimes) MarshalJSON() ([]byte, error) {
	values := make([]string, len(r))

	for i, runtime := range r {
		if !runtime.Valid() {
			return nil, fmt.Errorf("unsupported container runtime %q", runtime)
		}
		values[i] = string(runtime)
	}

	return json.Marshal(strings.Join(values, "+"))
}

type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Repository string `json:"repository,omitempty"`
}

type Source struct {
	Type     SourceType  `json:"type,omitempty"`
	Provider string      `json:"provider,omitempty"`
	Workflow string      `json:"workflow,omitempty"`
	Job      string      `json:"job,omitempty"`
	Attempt  int         `json:"attempt,omitempty"`
	Trigger  TriggerType `json:"trigger,omitempty"`
}

type SourceType string

const (
	SourceTypeCI        SourceType = "ci"
	SourceTypeLocal     SourceType = "local"
	SourceTypeScheduled SourceType = "scheduled"
	SourceTypeManual    SourceType = "manual"
)

func (v *SourceType) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "source type", true, SourceTypeCI, SourceTypeLocal, SourceTypeScheduled, SourceTypeManual)
}

type TriggerType string

const (
	TriggerPush        TriggerType = "push"
	TriggerPullRequest TriggerType = "pull_request"
	TriggerSchedule    TriggerType = "schedule"
	TriggerManual      TriggerType = "manual"
)

func (v *TriggerType) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "trigger", true, TriggerPush, TriggerPullRequest, TriggerSchedule, TriggerManual)
}

type Git struct {
	CommitSHA       string     `json:"commit_sha"`
	Branch          string     `json:"branch,omitempty"`
	Tag             *string    `json:"tag,omitempty"`
	Dirty           *bool      `json:"dirty,omitempty"`
	CommitTimestamp *time.Time `json:"commit_timestamp,omitempty"`
}

type Timing struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

type Runner struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name,omitempty"`
	Hostname    string `json:"hostname,omitempty"`

	OS           string `json:"os"`
	OSVersion    string `json:"os_version,omitempty"`
	Kernel       string `json:"kernel,omitempty"`
	Architecture string `json:"architecture"`

	CPU         string `json:"cpu,omitempty"`
	MemoryBytes *int64 `json:"memory_bytes,omitempty"`

	ContainerRuntime        ContainerRuntime `json:"container_runtime,omitempty"`
	ContainerRuntimeVersion string           `json:"container_runtime_version,omitempty"`
	FixtureImages           []string         `json:"fixture_images,omitempty"`
}

type Summary struct {
	Status    RunStatus `json:"status"`
	Total     int64     `json:"total"`
	Passed    int64     `json:"passed"`
	Failed    int64     `json:"failed"`
	Skipped   int64     `json:"skipped"`
	Errored   int64     `json:"errored"`
	Cancelled int64     `json:"cancelled,omitempty"`
}

type RunStatus string

const (
	RunStatusPassed    RunStatus = "passed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusError     RunStatus = "error"
	RunStatusCancelled RunStatus = "cancelled"
)

func (v *RunStatus) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "run status", false, RunStatusPassed, RunStatusFailed, RunStatusError, RunStatusCancelled)
}

type ArchiveFile struct {
	Path      string   `json:"path"`
	Type      FileType `json:"type"`
	MediaType string   `json:"media_type"`
	SizeBytes int64    `json:"size_bytes"`
	SHA256    string   `json:"sha256"`
	Public    bool     `json:"public"`
}

type FileType string

const (
	FileTypeArtifact      FileType = "artifact"
	FileTypeConfig        FileType = "config"
	FileTypeLog           FileType = "log"
	FileTypeResults       FileType = "results"
	FileTypePacketCapture FileType = "packet_capture"
)

func (v *FileType) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "archive file type", false, FileTypeArtifact, FileTypeConfig, FileTypeLog, FileTypeResults, FileTypePacketCapture)
}

type ResultsDocument struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	Tests         []TestResult `json:"tests"`
}

type TestResult struct {
	TestID string     `json:"test_id"`
	Name   string     `json:"name"`
	Suite  string     `json:"suite"`
	Status TestStatus `json:"status"`

	Timing  ResultTiming `json:"timing"`
	Attempt int64        `json:"attempt"`

	Source *ResultSource `json:"source,omitempty"`

	Features   []string       `json:"features,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Metrics    map[string]any `json:"metrics,omitempty"`

	Message *string  `json:"message"`
	Failure *Failure `json:"failure"`

	Artifacts []string          `json:"artifacts"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type ResultTiming struct {
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS int64      `json:"duration_ms"`
}

type ResultSource struct {
	File   string  `json:"file,omitempty"`
	Symbol *string `json:"symbol"`
	Line   *int64  `json:"line"`
}

type Failure struct {
	Kind       FailureKind `json:"kind,omitempty"`
	Phase      string      `json:"phase,omitempty"`
	Expected   *string     `json:"expected,omitempty"`
	Actual     *string     `json:"actual,omitempty"`
	ErrorCode  string      `json:"error_code,omitempty"`
	StackTrace *string     `json:"stack_trace,omitempty"`
}

type TestStatus string

const (
	TestStatusPassed    TestStatus = "passed"
	TestStatusFailed    TestStatus = "failed"
	TestStatusSkipped   TestStatus = "skipped"
	TestStatusError     TestStatus = "error"
	TestStatusCancelled TestStatus = "cancelled"
)

func (v *TestStatus) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "test status", false, TestStatusPassed, TestStatusFailed, TestStatusSkipped, TestStatusError, TestStatusCancelled)
}

type FailureKind string

const (
	FailureAssertion          FailureKind = "assertion"
	FailureTimeout            FailureKind = "timeout"
	FailureProcessExit        FailureKind = "process_exit"
	FailureCrash              FailureKind = "crash"
	FailureSetup              FailureKind = "setup"
	FailureCleanup            FailureKind = "cleanup"
	FailureNetwork            FailureKind = "network"
	FailureResourceExhaustion FailureKind = "resource_exhaustion"
	FailureUnsupported        FailureKind = "unsupported"
	FailureUnknown            FailureKind = "unknown"
)

func (v *FailureKind) UnmarshalJSON(data []byte) error {
	return decodeEnum(data, v, "failure kind", true, FailureAssertion, FailureTimeout, FailureProcessExit, FailureCrash, FailureSetup, FailureCleanup, FailureNetwork, FailureResourceExhaustion, FailureUnsupported, FailureUnknown)
}

func decodeEnum[T ~string](data []byte, target *T, name string, optional bool, allowed ...T) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", name, err)
	}
	if optional && value == "" {
		*target = ""
		return nil
	}
	for _, candidate := range allowed {
		if value == string(candidate) {
			*target = candidate
			return nil
		}
	}
	return fmt.Errorf("unsupported %s %q", name, value)
}
