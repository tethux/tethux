-- name: GetArchiveByPath :one
SELECT
    *
FROM
    archives
WHERE
    relative_path = sqlc.arg(relative_path)
LIMIT
    1;

-- name: GetArchiveByID :one
SELECT
    *
FROM
    archives
WHERE
    id = sqlc.arg(id)
LIMIT
    1;

-- name: CreateArchive :one
INSERT INTO
    archives (
        relative_path,
        file_size_bytes,
        file_mtime_ns,
        archive_sha256,
        import_status
    )
VALUES
    (
        sqlc.arg(relative_path),
        sqlc.arg(file_size_bytes),
        sqlc.narg(file_mtime_ns),
        sqlc.narg(archive_sha256),
        'discovered'
    )
RETURNING
    *;

-- name: CreateImportingArchive :one
INSERT INTO
    archives (
        relative_path,
        file_size_bytes,
        file_mtime_ns,
        archive_sha256,
        import_status,
        import_attempts,
        import_started_at
    )
VALUES
    (
        sqlc.arg(relative_path),
        sqlc.arg(file_size_bytes),
        sqlc.narg(file_mtime_ns),
        sqlc.narg(archive_sha256),
        'importing',
        1,
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    )
RETURNING
    *;

-- Use this when rescanning files that may already be known.
-- It deliberately does not reset successfully imported archives.
-- name: UpsertDiscoveredArchive :one
INSERT INTO
    archives (
        relative_path,
        file_size_bytes,
        file_mtime_ns,
        archive_sha256,
        import_status
    )
VALUES
    (
        sqlc.arg(relative_path),
        sqlc.arg(file_size_bytes),
        sqlc.narg(file_mtime_ns),
        sqlc.narg(archive_sha256),
        'discovered'
    ) ON CONFLICT(relative_path) DO
UPDATE
SET
    file_size_bytes = excluded.file_size_bytes,
    file_mtime_ns = excluded.file_mtime_ns,
    archive_sha256 = COALESCE(
        excluded.archive_sha256,
        archives.archive_sha256
    )
WHERE
    archives.import_status != 'imported'
RETURNING
    *;

-- name: MarkArchiveImporting :one
UPDATE
    archives
SET
    import_status = 'importing',
    import_started_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    import_attempts = import_attempts + 1,
    import_error = NULL
WHERE
    id = sqlc.arg(id)
    AND import_status IN ('discovered', 'failed')
RETURNING
    *;

-- name: MarkArchiveImported :exec
UPDATE
    archives
SET
    import_status = 'imported',
    imported_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    import_error = NULL
WHERE
    id = sqlc.arg(id);

-- name: MarkArchiveFailed :exec
UPDATE
    archives
SET
    import_status = 'failed',
    import_error = sqlc.arg(import_error)
WHERE
    id = sqlc.arg(id);

-- name: MarkArchiveIgnored :exec
UPDATE
    archives
SET
    import_status = 'ignored',
    import_error = sqlc.narg(reason)
WHERE
    id = sqlc.arg(id);

-- name: ResetFailedArchive :exec
UPDATE
    archives
SET
    import_status = 'discovered',
    import_error = NULL,
    import_started_at = NULL
WHERE
    id = sqlc.arg(id)
    AND import_status = 'failed';

-- name: ListArchivesPendingImport :many
SELECT
    *
FROM
    archives
WHERE
    import_status IN ('discovered', 'failed')
ORDER BY
    discovered_at ASC
LIMIT
    sqlc.arg(result_limit);

-- name: ListFailedArchives :many
SELECT
    *
FROM
    archives
WHERE
    import_status = 'failed'
ORDER BY
    discovered_at DESC
LIMIT
    sqlc.arg(result_limit);

-- name: CountArchivesByImportStatus :many
SELECT
    import_status,
    COUNT(*) AS archive_count
FROM
    archives
GROUP BY
    import_status
ORDER BY
    import_status;
