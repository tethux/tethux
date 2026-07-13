-- name: CreateRun :one
INSERT INTO
    runs (
        run_uid,
        schema_version,
        archive_id,
        project_id,
        device_id,
        source_type,
        source_provider,
        workflow,
        job,
        trigger_name,
        source_attempt,
        commit_sha,
        branch,
        tag,
        git_dirty,
        commit_timestamp,
        started_at,
        finished_at,
        duration_ms,
        STATUS,
        total_count,
        passed_count,
        failed_count,
        skipped_count,
        errored_count,
        cancelled_count,
        software_json,
        environment_json,
        labels_json,
        manifest_json
    )
VALUES
    (
        sqlc.arg(run_uid),
        sqlc.arg(schema_version),
        sqlc.arg(archive_id),
        sqlc.arg(project_id),
        sqlc.arg(device_id),
        sqlc.narg(source_type),
        sqlc.narg(source_provider),
        sqlc.narg(workflow),
        sqlc.narg(job),
        sqlc.narg(trigger_name),
        sqlc.arg(source_attempt),
        sqlc.arg(commit_sha),
        sqlc.narg(branch),
        sqlc.narg(tag),
        sqlc.narg(git_dirty),
        sqlc.narg(commit_timestamp),
        sqlc.arg(started_at),
        sqlc.arg(finished_at),
        sqlc.arg(duration_ms),
        sqlc.arg(STATUS),
        sqlc.arg(total_count),
        sqlc.arg(passed_count),
        sqlc.arg(failed_count),
        sqlc.arg(skipped_count),
        sqlc.arg(errored_count),
        sqlc.arg(cancelled_count),
        sqlc.arg(software_json),
        sqlc.arg(environment_json),
        sqlc.arg(labels_json),
        sqlc.arg(manifest_json)
    )
RETURNING
    *;

-- Normally a duplicate run should make the import idempotently stop.
-- name: GetRunByUID :one
SELECT
    *
FROM
    runs
WHERE
    run_uid = sqlc.arg(run_uid)
LIMIT
    1;

-- name: GetRunByArchiveID :one
SELECT
    *
FROM
    runs
WHERE
    archive_id = sqlc.arg(archive_id)
LIMIT
    1;

-- name: RunUIDExists :one
SELECT
    EXISTS (
        SELECT
            1
        FROM
            runs
        WHERE
            run_uid = sqlc.arg(run_uid)
    ) AS exists_flag;

-- name: CountRuns :one
SELECT
    COUNT(*)
FROM
    runs;

-- name: GetLatestRunForProject :one
SELECT
    *
FROM
    runs
WHERE
    project_id = sqlc.arg(project_id)
ORDER BY
    started_at DESC,
    id DESC
LIMIT
    1;

-- name: GetLatestRunForProjectBranch :one
SELECT
    *
FROM
    runs
WHERE
    project_id = sqlc.arg(project_id)
    AND branch = sqlc.arg(branch)
ORDER BY
    started_at DESC,
    id DESC
LIMIT
    1;

-- name: GetLatestRunForDevice :one
SELECT
    *
FROM
    runs
WHERE
    project_id = sqlc.arg(project_id)
    AND device_id = sqlc.arg(device_id)
ORDER BY
    started_at DESC,
    id DESC
LIMIT
    1;

-- name: ListRuns :many
SELECT
    r.*,
    p.project_key,
    p.name AS project_name,
    d.device_key,
    d.display_name AS device_name
FROM
    runs r
    JOIN projects p ON p.id = r.project_id
    JOIN devices d ON d.id = r.device_id
ORDER BY
    r.started_at DESC,
    r.id DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListRunsForProject :many
SELECT
    r.*,
    d.device_key,
    d.display_name AS device_name
FROM
    runs r
    JOIN devices d ON d.id = r.device_id
WHERE
    r.project_id = sqlc.arg(project_id)
ORDER BY
    r.started_at DESC,
    r.id DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListRunsByCommit :many
SELECT
    r.*,
    d.device_key,
    d.display_name AS device_name
FROM
    runs r
    JOIN devices d ON d.id = r.device_id
WHERE
    r.project_id = sqlc.arg(project_id)
    AND r.commit_sha = sqlc.arg(commit_sha)
ORDER BY
    r.started_at DESC,
    r.id DESC;

-- name: ListRunsForDevice :many
SELECT
    *
FROM
    runs
WHERE
    project_id = sqlc.arg(project_id)
    AND device_id = sqlc.arg(device_id)
ORDER BY
    started_at DESC,
    id DESC
LIMIT
    sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);
