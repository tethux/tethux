package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/internal/ciresults/ingest/archiveformat"
	"github.com/0xveya/tethux/internal/ciresults/ingest/crosshostlink"
)

type goTestEvent struct {
	Time    time.Time    `json:"Time"`
	Action  goTestAction `json:"Action"`
	Package string       `json:"Package"`
	Test    string       `json:"Test,omitempty"`
	Elapsed float64      `json:"Elapsed,omitempty"`
	Output  string       `json:"Output,omitempty"`
}

type goTestAction string

const (
	goTestActionStart  goTestAction = "start"
	goTestActionRun    goTestAction = "run"
	goTestActionPause  goTestAction = "pause"
	goTestActionCont   goTestAction = "cont"
	goTestActionBench  goTestAction = "bench"
	goTestActionPass   goTestAction = "pass"
	goTestActionFail   goTestAction = "fail"
	goTestActionSkip   goTestAction = "skip"
	goTestActionPanic  goTestAction = "panic"
	goTestActionOutput goTestAction = "output"
)

func (a goTestAction) String() string {
	switch a {
	case goTestActionStart, goTestActionRun, goTestActionPause,
		goTestActionCont, goTestActionBench,
		goTestActionPass, goTestActionFail, goTestActionSkip,
		goTestActionPanic, goTestActionOutput:
		return string(a)
	default:
		return "unknown"
	}
}

func (a *goTestAction) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("go test action must be a string: %w", err)
	}
	action := goTestAction(value)
	if action.String() == "unknown" {
		return fmt.Errorf("unsupported go test action %q", value)
	}
	*a = action
	return nil
}

func decodeGoTestLogEvent(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	data []byte,
) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var event goTestEvent
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode go test event: %w", err)
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return fmt.Errorf("decode go test event: %w", err)
	}

	return handleGoTestEvent(ctx, store, record, source, event)
}

func handleGoTestEvent(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	event goTestEvent,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Printf(
		"go-test source=%s:%d run=%s package=%s test=%s action=%s elapsed=%f\n",
		source.Path, source.Line, record.RunID, event.Package, event.Test,
		event.Action, event.Elapsed,
	)
	_ = store
	return nil
}

type providerTestEvent struct {
	Schema     string                         `json:"schema"`
	Timestamp  time.Time                      `json:"timestamp"`
	StartedAt  time.Time                      `json:"started_at"`
	FinishedAt time.Time                      `json:"finished_at"`
	Host       string                         `json:"host"`
	Provider   archiveformat.ContainerRuntime `json:"provider"`
	Image      string                         `json:"image,omitempty"`
	API        providerAPI                    `json:"api,omitempty"`
	Operation  string                         `json:"operation"`
	Status     archiveformat.TestStatus       `json:"status"`
	DurationMS int64                          `json:"duration_ms"`
	Details    map[string]any                 `json:"details,omitempty"`
}

type laptopIntegrationEvent struct {
	Schema     string                         `json:"schema"`
	Host       string                         `json:"host"`
	Runtime    archiveformat.ContainerRuntime `json:"runtime"`
	Status     archiveformat.TestStatus       `json:"status"`
	StartedAt  time.Time                      `json:"started_at"`
	FinishedAt time.Time                      `json:"finished_at"`
	DurationMS int64                          `json:"duration_ms"`
	Image      string                         `json:"image"`
	Topology   map[string]any                 `json:"topology"`
}

type bridgeBackendEvent struct {
	Schema     string                   `json:"schema"`
	Backend    bridgeBackend            `json:"backend"`
	Status     archiveformat.TestStatus `json:"status"`
	StartedAt  time.Time                `json:"started_at"`
	FinishedAt time.Time                `json:"finished_at"`
	DurationMS int64                    `json:"duration_ms"`
	Parameters map[string]any           `json:"parameters"`
	Metrics    map[string]any           `json:"metrics"`
	Message    *string                  `json:"message"`
}

type providerAPI string

const (
	providerAPIProvider  providerAPI = "provider"
	providerAPIContainer providerAPI = "container"
)

func (v *providerAPI) UnmarshalJSON(data []byte) error {
	return decodeStringEnum(data, v, "provider API", providerAPIProvider, providerAPIContainer)
}

type bridgeBackend string

const (
	bridgeBackendUDP  bridgeBackend = "udp"
	bridgeBackendRaw  bridgeBackend = "raw"
	bridgeBackendPCAP bridgeBackend = "pcap"
	bridgeBackendTAP  bridgeBackend = "tap"
)

