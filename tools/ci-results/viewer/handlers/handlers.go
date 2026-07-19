package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/0xveya/tethux/internal/ciresults/db"
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
	router.Get("/file/{id}", h.File)
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
		h.Logger.Info("HTTP request",
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
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.writeAPIError(w, "invalid file id", ErrCodeInvalidInput, http.StatusBadRequest, err)
		return
	}
	file, err := h.Store.GetArchiveFileByID(r.Context(), id)
	if err != nil {
		h.writeAPIError(w, "file not found", ErrCodeNotFound, http.StatusNotFound, err)
		return
	}

	var manifest, software, environment string
	if err := h.Store.DB.QueryRowContext(r.Context(), `SELECT manifest_json, software_json, environment_json FROM runs WHERE id = ?`, file.RunID).Scan(&manifest, &software, &environment); err != nil {
		h.writeAPIError(w, "query file data", ErrCodeQueryFailed, http.StatusInternalServerError, err)
		return
	}

	content := any(nil)
	available := true
	switch file.ArchivePath {
	case "manifest.json":
		content = json.RawMessage(manifest)
	case "results.json":
		results, err := h.Store.ListResultsForRun(r.Context(), file.RunID)
		if err != nil {
			h.writeAPIError(w, "query result data", ErrCodeQueryFailed, http.StatusInternalServerError, err)
			return
		}
		content = map[string]any{"schema_version": 1, "run_id": file.RunUid, "tests": results}
	default:
		available = false
		metadata := map[string]any{
			"message":    "This archive entry is indexed, but its bytes are not retained in SQLite.",
			"archive":    file.ArchiveRelativePath,
			"sha256":     file.Sha256,
			"media_type": file.MediaType,
			"size_bytes": file.SizeBytes,
		}
		if file.FileType == "config" {
			metadata["run_metadata"] = map[string]any{"software": json.RawMessage(software), "environment": json.RawMessage(environment)}
		}
		content = metadata
	}
	h.writeJSON(w, map[string]any{"file": file, "available": available, "content": content})
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
	fmt.Println(request.SQL)

	h.writeAPIError(w, "query execution is not implemented", ErrCodeNotImplemented, http.StatusNotImplemented, nil)
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
