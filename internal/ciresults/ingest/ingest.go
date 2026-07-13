package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/internal/ciresults/ingest/archiveformat"
)

func DecodeManifest(r io.Reader) (archiveformat.Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return archiveformat.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var manifest archiveformat.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return archiveformat.Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		return archiveformat.Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	return manifest, nil
}

func DecodeResults(r io.Reader) (archiveformat.ResultsDocument, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var results archiveformat.ResultsDocument

	if err := decoder.Decode(&results); err != nil {
		return archiveformat.ResultsDocument{}, fmt.Errorf(
			"decode results: %w",
			err,
		)
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		return archiveformat.ResultsDocument{}, fmt.Errorf(
			"decode results: %w",
			err,
		)
	}

	return results, nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any

	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}

	return fmt.Errorf("document contains multiple JSON values")
}

func IngestManifest(
	ctx context.Context,
	store *db.Store,
	manifestString string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	manifest, err := DecodeManifest(strings.NewReader(manifestString))
	if err != nil {
		return err
	}

	fmt.Printf(
		"ingesting manifest %s / %s / %s\n",
		manifest.RunID,
		manifest.Project.ID,
		manifest.Git.CommitSHA,
	)

	return nil
}

func IngestResults(
	ctx context.Context,
	store *db.Store,
	resultsString string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	results, err := DecodeResults(strings.NewReader(resultsString))
	if err != nil {
		return err
	}

	fmt.Printf(
		"ingesting result %s / %s / %s\n",
		results.RunID,
		results.Tests[0].Name,
		results.Tests[0].Suite,
	)

	return nil
}

func IngestRun(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	manifest, err := DecodeManifest(
		bytes.NewReader(record.ManifestJSON),
	)
	if err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}

	if store != nil {
		if err := persistRun(ctx, store, record, manifest); err != nil {
			return fmt.Errorf("persist run: %w", err)
		}
	}

	for _, file := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}

		switch file.Type {
		case archiveformat.FileTypeArtifact:
			if err := ingestArtifact(
				ctx,
				store,
				record,
				file,
			); err != nil {
				return err
			}
		case archiveformat.FileTypePacketCapture:
			if err := ingestArtifact(ctx, store, record, file); err != nil {
				return err
			}
		case archiveformat.FileTypeConfig:
			if err := ingestConfig(
				ctx,
				store,
				record,
				file,
			); err != nil {
				return err
			}
		case archiveformat.FileTypeLog:
			if err := ingestLog(
				ctx,
				store,
				record,
				file,
			); err != nil {
				return err
			}
		case archiveformat.FileTypeResults:
			if err := ingestResults(
				ctx,
				store,
				record,
				file,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func ingestArtifact(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := artifactPath(record.ArtifactsDir, file.Path)
	if err != nil {
		return fmt.Errorf("resolve artifact %q: %w", file.Path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact %q: %w", path, err)
	}
	defer f.Close()

	name := filepath.Base(file.Path)

	handler, ok := artifactHandlers[name]
	if !ok {
		fmt.Printf("skipping unsupported artifact %s\n", file.Path)
		return nil
	}

	if err := handler(ctx, store, record, file, f); err != nil {
		return fmt.Errorf("ingest artifact %q: %w", file.Path, err)
	}

	return nil
}

func artifactPath(artifactsDir, manifestPath string) (string, error) {
	path := filepath.Clean(filepath.FromSlash(manifestPath))

	if filepath.IsAbs(path) {
		return "", errors.New("absolute artifact path is not allowed")
	}

	prefix := "artifacts" + string(filepath.Separator)

	if after, ok := strings.CutPrefix(path, prefix); ok {
		path = after
	}

	if path == "" || path == "." {
		return "", errors.New("artifact path does not name a file")
	}

	if path == ".." ||
		strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes artifact directory")
	}

	return filepath.Join(artifactsDir, path), nil
}

type envelope struct {
	Schema string `json:"schema"`
}

type schemaHandler func(
	context.Context,
	*db.Store,
	IngestionRecord,
	EventSource,
	[]byte,
) error

var schemaHandlers = map[string]schemaHandler{
	"tethux.cross-host-link/v1":    decodeSchema(handleCrossHostLink),
	"tethux.provider-test/v1":      decodeSchema(handleProviderTest),
	"tethux.laptop-integration/v1": decodeSchema(handleLaptopIntegration),
	"tethux.bridge-backend/v1":     decodeSchema(handleBridgeBackend),
}

type structuredLogHandler func(
	context.Context,
	*db.Store,
	IngestionRecord,
	EventSource,
	[]byte,
) error

func dispatchStructuredLogRecord(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	data []byte,
) error {
	var env structuredEnvelope

	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode structured log envelope: %w", err)
	}

	switch {
	case env.Schema != "":
		return dispatchSchemaRecord(ctx, store, record, source, data)
	case env.Time != "" && env.Action != "" && env.Package != "":
		return decodeGoTestLogEvent(ctx, store, record, source, data)
	default:
		fmt.Printf(
			"skipping unknown structured JSON source=%s line=%d\n",
			source.Path,
			source.Line,
		)
		return nil
	}
}

type artifactHandler func(
	context.Context,
	*db.Store,
	IngestionRecord,
	archiveformat.ArchiveFile,
	io.Reader,
) error

var artifactHandlers = map[string]artifactHandler{
	"cross-link.jsonl":      ingestSchemaJSONL,
	"go-test.jsonl":         ingestGoTestJSONL,
	"providers.jsonl":       ingestProvidersJSONL,
	"summary.jsonl":         ingestSummaryJSONL,
	"bridge-backends.jsonl": ingestSchemaJSONL,
	"runner.json":           ingestRunnerJSON,
	"topology-docker.log":   ingestTextArtifact,
	"topology-podman.log":   ingestTextArtifact,
	"bridge-backends.pcap":  ingestBinaryArtifact,
}

func dispatchSchemaRecord(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	source EventSource,
	data []byte,
) error {
	var env envelope

	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode schema envelope: %w", err)
	}

	if env.Schema == "" {
		return errors.New("structured record is missing schema")
	}

	handler, ok := schemaHandlers[env.Schema]
	if !ok {
		return fmt.Errorf(
			"unsupported structured schema %q",
			env.Schema,
		)
	}

	return handler(ctx, store, record, source, data)
}

func ingestSchemaJSONL(
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

		source := EventSource{
			Kind: "artifact",
			Path: file.Path,
			Line: lineNumber,
		}

		if err := dispatchSchemaRecord(
			ctx,
			store,
			record,
			source,
			scanner.Bytes(),
		); err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan JSONL: %w", err)
	}

	return nil
}

type EventSource struct {
	Kind string
	Path string
	Line int
}

func decodeSchema[T any](
	fn func(
		context.Context,
		*db.Store,
		IngestionRecord,
		EventSource,
		T,
	) error,
) schemaHandler {
	return func(
		ctx context.Context,
		store *db.Store,
		record IngestionRecord,
		source EventSource,
		data []byte,
	) error {
		var value T

		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode %T: %w", value, err)
		}

		if err := ensureSingleJSONValue(decoder); err != nil {
			return fmt.Errorf("decode %T: %w", value, err)
		}

		return fn(ctx, store, record, source, value)
	}
}
