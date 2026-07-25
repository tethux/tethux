package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/0xveya/tethux/internal/ciresults/db"
	dbgen "github.com/0xveya/tethux/internal/ciresults/db/sqlc"
	"github.com/0xveya/tethux/tools/ci-results/viewer/handlers/types"
)

type Handlers struct {
	Store  *db.Store
	Logger *slog.Logger
}

func New(store *db.Store, logger *slog.Logger) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{Store: store, Logger: logger}
}

func (h *Handlers) Routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/summary", h.Summary)
	router.Get("/tests", h.Tests)
	router.Get("/runs", h.Runs)
	router.Get("/run/{id}", h.Run)
	router.Get("/artifacts", h.Artifacts)
	router.Get("/file/{id}", h.File)
	router.Get("/file/{id}/raw", h.RawFile)
	router.Post("/query/execute", h.ExecuteQuery)
	router.Get("/schema", h.Schema)
	router.Get("/schema/info", h.SchemaInfo)
	return router
}

func (h *Handlers) LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		h.Logger.Info(
			"HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", writer.status,
			"duration", time.Since(started),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.Store.GetViewerSummary(r.Context())
	if err != nil {
		h.writeAPIError(w, "query results summary", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, summary)
}

func (h *Handlers) Tests(w http.ResponseWriter, r *http.Request) {
	tests, err := h.Store.ListViewerTests(r.Context())
	if err != nil {
		h.writeAPIError(w, "query tests", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, tests)
}

func (h *Handlers) Runs(w http.ResponseWriter, r *http.Request) {
	runs, err := h.Store.ListViewerRuns(r.Context())
	if err != nil {
		h.writeAPIError(w, "query runs", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, runs)
}

func (h *Handlers) Run(w http.ResponseWriter, r *http.Request) {
	run, err := h.Store.GetRunByUID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.writeAPIError(w, "query run", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	results, err := h.Store.ListResultsForRun(r.Context(), run.ID)
	if err != nil {
		h.writeAPIError(w, "query test results", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	files, err := h.Store.ListArchiveFilesForRun(r.Context(), run.ID)
	if err != nil {
		h.writeAPIError(w, "query run files", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, struct {
		Run   any `json:"run"`
		Tests any `json:"tests"`
		Files any `json:"files"`
	}{run, results, files})
}

func (h *Handlers) File(w http.ResponseWriter, r *http.Request) {
	file, err := h.loadFile(r)
	if err != nil {
		h.writeFileError(w, err)
		return
	}
	content, available, synthesized, err := h.fileBytes(r.Context(), file)
	if err != nil {
		h.writeAPIError(w, "load file content", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	const previewLimit = 256 << 10
	truncated := len(content) > previewLimit
	if truncated {
		content = content[:previewLimit]
	}
	var preview any
	if available && isTextMediaType(file.MediaType) {
		if strings.Contains(file.MediaType, "json") {
			if json.Unmarshal(content, &preview) != nil {
				preview = string(content)
			}
		} else {
			preview = string(content)
		}
	}
	h.writeJSON(w, map[string]any{
		"file":        fileMetadata(file),
		"available":   available,
		"synthesized": synthesized,
		"truncated":   truncated,
		"preview":     preview,
		"raw_url":     fmt.Sprintf("/api/v1/file/%d/raw", file.ID),
	})
}

func (h *Handlers) Artifacts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	before := int64(math.MaxInt64)
	if value := query.Get("cursor"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 {
			h.writeAPIError(w, "invalid artifact cursor", ErrCodeInvalidInput, http.StatusBadRequest, err)
			return
		}
		before = parsed
	}
	limit := int64(50)
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 1 || parsed > 200 {
			h.writeAPIError(w, "artifact limit must be between 1 and 200", ErrCodeInvalidInput, http.StatusBadRequest, err)
			return
		}
		limit = parsed
	}
	publicFilter, err := triStateFilter(query.Get("visibility"), "public", "private")
	if err != nil {
		h.writeAPIError(w, err.Error(), ErrCodeInvalidInput, http.StatusBadRequest, err)
		return
	}
	availableFilter, err := triStateFilter(query.Get("availability"), "available", "unavailable")
	if err != nil {
		h.writeAPIError(w, err.Error(), ErrCodeInvalidInput, http.StatusBadRequest, err)
		return
	}
	rows, err := h.Store.ListArtifactFiles(r.Context(), dbgen.ListArtifactFilesParams{
		BeforeID:        before,
		SearchText:      strings.TrimSpace(query.Get("q")),
		FileTypeFilter:  query.Get("type"),
		MediaTypeFilter: query.Get("media"),
		WorkflowFilter:  query.Get("workflow"),
		RunFilter:       query.Get("run"),
		PublicFilter:    publicFilter,
		AvailableFilter: availableFilter,
		ResultLimit:     limit + 1,
	})
	if err != nil {
		h.writeAPIError(w, "query artifacts", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	var next string
	if int64(len(rows)) > limit {
		next = strconv.FormatInt(rows[limit-1].ID, 10)
		rows = rows[:limit]
	}
	h.writeJSON(w, map[string]any{"items": rows, "next_cursor": next})
}

func (h *Handlers) RawFile(w http.ResponseWriter, r *http.Request) {
	file, err := h.loadFile(r)
	if err != nil {
		h.writeFileError(w, err)
		return
	}
	content, available, _, err := h.fileBytes(r.Context(), file)
	if err != nil {
		h.writeAPIError(w, "load file content", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	if !available {
		h.writeAPIError(w, "artifact bytes are unavailable; re-ingest the source archive", ErrCodeNotFound, http.StatusNotFound, nil)
		return
	}
	etag := `"` + file.Sha256 + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	filename := filepath.Base(file.ArchivePath)
	disposition := "attachment"
	if isTextMediaType(file.MediaType) || strings.HasPrefix(file.MediaType, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", file.MediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename=%q`, disposition, filename))
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(content))
}

var errInvalidFileID = errors.New("invalid file id")

func (h *Handlers) loadFile(r *http.Request) (dbgen.GetArchiveFileContentByIDRow, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return dbgen.GetArchiveFileContentByIDRow{}, errInvalidFileID
	}
	return h.Store.GetArchiveFileContentByID(r.Context(), id)
}

func (h *Handlers) writeFileError(w http.ResponseWriter, err error) {
	if errors.Is(err, errInvalidFileID) {
		h.writeAPIError(w, "invalid file id", ErrCodeInvalidInput, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		h.writeAPIError(w, "file not found", ErrCodeNotFound, http.StatusNotFound, err)
		return
	}
	h.writeAPIError(w, "query file", ErrCodeQueryFailed, http.StatusInternalServerError, err)
}

func (h *Handlers) fileBytes(ctx context.Context, file dbgen.GetArchiveFileContentByIDRow) ([]byte, bool, bool, error) {
	if file.ContentAvailable == 1 {
		return file.Content, true, false, nil
	}
	switch file.ArchivePath {
	case "manifest.json":
		var value string
		err := h.Store.DB.QueryRowContext(ctx, `SELECT manifest_json FROM runs WHERE id = ?`, file.RunID).Scan(&value)
		return []byte(value), err == nil, true, err
	case "results.json":
		results, err := h.Store.ListResultsForRun(ctx, file.RunID)
		if err != nil {
			return nil, false, false, err
		}
		value, err := json.Marshal(map[string]any{"schema_version": 1, "run_id": file.RunUid, "tests": results})
		return value, err == nil, true, err
	default:
		return nil, false, false, nil
	}
}

func fileMetadata(file dbgen.GetArchiveFileContentByIDRow) map[string]any {
	return map[string]any{
		"id": file.ID, "run_id": file.RunID, "run_uid": file.RunUid,
		"archive_path": file.ArchivePath, "file_type": file.FileType,
		"media_type": file.MediaType, "size_bytes": file.SizeBytes,
		"sha256": file.Sha256, "is_public": file.IsPublic,
		"content_available": file.ContentAvailable, "content_error": file.ContentError,
		"workflow": file.Workflow, "commit_sha": file.CommitSha, "run_status": file.RunStatus,
	}
}

func isTextMediaType(mediaType string) bool {
	parsed, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		parsed = mediaType
	}
	return strings.HasPrefix(parsed, "text/") ||
		strings.Contains(parsed, "json") ||
		strings.Contains(parsed, "yaml") ||
		strings.Contains(parsed, "xml") ||
		strings.Contains(parsed, "javascript")
}

func triStateFilter(value, trueName, falseName string) (int64, error) {
	switch value {
	case "", "all":
		return -1, nil
	case trueName:
		return 1, nil
	case falseName:
		return 0, nil
	default:
		return 0, fmt.Errorf("invalid filter %q", value)
	}
}

func (h *Handlers) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	var request types.ExecuteQueryRequest
	if !DecodeJSON(w, r, &request) {
		return
	}
	if request.SQL == "" {
		h.writeAPIError(w, "sql is required", ErrCodeInvalidInput, http.StatusBadRequest, nil)
		return
	}
	sql := strings.TrimSpace(request.SQL)
	if !strings.HasSuffix(sql, ";") {
		sql += ";"
	}
	res, err := executeQuery(r.Context(), h.Store, sql)
	if err != nil {
		h.writeAPIError(w, "query execution failed", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, res)
}

func (h *Handlers) writeJSON(w http.ResponseWriter, data any) {
	if err := WriteJSON(w, data); err != nil {
		h.Logger.Error("write API response", "error", err)
	}
}

func (h *Handlers) writeAPIError(w http.ResponseWriter, message, code string, status int, err error) {
	if err != nil {
		h.Logger.Error(message, "error", err, "status", status, "code", code)
		WriteAPIError(w, message, code, err.Error(), status)
		return
	}
	h.Logger.Warn(message, "status", status, "code", code)
	WriteAPIError(w, message, code, "", status)
}

func (h *Handlers) Schema(w http.ResponseWriter, r *http.Request) {
	schema, err := h.Store.GetSchema(r.Context())
	if err != nil {
		h.writeAPIError(w, "query schema", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, schema)
}

func (h *Handlers) SchemaInfo(w http.ResponseWriter, r *http.Request) {
	schema, err := h.Store.GetSchemaInfo(r.Context())
	if err != nil {
		h.writeAPIError(w, "query schema", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}
	h.writeJSON(w, schema)
}

func executeQuery(
	ctx context.Context,
	store *db.Store,
	query string,
) (*types.ExecuteQueryResponse, error) {
	conn, err := store.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, execErr := conn.ExecContext(ctx, "pragma query_only = on"); execErr != nil {
		return nil, execErr
	}

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	columns := make([]types.QueryColumn, len(columnNames))
	for i, name := range columnNames {
		columns[i] = types.QueryColumn{
			Name: name,
			Type: columnTypes[i].DatabaseTypeName(),
		}
	}

	resultRows := make([]map[string]any, 0)

	for rows.Next() {
		values := make([]any, len(columnNames))
		destinations := make([]any, len(columnNames))

		for i := range values {
			destinations[i] = &values[i]
		}

		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columnNames))

		for i, name := range columnNames {
			value := values[i]

			if bytes, ok := value.([]byte); ok {
				value = string(bytes)
			}

			row[name] = value
		}

		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &types.ExecuteQueryResponse{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
	}, nil
}
