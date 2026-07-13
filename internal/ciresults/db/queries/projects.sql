-- name: UpsertProject :one
INSERT INTO
    projects (project_key, name, repository)
VALUES
    (
        sqlc.arg(project_key),
        sqlc.narg(name),
        sqlc.narg(repository)
    ) ON CONFLICT(project_key) DO
UPDATE
SET
    name = COALESCE(excluded.name, projects.name),
    repository = COALESCE(excluded.repository, projects.repository)
RETURNING
    *;

-- name: GetProjectByKey :one
SELECT
    *
FROM
    projects
WHERE
    project_key = sqlc.arg(project_key)
LIMIT
    1;

-- name: GetProjectByID :one
SELECT
    *
FROM
    projects
WHERE
    id = sqlc.arg(id)
LIMIT
    1;

-- name: ListProjects :many
SELECT
    *
FROM
    projects
ORDER BY
    project_key;
