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

func ingestLog(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := logPath(record.LogsDir, file.Path)
	if err != nil {
		return fmt.Errorf("resolve log %q: %w", file.Path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log %q: %w", path, err)
	}
	defer f.Close()

	name := filepath.Base(file.Path)

	handler, ok := logHandlers[name]
	if !ok {
		fmt.Printf("skipping unsupported log %s\n", file.Path)
		return nil
	}

	if err := handler(ctx, store, record, file, f); err != nil {
		return fmt.Errorf("ingest log %q: %w", file.Path, err)
	}

	return nil
}

func logPath(logsDir, manifestPath string) (string, error) {
	path := filepath.Clean(filepath.FromSlash(manifestPath))

	if filepath.IsAbs(path) {
		return "", errors.New("absolute log path is not allowed")
	}

	prefix := "logs" + string(filepath.Separator)

	if after, ok := strings.CutPrefix(path, prefix); ok {
		path = after
	}

	if path == "" || path == "." {
		return "", errors.New("log path does not name a file")
	}

	if path == ".." ||
		strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", errors.New("log path escapes logs directory")
	}

	return filepath.Join(logsDir, path), nil
}

type logHandler func(
	context.Context,
	*db.Store,
	IngestionRecord,
	archiveformat.ArchiveFile,
	io.Reader,
) error

var logHandlers = map[string]logHandler{
	"runner.log":   ingestStructuredLog,
	"topology.log": ingestStructuredLog,
}

func ingestStructuredLog(
	ctx context.Context,
	store *db.Store,
	record IngestionRecord,
	file archiveformat.ArchiveFile,
	r io.Reader,
) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		if err := ctx.Err(); err != nil {
			return err
		}

		data, structured, err := extractStructuredLogRecord(
			scanner.Bytes(),
		)
		if err != nil {
			return fmt.Errorf(
				"%s line %d: %w",
				file.Path,
				lineNumber,
				err,
			)
		}

		if !structured {
			continue
		}

		source := EventSource{
			Kind: "log",
			Path: file.Path,
			Line: lineNumber,
		}

		if err := dispatchStructuredLogRecord(
			ctx,
			store,
			record,
			source,
			data,
		); err != nil {
			return fmt.Errorf(
				"%s line %d: %w",
				file.Path,
				lineNumber,
				err,
			)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan structured log %q: %w", file.Path, err)
	}

	return nil
}

func extractStructuredLogRecord(
	line []byte,
) ([]byte, bool, error) {
	start := bytes.IndexByte(line, '{')
	if start < 0 {
		return nil, false, nil
	}

	data := bytes.TrimSpace(line[start:])
	if len(data) == 0 {
		return nil, false, nil
	}

	if !json.Valid(data) {
		return nil, false, nil
	}

	return data, true, nil
}

type structuredEnvelope struct {
	Schema  string `json:"schema"`
	Time    string `json:"Time"`
	Action  string `json:"Action"`
	Package string `json:"Package"`
}
