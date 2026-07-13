-- name: LinkResultFile :exec
INSERT INTO
    result_files (
        result_id,
        archive_file_id,
        relationship
    )
VALUES
    (
        sqlc.arg(result_id),
        sqlc.arg(archive_file_id),
        sqlc.arg(relationship)
    ) ON CONFLICT(result_id, archive_file_id) DO
UPDATE
SET
    relationship = excluded.relationship;

-- name: ListFilesForResult :many
SELECT
    af.*,
    rf.relationship
FROM
    result_files rf
    JOIN archive_files af ON af.id = rf.archive_file_id
WHERE
    rf.result_id = sqlc.arg(result_id)
ORDER BY
    rf.relationship,
    af.archive_path;

-- name: ListPublicFilesForResult :many
SELECT
    af.*,
    rf.relationship
FROM
    result_files rf
    JOIN archive_files af ON af.id = rf.archive_file_id
WHERE
    rf.result_id = sqlc.arg(result_id)
    AND af.is_public = 1
ORDER BY
    rf.relationship,
    af.archive_path;
