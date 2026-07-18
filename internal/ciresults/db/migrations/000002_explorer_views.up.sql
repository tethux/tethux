CREATE TABLE saved_queries (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    sql_text TEXT NOT NULL,
    is_favorite INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    updated_at TEXT NOT NULL DEFAULT (
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    ),
    CHECK (length(name) BETWEEN 1 AND 200),
    CHECK (length(sql_text) BETWEEN 1 AND 100000),
    CHECK (is_favorite IN (0, 1))
) STRICT;

CREATE UNIQUE INDEX saved_queries_name_idx ON saved_queries(name);

CREATE INDEX saved_queries_favorite_updated_idx ON saved_queries(is_favorite DESC, updated_at DESC);

CREATE VIEW run_explorer AS
SELECT
    r.id AS run_id,
    r.run_uid,
    r.schema_version,
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    p.repository,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name,
    d.last_os AS device_os,
    d.last_os_version AS device_os_version,
    d.last_kernel AS device_kernel,
    d.last_arch AS device_arch,
    d.last_cpu AS device_cpu,
    d.last_memory_bytes AS device_memory_bytes,
    a.id AS archive_id,
    a.relative_path AS archive_relative_path,
    a.file_size_bytes AS archive_size_bytes,
    a.archive_sha256,
    a.import_status,
    a.import_attempts,
    a.import_error,
    a.discovered_at AS archive_discovered_at,
    a.import_started_at AS archive_import_started_at,
    a.imported_at AS archive_imported_at,
    r.source_type,
    r.source_provider,
    r.workflow,
    r.job,
    r.trigger_name,
    r.source_attempt,
    r.commit_sha,
    r.branch,
    r.tag,
    r.git_dirty,
    r.commit_timestamp,
    r.started_at,
    r.finished_at,
    r.duration_ms,
    r.status,
    r.total_count,
    r.passed_count,
    r.failed_count,
    r.skipped_count,
    r.errored_count,
    r.cancelled_count,
    CASE
        WHEN r.total_count = 0 THEN NULL
        ELSE CAST(r.passed_count AS REAL) / r.total_count
    END AS pass_rate,
    r.software_json,
    r.environment_json,
    r.labels_json,
    r.manifest_json,
    r.imported_at
FROM
    runs AS r
    JOIN projects AS p ON p.id = r.project_id
    JOIN devices AS d ON d.id = r.device_id
    JOIN archives AS a ON a.id = r.archive_id;

CREATE VIEW result_explorer AS
SELECT
    tr.id AS result_id,
    tr.attempt,
    r.id AS run_id,
    r.run_uid,
    r.schema_version,
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    p.repository,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name,
    d.last_os AS device_os,
    d.last_os_version AS device_os_version,
    d.last_arch AS device_arch,
    r.source_type,
    r.source_provider,
    r.workflow,
    r.job,
    r.trigger_name,
    r.source_attempt,
    r.commit_sha,
    r.branch,
    r.tag,
    r.git_dirty,
    r.commit_timestamp,
    r.started_at AS run_started_at,
    r.finished_at AS run_finished_at,
    r.duration_ms AS run_duration_ms,
    r.status AS run_status,
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.result_kind,
    tc.source_file,
    tc.source_symbol,
    tr.status AS result_status,
    tr.started_at AS result_started_at,
    tr.finished_at AS result_finished_at,
    tr.duration_ms,
    tr.message,
    tr.failure_kind,
    tr.failure_phase,
    tr.failure_code,
    tr.expected_value,
    tr.actual_value,
    tr.stack_trace,
    tr.parameters_json,
    tr.metrics_json,
    tr.labels_json,
    tr.details_json
FROM
    test_results AS tr
    JOIN runs AS r ON r.id = tr.run_id
    JOIN projects AS p ON p.id = r.project_id
    JOIN devices AS d ON d.id = r.device_id
    JOIN test_cases AS tc ON tc.id = tr.test_case_id;

CREATE VIEW failure_explorer AS
SELECT
    *
FROM
    result_explorer
WHERE
    result_status IN ('failed', 'error');

CREATE VIEW test_history AS
SELECT
    test_case_id,
    test_key,
    test_name,
    suite,
    result_kind,
    source_file,
    source_symbol,
    project_id,
    project_key,
    project_name,
    repository,
    run_id,
    run_uid,
    workflow,
    job,
    source_provider,
    source_attempt,
    commit_sha,
    branch,
    tag,
    git_dirty,
    commit_timestamp,
    device_id,
    device_key,
    device_name,
    device_os,
    device_os_version,
    device_arch,
    run_started_at AS started_at,
    attempt,
    result_status AS STATUS,
    duration_ms,
    failure_kind,
    failure_phase,
    failure_code,
    message,
    expected_value,
    actual_value,
    stack_trace
