-- name: UpsertFeature :one
INSERT INTO
    features (
        project_id,
        feature_key,
        name,
        description,
        source_file,
        source_symbol
    )
VALUES
    (
        sqlc.arg(project_id),
        sqlc.arg(feature_key),
        sqlc.narg(name),
        sqlc.narg(description),
        sqlc.narg(source_file),
        sqlc.narg(source_symbol)
    ) ON CONFLICT(project_id, feature_key) DO
UPDATE
SET
    name = COALESCE(excluded.name, features.name),
    description = COALESCE(
        excluded.description,
        features.description
    ),
    source_file = COALESCE(
        excluded.source_file,
        features.source_file
    ),
    source_symbol = COALESCE(
        excluded.source_symbol,
        features.source_symbol
    )
RETURNING
    *;

-- name: LinkTestFeature :exec
INSERT INTO
    test_features (test_case_id, feature_id)
VALUES
    (
        sqlc.arg(test_case_id),
        sqlc.arg(feature_id)
    ) ON CONFLICT(test_case_id, feature_id) DO NOTHING;

-- name: ListFeaturesForTest :many
SELECT
    f.*
FROM
    test_features tf
    JOIN features f ON f.id = tf.feature_id
WHERE
    tf.test_case_id = sqlc.arg(test_case_id)
ORDER BY
    f.feature_key;

-- name: ListTestsForFeature :many
SELECT
    tc.*
FROM
    test_features tf
    JOIN test_cases tc ON tc.id = tf.test_case_id
WHERE
    tf.feature_id = sqlc.arg(feature_id)
ORDER BY
    tc.test_key;
