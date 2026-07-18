package viewer

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/0xveya/tethux/internal/ciresults/db"
	"github.com/0xveya/tethux/tools/ci-results/viewer/handlers"
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

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	api := handlers.New(store, logger)
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Recoverer)
	router.Mount("/api/v1", api.LogRequests(api.Routes()))
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

	logger.Info("CI viewer listening", "address", "http://"+address, "database", dbPath)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