FROM
    result_explorer;

CREATE VIEW artifact_explorer AS
SELECT
    af.id AS archive_file_id,
    r.id AS run_id,
    r.run_uid,
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    p.repository,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name,
    r.workflow,
    r.job,
    r.commit_sha,
    r.branch,
    r.tag,
    r.started_at AS run_started_at,
    r.status AS run_status,
    af.archive_path,
    af.file_type,
    af.media_type,
    af.size_bytes,
    af.sha256,
    af.is_public
FROM
    archive_files AS af
    JOIN runs AS r ON r.id = af.run_id
    JOIN projects AS p ON p.id = r.project_id
    JOIN devices AS d ON d.id = r.device_id;

CREATE VIEW result_artifact_explorer AS
SELECT
    tr.id AS result_id,
    tr.status AS result_status,
    tr.attempt,
    tr.message,
    tr.failure_kind,
    tr.failure_phase,
    tr.failure_code,
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    r.id AS run_id,
    r.run_uid,
    r.workflow,
    r.job,
    r.commit_sha,
    r.branch,
    r.tag,
    r.started_at AS run_started_at,
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    d.id AS device_id,
    d.device_key,
    d.display_name AS device_name,
    rf.relationship,
    af.id AS archive_file_id,
    af.archive_path,
    af.file_type,
    af.media_type,
    af.size_bytes,
    af.sha256,
    af.is_public
FROM
    result_files AS rf
    JOIN test_results AS tr ON tr.id = rf.result_id
    JOIN test_cases AS tc ON tc.id = tr.test_case_id
    JOIN runs AS r ON r.id = tr.run_id
    JOIN projects AS p ON p.id = r.project_id
    JOIN devices AS d ON d.id = r.device_id
    JOIN archive_files AS af ON af.id = rf.archive_file_id;

CREATE VIEW feature_explorer AS
SELECT
    f.id AS feature_id,
    f.feature_key,
    f.name AS feature_name,
    f.description AS feature_description,
    f.source_file AS feature_source_file,
    f.source_symbol AS feature_source_symbol,
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    p.repository,
    COUNT(tf.test_case_id) AS test_count
FROM
    features AS f
    JOIN projects AS p ON p.id = f.project_id
    LEFT JOIN test_features AS tf ON tf.feature_id = f.id
GROUP BY
    f.id,
    f.feature_key,
    f.name,
    f.description,
    f.source_file,
    f.source_symbol,
    p.id,
    p.project_key,
    p.name,
    p.repository;

CREATE VIEW test_feature_explorer AS
SELECT
    p.id AS project_id,
    p.project_key,
    p.name AS project_name,
    tc.id AS test_case_id,
    tc.test_key,
    tc.name AS test_name,
    tc.suite,
    tc.result_kind,
    tc.source_file AS test_source_file,
    tc.source_symbol AS test_source_symbol,
    f.id AS feature_id,
    f.feature_key,
    f.name AS feature_name,
    f.description AS feature_description,
    f.source_file AS feature_source_file,
    f.source_symbol AS feature_source_symbol
FROM
    test_features AS tf
    JOIN test_cases AS tc ON tc.id = tf.test_case_id
    JOIN features AS f ON f.id = tf.feature_id
    JOIN projects AS p ON p.id = tc.project_id;

CREATE VIEW run_failure_summary AS
SELECT
    r.id AS run_id,
    r.run_uid,
    p.id AS project_id,
    p.project_key,
    d.id AS device_id,
    d.device_key,
    r.workflow,
    r.job,
    r.commit_sha,
    r.branch,
    r.started_at,
    r.status AS run_status,
    COUNT(tr.id) FILTER (
        WHERE
            tr.status IN ('failed', 'error')
    ) AS failure_count,
    COUNT(DISTINCT tr.test_case_id) FILTER (
        WHERE
            tr.status IN ('failed', 'error')
    ) AS failed_test_count,
    COUNT(DISTINCT tr.failure_kind) FILTER (
        WHERE
            tr.status IN ('failed', 'error')
            AND tr.failure_kind IS NOT NULL
    ) AS failure_kind_count
FROM
    runs AS r
    JOIN projects AS p ON p.id = r.project_id
    JOIN devices AS d ON d.id = r.device_id
    LEFT JOIN test_results AS tr ON tr.run_id = r.id
GROUP BY
    r.id,
    r.run_uid,
    p.id,
    p.project_key,
    d.id,
    d.device_key,
    r.workflow,
    r.job,
    r.commit_sha,
    r.branch,
    r.started_at,
    r.status;

CREATE VIRTUAL TABLE test_result_search USING fts5(
    message,
    stack_trace,
    expected_value,
    actual_value,
    content = 'test_results',
    content_rowid = 'id'
);
