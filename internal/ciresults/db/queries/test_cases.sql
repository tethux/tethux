-- name: UpsertTestCase :one
INSERT INTO
    test_cases (
        project_id,
        test_key,
        name,
        suite,
        result_kind,
        source_file,
        source_symbol,
        first_seen_at,
        last_seen_at
    )
VALUES
    (
        sqlc.arg(project_id),
        sqlc.arg(test_key),
        sqlc.arg(name),
        sqlc.narg(suite),
        sqlc.arg(result_kind),
        sqlc.narg(source_file),
        sqlc.narg(source_symbol),
        sqlc.narg(first_seen_at),
        sqlc.narg(last_seen_at)
    ) ON CONFLICT(project_id, test_key) DO
UPDATE
SET
    name = excluded.name,
    suite = excluded.suite,
    source_file = excluded.source_file,
    source_symbol = excluded.source_symbol,
    last_seen_at = excluded.last_seen_at
RETURNING
    *;

-- name: CreateTestResult :one
INSERT INTO
    test_results (
        run_id,
        test_case_id,
        attempt,
        STATUS,
        started_at,
        finished_at,
        duration_ms,
        message,
        failure_kind,
        failure_phase,
        failure_code,
        expected_value,
        actual_value,
        stack_trace,
        parameters_json,
        metrics_json,
        labels_json,
        details_json
    )
VALUES
    (
        sqlc.arg(run_id),
        sqlc.arg(test_case_id),
        sqlc.arg(attempt),
        sqlc.arg(STATUS),
        sqlc.narg(started_at),
        sqlc.narg(finished_at),
        sqlc.narg(duration_ms),
        sqlc.narg(message),
        sqlc.narg(failure_kind),
        sqlc.narg(failure_phase),
        sqlc.narg(failure_code),
        sqlc.narg(expected_value),
        sqlc.narg(actual_value),
        sqlc.narg(stack_trace),
        sqlc.arg(parameters_json),
        sqlc.arg(metrics_json),
        sqlc.arg(labels_json),
        sqlc.arg(details_json)
    )
RETURNING
    *;

-- Useful if an importer retry can encounter already inserted rows
-- within an otherwise incomplete run.
-- name: UpsertTestResult :one
INSERT INTO
    test_results (
        run_id,
        test_case_id,
        attempt,
        STATUS,
        started_at,
        finished_at,
        duration_ms,
        message,
        failure_kind,
        failure_phase,
        failure_code,
        expected_value,
        actual_value,
        stack_trace,
        parameters_json,
        metrics_json,
        labels_json,
        details_json
    )
VALUES
    (
        sqlc.arg(run_id),
        sqlc.arg(test_case_id),
        sqlc.arg(attempt),
        sqlc.arg(STATUS),
        sqlc.narg(started_at),
        sqlc.narg(finished_at),
        sqlc.narg(duration_ms),
        sqlc.narg(message),
        sqlc.narg(failure_kind),
        sqlc.narg(failure_phase),
        sqlc.narg(failure_code),
        sqlc.narg(expected_value),
        sqlc.narg(actual_value),
        sqlc.narg(stack_trace),
        sqlc.arg(parameters_json),
        sqlc.arg(metrics_json),
        sqlc.arg(labels_json),
        sqlc.arg(details_json)
    ) ON CONFLICT(run_id, test_case_id, attempt) DO
UPDATE
SET
    STATUS = excluded.status,
    started_at = excluded.started_at,
    finished_at = excluded.finished_at,
    duration_ms = excluded.duration_ms,
    message = excluded.message,
    failure_kind = excluded.failure_kind,
    failure_phase = excluded.failure_phase,
    failure_code = excluded.failure_code,
    expected_value = excluded.expected_value,
    actual_value = excluded.actual_value,
    stack_trace = excluded.stack_trace,
    parameters_json = excluded.parameters_json,
    metrics_json = excluded.metrics_json,
    labels_json = excluded.labels_json,
    details_json = excluded.details_json
RETURNING
    *;

-- name: GetTestResultByID :one
SELECT
    tr.*,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.result_kind,
    tc.source_file,
    tc.source_symbol
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
WHERE
    tr.id = sqlc.arg(id)
LIMIT
    1;

-- name: ListResultsForRun :many
SELECT
    tr.*,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.result_kind,
    tc.source_file,
    tc.source_symbol
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
WHERE
    tr.run_id = sqlc.arg(run_id)
