package viewer

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed all:frontend/build
var frontend embed.FS

func Serve(ctx context.Context, address, dbPath string) error {
	store, err := db.NewStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	assets, err := fs.Sub(frontend, "frontend/build")
	if err != nil {
		return fmt.Errorf("open embedded frontend: %w", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Recoverer)
	router.Get("/api/v1/summary", func(w http.ResponseWriter, r *http.Request) {
		summary, err := store.GetViewerSummary(r.Context())
		if err != nil {
			http.Error(w, "query results summary", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("encode summary", "error", err)
		}
	})
	router.Get("/api/v1/tests", func(w http.ResponseWriter, r *http.Request) {
		tests, err := store.ListViewerTests(r.Context())
		if err != nil {
			http.Error(w, "query tests", http.StatusInternalServerError)
			return
		}
		writeJSON(w, tests)
	})
	router.Get("/api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := store.ListViewerRuns(r.Context())
		if err != nil {
			http.Error(w, "query runs", http.StatusInternalServerError)
			return
		}
		writeJSON(w, runs)
	})
	router.Get("/api/v1/run/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		run, err := store.GetRunByUID(r.Context(), id)
		if err != nil {
			http.Error(w, "query run", http.StatusInternalServerError)
			return
		}
		results, err := store.ListResultsForRun(r.Context(), run.ID)
		if err != nil {
			http.Error(w, "query test results", http.StatusInternalServerError)
			return
		}
		files, err := store.ListArchiveFilesForRun(r.Context(), run.ID)
		if err != nil {
			http.Error(w, "query run files", http.StatusInternalServerError)
			return
		}
		writeJSON(w, struct {
			Run   any `json:"run"`
			Tests any `json:"tests"`
			Files any `json:"files"`
		}{run, results, files})
	})
	router.Get("/api/v1/file/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid file id", http.StatusBadRequest)
			return
		}
		file, err := store.GetArchiveFileByID(r.Context(), id)
		if err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}

		var manifest, software, environment string
		if queryErr := store.DB.QueryRowContext(r.Context(), `SELECT manifest_json, software_json, environment_json FROM runs WHERE id = ?`, file.RunID).Scan(&manifest, &software, &environment); queryErr != nil {
			http.Error(w, "query file data", http.StatusInternalServerError)
			return
		}
		content := any(nil)
		available := true
		switch file.ArchivePath {
		case "manifest.json":
			content = json.RawMessage(manifest)
		case "results.json":
			results, resultsErr := store.ListResultsForRun(r.Context(), file.RunID)
			err = resultsErr
			content = map[string]any{"schema_version": 1, "run_id": file.RunUid, "tests": results}
			if err != nil {
				http.Error(w, "query result data", http.StatusInternalServerError)
				return
			}
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
		writeJSON(w, map[string]any{"file": file, "available": available, "content": content})
	})
	router.Handle("/*", spaHandler(assets))

	server := &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		shutdownTimeoutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownTimeoutCtx)
	}()

	slog.Info("CI viewer listening", "address", "http://"+address, "database", dbPath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}

func spaHandler(assets fs.FS) http.Handler {
	index, indexErr := fs.ReadFile(assets, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested == "." {
			requested = "index.html"
		}
		content, err := fs.ReadFile(assets, requested)
		if err != nil {
			if indexErr != nil {
				http.Error(w, "frontend unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if strings.HasPrefix(requested, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, requested, time.Time{}, bytes.NewReader(content))
	})
}
