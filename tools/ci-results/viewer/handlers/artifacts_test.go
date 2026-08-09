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

	"github.com/tethux/tethux/internal/ciresults/db"
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

	searchRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/file/%d/search?q=socket&severity=error", fileID), http.NoBody)
	searchResponse := httptest.NewRecorder()
	handler.ServeHTTP(searchResponse, searchRequest)
	if searchResponse.Code != http.StatusOK ||
		!strings.Contains(searchResponse.Body.String(), `"line":2`) ||
		!strings.Contains(searchResponse.Body.String(), `"severity":"error"`) {
		t.Fatalf("search response code=%d body=%s", searchResponse.Code, searchResponse.Body.String())
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

func TestExecuteQueryCapsUnboundedResults(t *testing.T) {
	store, err := db.NewStore(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	result, err := executeQuery(context.Background(), store, `
		WITH RECURSIVE numbers(value) AS (
			SELECT 1 UNION ALL SELECT value + 1 FROM numbers WHERE value < 750
		) SELECT value FROM numbers;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 500 || result.RowCount != 500 || !result.Truncated {
		t.Fatalf("rows=%d count=%d truncated=%t", len(result.Rows), result.RowCount, result.Truncated)
	}
}

func TestExecuteQueryDoesNotReturnArtifactSizedCells(t *testing.T) {
	store, err := db.NewStore(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	result, err := executeQuery(
		context.Background(),
		store,
		`SELECT randomblob(1048576) AS content, printf('%.*c', 20000, 'x') AS long_text;`,
	)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := result.Rows[0]["content"].(string)
	longText, _ := result.Rows[0]["long_text"].(string)
	if content != "<BLOB 1048576 bytes — use the artifact viewer to inspect or download>" {
		t.Fatalf("content descriptor = %q", content)
	}
	if len(longText) > maxQueryTextBytes+100 || !strings.Contains(longText, "truncated from 20000 bytes") {
		t.Fatalf("long text was not bounded: length=%d suffix=%q", len(longText), longText[len(longText)-40:])
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
	content := []byte("hello artifact\nERROR socket connection failed\nINFO retry scheduled\n")
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