func (v *bridgeBackend) UnmarshalJSON(data []byte) error {
	return decodeStringEnum(data, v, "bridge backend", bridgeBackendUDP, bridgeBackendRaw, bridgeBackendPCAP, bridgeBackendTAP)
}

func handleBridgeBackend(ctx context.Context, store *db.Store, record IngestionRecord, source EventSource, event bridgeBackendEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Printf("bridge-backend source=%s:%d run=%s backend=%s status=%s duration_ms=%d\n", source.Path, source.Line, record.RunID, event.Backend, event.Status, event.DurationMS)
	_ = store
	return nil
}

func handleLaptopIntegration(
	ctx context.Context, store *db.Store, record IngestionRecord,
	source EventSource, event laptopIntegrationEvent,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Printf("laptop-integration source=%s:%d run=%s runtime=%s status=%s duration_ms=%d\n",
		source.Path, source.Line, record.RunID, event.Runtime, event.Status, event.DurationMS)
	_ = store
	return nil
}

func handleProviderTest(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	event providerTestEvent,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Printf(
		"provider-test source=%s:%d run=%s provider=%s api=%s operation=%s status=%s duration_ms=%d\n",
		source.Path, source.Line, record.RunID, event.Provider, event.API,
		event.Operation, event.Status, event.DurationMS,
	)
	_ = store
	return nil
}

func ingestGoTestJSONL(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
	r io.Reader,
) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		if err := ctx.Err(); err != nil {
			return err
		}

		var event goTestEvent

		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf(
				"decode go-test event at line %d: %w",
				lineNumber,
				err,
			)
		}

		fmt.Printf(
			"go-test package=%s test=%s action=%s\n",
			event.Package,
			event.Test,
			event.Action,
		)

		// Store later.
		_ = store
		_ = record
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan go-test JSONL: %w", err)
	}

	return nil
}

func handleCrossHostLink(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	event crosshostlink.Event,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fmt.Printf(
		"cross-host-link source=%s:%d run=%s host=%s provider=%s status=%s\n",
		source.Path,
		source.Line,
		record.RunID,
		event.Host,
		event.Provider,
		event.Status,
	)

	_ = store

	return nil
}

func ingestProvidersJSONL(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
	r io.Reader,
) error {
	return ingestSchemaJSONL(ctx, store, record, file, r)
}

func ingestSummaryJSONL(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
	r io.Reader,
) error {
	return ingestSchemaJSONL(ctx, store, record, file, r)
}

func ingestRunnerJSON(ctx context.Context, store *db.Store, record IngestionRecord, file archiveformat.ArchiveFile, r io.Reader) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var runner archiveformat.Runner
	if err := decoder.Decode(&runner); err != nil {
		return fmt.Errorf("decode runner metadata: %w", err)
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return fmt.Errorf("decode runner metadata: %w", err)
	}
	fmt.Printf("runner-metadata source=%s run=%s device=%s runtime=%s\n", file.Path, record.RunID, runner.DeviceID, runner.ContainerRuntime)
	_ = ctx
	_ = store
	return nil
}

func ingestTextArtifact(ctx context.Context, store *db.Store, record IngestionRecord, file archiveformat.ArchiveFile, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bytesRead, err := io.Copy(io.Discard, r)
	if err != nil {
		return fmt.Errorf("read text artifact: %w", err)
	}
	fmt.Printf("text-artifact source=%s run=%s bytes=%d\n", file.Path, record.RunID, bytesRead)
	_ = store
	return nil
}

func ingestBinaryArtifact(ctx context.Context, store *db.Store, record IngestionRecord, file archiveformat.ArchiveFile, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bytesRead, err := io.Copy(io.Discard, r)
	if err != nil {
		return fmt.Errorf("read binary artifact: %w", err)
	}
	fmt.Printf("binary-artifact source=%s run=%s bytes=%d\n", file.Path, record.RunID, bytesRead)
	_ = store
	return nil
}

func ingestResults(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
) error {
	results, err := DecodeResults(bytes.NewReader(record.ResultsJSON))
	if err != nil {
		return err
	}
	if results.RunID != record.RunID {
		return fmt.Errorf("results run_id %q does not match archive run_id %q", results.RunID, record.RunID)
	}
	fmt.Printf("results source=%s run=%s tests=%d\n", file.Path, record.RunID, len(results.Tests))
	_ = ctx
	_ = store
	return nil
}
