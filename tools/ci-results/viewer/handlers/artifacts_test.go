package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xveya/tethux/internal/ciresults/db"
)

func TestArtifactListPreviewAndRaw(t *testing.T) {
	store, fileID, content := artifactTestStore(t)
	handler := New(store, nil).Routes()

	listRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/artifacts?type=log&availability=available&visibility=private", http.NoBody)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `runner.log`) {
		t.Fatalf("list response code=%d body=%s", listResponse.Code, listResponse.Body.String())
	}

	previewRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/file/%d", fileID), http.NoBody)
	previewResponse := httptest.NewRecorder()
	handler.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), `"available":true`) {
		t.Fatalf("preview response code=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}

	rawPath := fmt.Sprintf("/file/%d/raw", fileID)
	rawRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, rawPath, http.NoBody)
	rawResponse := httptest.NewRecorder()
	handler.ServeHTTP(rawResponse, rawRequest)
	if rawResponse.Code != http.StatusOK || rawResponse.Body.String() != string(content) {
		t.Fatalf("raw response code=%d body=%q", rawResponse.Code, rawResponse.Body.String())
	}
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(content))
	if rawResponse.Header().Get("ETag") != etag {
		t.Fatalf("etag=%q want=%q", rawResponse.Header().Get("ETag"), etag)
	}

	cachedRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, rawPath, http.NoBody)
	cachedRequest.Header.Set("If-None-Match", etag)
	cachedResponse := httptest.NewRecorder()
	handler.ServeHTTP(cachedResponse, cachedRequest)
	if cachedResponse.Code != http.StatusNotModified {
		t.Fatalf("conditional response code=%d", cachedResponse.Code)
	}
}

func artifactTestStore(t *testing.T) (*db.Store, int64, []byte) {
	t.Helper()
	store, err := db.NewStore(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO projects(id, project_key, name) VALUES(1, 'tethux', 'tethux')`)
	mustExec(`INSERT INTO devices(id, device_key) VALUES(1, 'test-runner')`)
	mustExec(`INSERT INTO archives(id, relative_path, file_size_bytes, import_status) VALUES(1, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/normal/test.tar.zst', 1, 'imported')`)
	mustExec(`
		INSERT INTO runs(
			id, run_uid, schema_version, archive_id, project_id, device_id,
			source_type, source_provider, workflow, job, source_attempt,
			commit_sha, started_at, finished_at, duration_ms, status,
			total_count, passed_count, failed_count, skipped_count, errored_count,
			software_json, environment_json, labels_json, manifest_json
		) VALUES(
			1, '018f0000-0000-7000-8000-000000000001', 1, 1, 1, 1,
			'ci', 'woodpecker', 'normal', 'normal', 1,
			'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
			'2026-06-06T12:00:00Z', '2026-06-06T12:00:01Z', 1000, 'passed',
			0, 0, 0, 0, 0, '{}', '{}', '{}', '{}'
		)
	`)
	content := []byte("hello artifact\n")
	sum := fmt.Sprintf("%x", sha256.Sum256(content))
	result, err := store.DB.ExecContext(ctx, `
		INSERT INTO archive_files(
			run_id, archive_path, file_type, media_type, size_bytes, sha256,
			is_public, content, content_available
		) VALUES(1, 'logs/runner.log', 'log', 'text/plain', ?, ?, 0, ?, 1)
	`, len(content), sum, content)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return store, id, content
}