ORDER BY
    CASE
        tr.status
        WHEN 'error' THEN 0
        WHEN 'failed' THEN 1
        WHEN 'cancelled' THEN 2
        WHEN 'skipped' THEN 3
        WHEN 'passed' THEN 4
        ELSE 5
    END,
    tc.test_key,
    tr.attempt;

-- name: ListFailuresForRun :many
SELECT
    tr.*,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.source_file,
    tc.source_symbol
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
WHERE
    tr.run_id = sqlc.arg(run_id)
    AND tr.status IN ('failed', 'error')
ORDER BY
    CASE
        tr.status
        WHEN 'error' THEN 0
        ELSE 1
    END,
    tc.test_key,
    tr.attempt;

-- name: CountResultsForRun :one
SELECT
    COUNT(*) AS total_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'passed'
    ) AS passed_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'failed'
    ) AS failed_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'skipped'
    ) AS skipped_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'error'
    ) AS errored_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'cancelled'
    ) AS cancelled_count
FROM
    test_results
WHERE
    run_id = sqlc.arg(run_id);

-- name: ListTestHistory :many
SELECT
    tr.id,
    tr.status,
    tr.attempt,
    tr.duration_ms,
    tr.message,
    tr.failure_kind,
    tr.failure_code,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at,
    r.source_type,
    d.device_key,
    d.display_name AS device_name
FROM
    test_results tr
    JOIN runs r ON r.id = tr.run_id
    JOIN devices d ON d.id = r.device_id
WHERE
    tr.test_case_id = sqlc.arg(test_case_id)
ORDER BY
    r.started_at DESC,
    r.id DESC,
    tr.attempt DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListTestHistoryForDevice :many
SELECT
    tr.id,
    tr.status,
    tr.attempt,
    tr.duration_ms,
    tr.message,
    tr.failure_kind,
    tr.failure_code,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at
FROM
    test_results tr
    JOIN runs r ON r.id = tr.run_id
WHERE
    tr.test_case_id = sqlc.arg(test_case_id)
    AND r.device_id = sqlc.arg(device_id)
ORDER BY
    r.started_at DESC,
    r.id DESC,
    tr.attempt DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: GetLatestResultForTestAndDevice :one
SELECT
    tr.*,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at
FROM
    test_results tr
    JOIN runs r ON r.id = tr.run_id
WHERE
    tr.test_case_id = sqlc.arg(test_case_id)
    AND r.device_id = sqlc.arg(device_id)
ORDER BY
    r.started_at DESC,
    r.id DESC,
    tr.attempt DESC
LIMIT
    1;

-- name: FilterTestResults :many
SELECT
    tr.id,
    tr.status,
    tr.attempt,
    tr.duration_ms,
    tr.message,
    tr.failure_kind,
    tr.failure_code,
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.result_kind,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at,
    r.source_type,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
    JOIN runs r ON r.id = tr.run_id
    JOIN devices d ON d.id = r.device_id
WHERE
    r.project_id = sqlc.arg(project_id)
    AND (
        sqlc.narg(STATUS) IS NULL
        OR tr.status = sqlc.narg(STATUS)
    )
    AND (
        sqlc.narg(device_id) IS NULL
        OR r.device_id = sqlc.narg(device_id)
    )
    AND (
        sqlc.narg(branch) IS NULL
        OR r.branch = sqlc.narg(branch)
    )
    AND (
        sqlc.narg(test_search) IS NULL
        OR tc.test_key LIKE '%' || sqlc.narg(test_search) || '%'
        OR tc.name LIKE '%' || sqlc.narg(test_search) || '%'
    )
    AND (
        sqlc.narg(suite) IS NULL
        OR tc.suite = sqlc.narg(suite)
    )
    AND (
        sqlc.narg(commit_prefix) IS NULL
        OR r.commit_sha LIKE sqlc.narg(commit_prefix) || '%'
    )
    AND (
        sqlc.narg(min_duration_ms) IS NULL
        OR tr.duration_ms >= sqlc.narg(min_duration_ms)
    )
    AND (
        sqlc.narg(started_from) IS NULL
        OR r.started_at >= sqlc.narg(started_from)
    )
    AND (
        sqlc.narg(started_until) IS NULL
        OR r.started_at < sqlc.narg(started_until)
    )
ORDER BY
    r.started_at DESC,
    tr.id DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);
