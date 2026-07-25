-- name: CreateArchiveFile :one
INSERT INTO
    archive_files (
        run_id,
        archive_path,
        file_type,
        media_type,
        size_bytes,
        sha256,
        is_public
    )
VALUES
    (
        sqlc.arg(run_id),
        sqlc.arg(archive_path),
        sqlc.arg(file_type),
        sqlc.arg(media_type),
        sqlc.arg(size_bytes),
        sqlc.arg(sha256),
        sqlc.arg(is_public)
    )
RETURNING
    *;

-- name: StoreArchiveFileContent :one
UPDATE
    archive_files
SET
    content = sqlc.arg(content),
    content_available = 1,
    content_error = NULL
WHERE
    id = sqlc.arg(id)
RETURNING
    *;

-- name: MarkArchiveFileContentUnavailable :exec
UPDATE
    archive_files
SET
    content = NULL,
    content_available = 0,
    content_error = sqlc.arg(content_error)
WHERE
    id = sqlc.arg(id);

-- name: GetArchiveFileContentByID :one
SELECT
    af.*,
    r.run_uid,
    r.workflow,
    r.commit_sha,
    r.status AS run_status,
    a.relative_path AS archive_relative_path
FROM
    archive_files af
    JOIN runs r ON r.id = af.run_id
    JOIN archives a ON a.id = r.archive_id
WHERE
    af.id = sqlc.arg(id)
LIMIT
    1;

-- name: ListArtifactFiles :many
SELECT
    af.id,
    af.run_id,
    af.archive_path,
    af.file_type,
    af.media_type,
    af.size_bytes,
    af.sha256,
    af.is_public,
    af.content_available,
    af.content_error,
    r.run_uid,
    r.workflow,
    r.commit_sha,
    r.status AS run_status,
    r.started_at
FROM
    archive_files af
    JOIN runs r ON r.id = af.run_id
WHERE
    af.id < sqlc.arg(before_id)
    AND (
        sqlc.arg(search_text) = ''
        OR af.archive_path LIKE '%' || sqlc.arg(search_text) || '%'
        OR af.media_type LIKE '%' || sqlc.arg(search_text) || '%'
        OR r.run_uid LIKE '%' || sqlc.arg(search_text) || '%'
        OR COALESCE(r.workflow, '') LIKE '%' || sqlc.arg(search_text) || '%'
    )
    AND (
        sqlc.arg(file_type_filter) = ''
        OR af.file_type = sqlc.arg(file_type_filter)
    )
    AND (
        sqlc.arg(media_type_filter) = ''
        OR af.media_type LIKE sqlc.arg(media_type_filter) || '%'
    )
    AND (
        sqlc.arg(workflow_filter) = ''
        OR COALESCE(r.workflow, '') = sqlc.arg(workflow_filter)
    )
    AND (
        sqlc.arg(run_filter) = ''
        OR r.run_uid = sqlc.arg(run_filter)
    )
    AND (
        sqlc.arg(public_filter) < 0
        OR af.is_public = sqlc.arg(public_filter)
    )
    AND (
        sqlc.arg(available_filter) < 0
        OR af.content_available = sqlc.arg(available_filter)
    )
ORDER BY
    af.id DESC
LIMIT
    sqlc.arg(result_limit);

-- name: GetArchiveFileForRunPath :one
SELECT
    af.*
FROM
    archive_files af
    JOIN runs r ON r.id = af.run_id
WHERE
    r.run_uid = sqlc.arg(run_uid)
    AND af.archive_path = sqlc.arg(archive_path)
LIMIT
    1;

-- name: UpsertArchiveFile :one
INSERT INTO
    archive_files (
        run_id,
        archive_path,
        file_type,
        media_type,
        size_bytes,
        sha256,
        is_public
    )
VALUES
    (
        sqlc.arg(run_id),
        sqlc.arg(archive_path),
        sqlc.arg(file_type),
        sqlc.arg(media_type),
        sqlc.arg(size_bytes),
        sqlc.arg(sha256),
        sqlc.arg(is_public)
    ) ON CONFLICT(run_id, archive_path) DO
UPDATE
SET
    file_type = excluded.file_type,
    media_type = excluded.media_type,
    size_bytes = excluded.size_bytes,
    sha256 = excluded.sha256,
    is_public = excluded.is_public
RETURNING
    *;

-- name: GetArchiveFileByID :one
SELECT
    af.*,
    r.run_uid,
    a.relative_path AS archive_relative_path
FROM
    archive_files af
    JOIN runs r ON r.id = af.run_id
    JOIN archives a ON a.id = r.archive_id
WHERE
    af.id = sqlc.arg(id)
LIMIT
    1;

-- Use this for the public artifact endpoint.
-- It guarantees the returned entry is marked public.
-- name: GetPublicArchiveFileByID :one
SELECT
    af.*,
    r.run_uid,
    a.relative_path AS archive_relative_path
FROM
    archive_files af
    JOIN runs r ON r.id = af.run_id
    JOIN archives a ON a.id = r.archive_id
WHERE
    af.id = sqlc.arg(id)
    AND af.is_public = 1
LIMIT
    1;

-- name: ListArchiveFilesForRun :many
SELECT
    id,
    run_id,
    archive_path,
    file_type,
    media_type,
    size_bytes,
    sha256,
    is_public,
    content_available,
    content_error
FROM
    archive_files
WHERE
    run_id = sqlc.arg(run_id)
ORDER BY
    archive_path;

-- name: ListPublicArchiveFilesForRun :many
SELECT
    id,
    run_id,
    archive_path,
    file_type,
    media_type,
    size_bytes,
    sha256,
    is_public,
    content_available,
    content_error
FROM
    archive_files
WHERE
    run_id = sqlc.arg(run_id)
    AND is_public = 1
ORDER BY
    archive_path;
