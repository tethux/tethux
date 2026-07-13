package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/internal/ciresults/ingest/archiveformat"
)

type executionConfig struct {
	Version  int         `json:"schema_version"`
	RunID    string      `json:"run_id"`
	Workflow Variant     `json:"workflow"`
	Commit   string      `json:"commit_sha"`
	Source   sourceType  `json:"source_type"`
	Trigger  triggerType `json:"trigger"`
}

type sourceType string

const (
	sourceCI        sourceType = "ci"
	sourceLocal     sourceType = "local"
	sourceScheduled sourceType = "scheduled"
	sourceManual    sourceType = "manual"
)

func (s *sourceType) UnmarshalJSON(data []byte) error {
	return decodeStringEnum(data, s, "source type", sourceCI, sourceLocal, sourceScheduled, sourceManual)
}

type triggerType string

const (
	triggerPush        triggerType = "push"
	triggerPullRequest triggerType = "pull_request"
	triggerSchedule    triggerType = "schedule"
	triggerManual      triggerType = "manual"
)

func (t *triggerType) UnmarshalJSON(data []byte) error {
	return decodeStringEnum(data, t, "trigger", triggerPush, triggerPullRequest, triggerSchedule, triggerManual)
}

func (c executionConfig) SchemaVersion() int {
	return c.Version
}

type topologyConfig struct {
	Version   int        `json:"schema_version"`
	Kind      string     `json:"kind"`
	Endpoints []Endpoint `json:"endpoints"`
	Transport Transport  `json:"transport"`
	Image     string     `json:"image"`
}

type Endpoint struct {
	Host     string                         `json:"host"`
	Provider archiveformat.ContainerRuntime `json:"provider"`
	Address  string                         `json:"address"`
}

type Transport struct {
	Type transportType `json:"type"`
	Port int           `json:"port"`
}

type transportType string

const (
	transportUDP transportType = "udp"
	transportTCP transportType = "tcp"
)

func (t *transportType) UnmarshalJSON(data []byte) error {
	return decodeStringEnum(data, t, "transport type", transportUDP, transportTCP)
}

func decodeStringEnum[T ~string](data []byte, target *T, name string, allowed ...T) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s must be a string: %w", name, err)
	}
	for _, candidate := range allowed {
		if value == string(candidate) {
			*target = candidate
			return nil
		}
	}
	return fmt.Errorf("unsupported %s %q", name, value)
}

func (c topologyConfig) SchemaVersion() int {
	return c.Version
}

func configPath(configDir, manifestPath string) (string, error) {
	path := filepath.Clean(filepath.FromSlash(manifestPath))

	if filepath.IsAbs(path) {
		return "", errors.New("absolute artifact path is not allowed")
	}

	prefix := "configs" + string(filepath.Separator)

	if after, ok := strings.CutPrefix(path, prefix); ok {
		path = after
	}

	if path == "" || path == "." {
		return "", errors.New("config path does not name a file")
	}

	if path == ".." ||
		strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", errors.New("config path escapes test directory")
	}

	return filepath.Join(configDir, path), nil
}

func ingestConfig(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := configPath(record.ConfigsDir, file.Path)
	if err != nil {
		return fmt.Errorf("resolve config %q: %w", file.Path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %q: %w", path, err)
	}

	name := filepath.Base(file.Path)

	handler, ok := configHandlers[name]
	if !ok {
		fmt.Printf("skipping unsupported config %s\n", file.Path)
		return nil
	}

	if err := handler(ctx, store, record, data); err != nil {
		return fmt.Errorf("ingest config %q: %w", file.Path, err)
	}

	return nil
}

type configHandler func(
	context.Context,
	*db.Store,
	IngestionRecord,
	[]byte,
) error

var configHandlers = map[string]configHandler{
	"execution.json": decodeConfig(1, handleExecutionConfig),
	"topology.json":  decodeConfig(1, handleTopologyConfig),
}

type versionedConfig interface {
	SchemaVersion() int
}

func decodeConfig[T versionedConfig](
	expectedVersion int,
	fn func(
		context.Context,
		*db.Store,
		IngestionRecord,
		T,
	) error,
) configHandler {
	return func(
		ctx context.Context,
		store *db.Store,
		record IngestionRecord,
		data []byte,
	) error {
		var config T

		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&config); err != nil {
			return fmt.Errorf("decode %T: %w", config, err)
		}

		if err := ensureSingleJSONValue(decoder); err != nil {
			return fmt.Errorf("decode %T: %w", config, err)
		}

		if config.SchemaVersion() != expectedVersion {
			return fmt.Errorf(
				"unsupported %T schema version %d; expected %d",
				config,
				config.SchemaVersion(),
				expectedVersion,
			)
		}

		return fn(ctx, store, record, config)
	}
}

func handleExecutionConfig(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	config executionConfig,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if config.RunID != record.RunID {
		return fmt.Errorf(
			"execution config run_id %q does not match archive run_id %q",
			config.RunID,
			record.RunID,
		)
	}

	fmt.Printf(
		"execution config run=%s variant=%s workflow=%s commit=%s source=%s trigger=%s\n",
		record.RunID,
		record.Variant,
		config.Workflow,
		config.Commit,
		config.Source,
		config.Trigger,
	)

	_ = store

	return nil
}

func handleTopologyConfig(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	config topologyConfig,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Printf(
		"topology config run=%s variant=%s kind=%s image=%s\n",
		record.RunID,
		record.Variant,
		config.Kind,
		config.Image,
	)
	_ = store
	return nil
}
