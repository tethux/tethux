-- name: GetViewerSummary :one
SELECT
    (
        SELECT
            COUNT(*)
        FROM
            runs
    ) AS run_count,
    COUNT(*) AS test_count,
    COUNT(*) FILTER (
        WHERE
            STATUS = 'passed'
    ) AS passed_count,
    COUNT(*) FILTER (
        WHERE
            STATUS != 'passed'
    ) AS failed_count
FROM
    test_results;

-- name: GetLatestTestStatusByDevice :many
WITH ranked AS (
    SELECT
        r.*,
        d.device_key,
        d.display_name,
        row_number() OVER (
            PARTITION BY r.project_id,
            r.device_id
            ORDER BY
                r.started_at DESC,
                r.id DESC
        ) AS position
    FROM
        runs r
        JOIN devices d ON d.id = r.device_id
    WHERE
        r.project_id = ?
        AND (
            ? IS NULL
            OR r.branch = ?
        )
)
SELECT
    *
FROM
    ranked
WHERE
    position = 1
ORDER BY
    device_key;

-- name: GetCurrentFailuresOnBranch :many
SELECT
    r.run_uid,
    r.commit_sha,
    r.started_at,
    d.device_key,
    tc.test_key,
    tc.name,
    tr.status,
    tr.duration_ms,
    tr.failure_kind,
    tr.failure_code,
    tr.message
FROM
    test_results tr
    JOIN runs r ON r.id = tr.run_id
    JOIN devices d ON d.id = r.device_id
    JOIN test_cases tc ON tc.id = tr.test_case_id
WHERE
    r.project_id = ?
    AND r.branch = ?
    AND tr.status IN ('failed', 'error')
ORDER BY
    r.started_at DESC,
    tc.test_key
LIMIT
    ?;

-- name: FilterRuns :many
SELECT
    r.id,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.tag,
    r.source_type,
    r.status,
    r.started_at,
    r.finished_at,
    r.duration_ms,
    r.total_count,
    r.passed_count,
    r.failed_count,
    r.skipped_count,
    r.errored_count,
    r.cancelled_count,
    p.project_key,
    p.name AS project_name,
    d.device_key,
    d.display_name AS device_name,
    d.last_os AS device_os,
    d.last_arch AS device_arch
FROM
    runs r
    JOIN projects p ON p.id = r.project_id
    JOIN devices d ON d.id = r.device_id
WHERE
    r.project_id = sqlc.arg(project_id)
    AND (
        sqlc.narg(branch) IS NULL
        OR r.branch = sqlc.narg(branch)
    )
    AND (
        sqlc.narg(device_id) IS NULL
        OR r.device_id = sqlc.narg(device_id)
    )
    AND (
        sqlc.narg(STATUS) IS NULL
        OR r.status = sqlc.narg(STATUS)
    )
    AND (
        sqlc.narg(source_type) IS NULL
        OR r.source_type = sqlc.narg(source_type)
    )
    AND (
        sqlc.narg(commit_prefix) IS NULL
        OR r.commit_sha LIKE sqlc.narg(commit_prefix) || '%'
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
    r.id DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListLatestRunsByDevice :many
WITH ranked_runs AS (
    SELECT
        r.*,
        ROW_NUMBER() OVER (
            PARTITION BY r.device_id
            ORDER BY
                r.started_at DESC,
                r.id DESC
        ) AS rank_number
    FROM
        runs r
    WHERE
        r.project_id = sqlc.arg(project_id)
        AND (
            sqlc.narg(branch) IS NULL
            OR r.branch = sqlc.narg(branch)
        )
)
SELECT
    rr.id,
    rr.run_uid,
    rr.commit_sha,
    rr.branch,
    rr.status,
    rr.started_at,
    rr.duration_ms,
    rr.total_count,
    rr.passed_count,
    rr.failed_count,
    rr.skipped_count,
    rr.errored_count,
    rr.cancelled_count,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name,
    d.last_os,
    d.last_arch
FROM
    ranked_runs rr
    JOIN devices d ON d.id = rr.device_id
WHERE
    rr.rank_number = 1
ORDER BY
    COALESCE(d.display_name, d.device_key);

-- name: ListCurrentFailures :many
WITH latest_runs AS (
    SELECT
        id
    FROM
        (
            SELECT
                r.id,
                ROW_NUMBER() OVER (
                    PARTITION BY r.device_id
                    ORDER BY
                        r.started_at DESC,
                        r.id DESC
                ) AS rank_number
            FROM
                runs r
            WHERE
                r.project_id = sqlc.arg(project_id)
                AND (
                    sqlc.narg(branch) IS NULL
                    OR r.branch = sqlc.narg(branch)
                )
        )
    WHERE
        rank_number = 1
)
SELECT
    tr.id AS result_id,
    tr.status,
    tr.duration_ms,
    tr.message,
    tr.failure_kind,
    tr.failure_code,
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name
FROM
    test_results tr
    JOIN latest_runs lr ON lr.id = tr.run_id
    JOIN runs r ON r.id = tr.run_id
    JOIN test_cases tc ON tc.id = tr.test_case_id
    JOIN devices d ON d.id = r.device_id
WHERE
    tr.status IN ('failed', 'error')
ORDER BY
    CASE
        tr.status
        WHEN 'error' THEN 0
        ELSE 1
    END,
    tc.test_key,
    d.device_key;

-- name: ListRunSummaryHistory :many
SELECT
    r.run_uid,
    r.commit_sha,
    r.branch,
    r.started_at,
    r.duration_ms,
    r.status,
    r.passed_count,
    r.failed_count,
    r.skipped_count,
    r.errored_count,
    r.cancelled_count,
    d.device_key,
    d.display_name AS device_name
FROM
    runs r
    JOIN devices d ON d.id = r.device_id
WHERE
    r.project_id = sqlc.arg(project_id)
    AND (
        sqlc.narg(branch) IS NULL
        OR r.branch = sqlc.narg(branch)
    )
    AND (
        sqlc.narg(device_id) IS NULL
        OR r.device_id = sqlc.narg(device_id)
    )
ORDER BY
    r.started_at DESC,
    r.id DESC
LIMIT
    sqlc.arg(result_limit);

-- name: ListSlowestResultsForRun :many
SELECT
    tr.id,
    tr.status,
    tr.duration_ms,
    tc.test_key,
    tc.name AS test_name,
    tc.suite
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
WHERE
    tr.run_id = sqlc.arg(run_id)
    AND tr.duration_ms IS NOT NULL
ORDER BY
    tr.duration_ms DESC
LIMIT
    sqlc.arg(result_limit);

-- name: ListMostFrequentFailures :many
SELECT
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    COUNT(*) AS failure_count,
    MAX(r.started_at) AS last_failed_at
FROM
    test_results tr
    JOIN test_cases tc ON tc.id = tr.test_case_id
    JOIN runs r ON r.id = tr.run_id
WHERE
    r.project_id = sqlc.arg(project_id)
    AND tr.status IN ('failed', 'error')
    AND (
        sqlc.narg(branch) IS NULL
        OR r.branch = sqlc.narg(branch)
    )
    AND (
        sqlc.narg(device_id) IS NULL
        OR r.device_id = sqlc.narg(device_id)
    )
    AND (
        sqlc.narg(started_from) IS NULL
        OR r.started_at >= sqlc.narg(started_from)
    )
GROUP BY
    tc.id,
    tc.test_key,
    tc.name,
    tc.suite
ORDER BY
    failure_count DESC,
    last_failed_at DESC
LIMIT
    sqlc.arg(result_limit);
