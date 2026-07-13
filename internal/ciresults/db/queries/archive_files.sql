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
    *
FROM
    archive_files
WHERE
    run_id = sqlc.arg(run_id)
ORDER BY
    archive_path;

-- name: ListPublicArchiveFilesForRun :many
SELECT
    *
FROM
    archive_files
WHERE
    run_id = sqlc.arg(run_id)
    AND is_public = 1
ORDER BY
    archive_path;
