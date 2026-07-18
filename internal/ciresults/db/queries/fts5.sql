-- name: SearchTestResultMessages :many
SELECT
    tr.id,
    tr.run_id,
    tr.test_case_id,
    tr.status,
    tr.message,
    tr.stack_trace
FROM
    test_result_search AS fts
    JOIN test_results AS tr ON tr.id = fts.rowid
WHERE
    fts.message MATCH sqlc.arg(query)
LIMIT
    sqlc.arg(limit_rows);
