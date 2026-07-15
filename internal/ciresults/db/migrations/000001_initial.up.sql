CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    project_key TEXT NOT NULL UNIQUE,
    name TEXT,
    repository TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (length(project_key) BETWEEN 1 AND 200)
) STRICT;

CREATE TABLE devices (
    id INTEGER PRIMARY KEY,
    -- Stable identifier supplied by CI, e.g. laptop-1.
    device_key TEXT NOT NULL UNIQUE,
    display_name TEXT,
    last_os TEXT,
    last_os_version TEXT,
    last_kernel TEXT,
    last_arch TEXT,
    last_cpu TEXT,
    last_memory_bytes INTEGER,
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata_json)),
    first_seen_at TEXT,
    last_seen_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (length(device_key) BETWEEN 1 AND 200),
    CHECK (
        last_memory_bytes IS NULL
        OR last_memory_bytes >= 0
    )
) STRICT;

-- One record for every discovered tar.zst file, including failed imports.
CREATE TABLE archives (
    id INTEGER PRIMARY KEY,
    relative_path TEXT NOT NULL UNIQUE,
    file_size_bytes INTEGER NOT NULL,
    file_mtime_ns INTEGER,
    archive_sha256 TEXT,
    import_status TEXT NOT NULL DEFAULT 'discovered',
    import_attempts INTEGER NOT NULL DEFAULT 0,
    import_error TEXT,
    discovered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    import_started_at TEXT,
    imported_at TEXT,
    CHECK (
        import_status IN (
            'discovered',
            'importing',
            'imported',
            'failed',
            'ignored'
        )
    ),
    CHECK (file_size_bytes >= 0),
    CHECK (
        file_mtime_ns IS NULL
        OR file_mtime_ns >= 0
    ),
    CHECK (
        archive_sha256 IS NULL
        OR archive_sha256 GLOB '[0-9a-f][0-9a-f][0-9a-f][0-9a-f]*'
    ),
    CHECK (import_attempts >= 0),
    CHECK (
        relative_path NOT LIKE '/%'
        AND relative_path NOT LIKE '%..%'
        AND relative_path NOT LIKE '%.partial'
    )
) STRICT;

CREATE TABLE runs (
    id INTEGER PRIMARY KEY,
    -- UUIDv7 from manifest.json.
    run_uid TEXT NOT NULL UNIQUE,
    schema_version INTEGER NOT NULL,
    archive_id INTEGER NOT NULL UNIQUE REFERENCES archives(id) ON DELETE RESTRICT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    -- Source information.
    source_type TEXT,
    source_provider TEXT,
    workflow TEXT,
    job TEXT,
    trigger_name TEXT,
    source_attempt INTEGER NOT NULL DEFAULT 1,
    -- Git information.
    commit_sha TEXT NOT NULL,
    branch TEXT,
    tag TEXT,
    git_dirty INTEGER,
    commit_timestamp TEXT,
    -- Run timing.
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    STATUS TEXT NOT NULL,
    total_count INTEGER NOT NULL,
    passed_count INTEGER NOT NULL,
    failed_count INTEGER NOT NULL,
    skipped_count INTEGER NOT NULL,
    errored_count INTEGER NOT NULL,
    cancelled_count INTEGER NOT NULL DEFAULT 0,
    -- Flexible manifest sections retained for future fields.
    software_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(software_json)),
    environment_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(environment_json)),
    labels_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(labels_json)),
    manifest_json TEXT NOT NULL CHECK (json_valid(manifest_json)),
    imported_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (schema_version >= 1),
    CHECK (
        source_type IS NULL
        OR source_type IN ('ci', 'local', 'scheduled', 'manual')
    ),
    CHECK (source_attempt >= 1),
    CHECK (
        STATUS IN ('passed', 'failed', 'error', 'cancelled')
    ),
    CHECK (length(commit_sha) = 40),
    CHECK (commit_sha NOT GLOB '*[^0-9a-f]*'),
    CHECK (duration_ms >= 0),
    CHECK (total_count >= 0),
    CHECK (passed_count >= 0),
    CHECK (failed_count >= 0),
    CHECK (skipped_count >= 0),
    CHECK (errored_count >= 0),
    CHECK (cancelled_count >= 0),
    CHECK (
        total_count = passed_count + failed_count + skipped_count + errored_count + cancelled_count
    ),
    CHECK (
        git_dirty IS NULL
        OR git_dirty IN (0, 1)
    )
) STRICT;

-- Stable identities shared across every run.
CREATE TABLE test_cases (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- e.g. bridge/backend/tap/exact-frame-forwarding
    test_key TEXT NOT NULL,
    name TEXT NOT NULL,
    suite TEXT,
    result_kind TEXT NOT NULL DEFAULT 'go_test',
    source_file TEXT,
    source_symbol TEXT,
    first_seen_at TEXT,
    last_seen_at TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata_json)),
    UNIQUE (project_id, test_key),
    CHECK (length(test_key) BETWEEN 1 AND 500),
    CHECK (test_key = lower(test_key)),
    CHECK (test_key NOT LIKE '/%'),
    CHECK (test_key NOT LIKE '%/'),
    CHECK (test_key NOT LIKE '%..%'),
    CHECK (
        result_kind IN (
            'go_test',
            'provider_operation',
            'topology_run',
            'cross_host_endpoint',
            'other'
        )
    )
) STRICT;

CREATE TABLE test_results (
    id INTEGER PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    test_case_id INTEGER NOT NULL REFERENCES test_cases(id) ON DELETE RESTRICT,
    attempt INTEGER NOT NULL DEFAULT 1,
    STATUS TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    duration_ms INTEGER,
    message TEXT,
    failure_kind TEXT,
    failure_phase TEXT,
    failure_code TEXT,
    expected_value TEXT,
    actual_value TEXT,
    stack_trace TEXT,
    parameters_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters_json)),
    metrics_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metrics_json)),
    labels_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(labels_json)),
    details_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(details_json)),
    UNIQUE (run_id, test_case_id, attempt),
    CHECK (attempt >= 1),
    CHECK (
        STATUS IN (
            'passed',
            'failed',
            'skipped',
            'error',
            'cancelled'
        )
    ),
    CHECK (
        duration_ms IS NULL
        OR duration_ms >= 0
    ),
    CHECK (
        failure_kind IS NULL
        OR failure_kind IN (
            'assertion',
            'timeout',
            'process_exit',
            'crash',
            'setup',
            'cleanup',
            'network',
            'resource_exhaustion',
            'unsupported',
            'unknown'
        )
    )
) STRICT;

-- Every entry listed in manifest.files.
CREATE TABLE archive_files (
    id INTEGER PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    archive_path TEXT NOT NULL,
    file_type TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    is_public INTEGER NOT NULL,
    UNIQUE (run_id, archive_path),
    CHECK (size_bytes >= 0),
    CHECK (is_public IN (0, 1)),
    CHECK (
        file_type IN (
            'artifact',
            'config',
            'log',
            'packet_capture',
            'results'
        )
    ),
    CHECK (length(sha256) = 64),
    CHECK (sha256 NOT GLOB '*[^0-9a-f]*'),
    CHECK (
        archive_path <> ''
        AND archive_path NOT LIKE '/%'
        AND archive_path NOT LIKE '%..%'
    )
) STRICT;

-- Connect individual results to logs, PCAPs, configs, or other entries.
CREATE TABLE result_files (
    result_id INTEGER NOT NULL REFERENCES test_results(id) ON DELETE CASCADE,
    archive_file_id INTEGER NOT NULL REFERENCES archive_files(id) ON DELETE CASCADE,
    relationship TEXT NOT NULL DEFAULT 'artifact',
    PRIMARY KEY (result_id, archive_file_id),
    CHECK (
        relationship IN (
            'artifact',
            'log',
            'config',
            'packet_capture',
            'input',
            'output'
        )
    )
) WITHOUT ROWID,
STRICT;

CREATE TABLE features (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    feature_key TEXT NOT NULL,
    name TEXT,
    description TEXT,
    source_file TEXT,
    source_symbol TEXT,
    UNIQUE (project_id, feature_key),
    CHECK (length(feature_key) BETWEEN 1 AND 500),
    CHECK (feature_key = lower(feature_key))
) STRICT;

CREATE TABLE test_features (
    test_case_id INTEGER NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    feature_id INTEGER NOT NULL REFERENCES features(id) ON DELETE CASCADE,
    PRIMARY KEY (test_case_id, feature_id)
) WITHOUT ROWID,
STRICT;

CREATE INDEX runs_started_at_idx ON runs(started_at DESC);

CREATE INDEX runs_project_started_at_idx ON runs(project_id, started_at DESC);

CREATE INDEX runs_branch_started_at_idx ON runs(branch, started_at DESC);

CREATE INDEX runs_device_started_at_idx ON runs(device_id, started_at DESC);

CREATE INDEX runs_status_started_at_idx ON runs(STATUS, started_at DESC);

CREATE INDEX runs_commit_sha_idx ON runs(commit_sha);

CREATE INDEX test_results_run_status_idx ON test_results(run_id, STATUS);

CREATE INDEX test_results_case_run_idx ON test_results(test_case_id, run_id DESC);

CREATE INDEX test_results_status_idx ON test_results(STATUS);

CREATE INDEX test_results_duration_idx ON test_results(duration_ms DESC)
WHERE
    duration_ms IS NOT NULL;

CREATE INDEX test_results_failures_idx ON test_results(test_case_id, run_id DESC)
WHERE
    STATUS IN ('failed', 'error');

CREATE INDEX archive_files_run_public_idx ON archive_files(run_id, is_public);

CREATE INDEX archives_import_status_idx ON archives(import_status, discovered_at);

CREATE VIEW latest_test_status_by_device AS WITH ranked AS (
    SELECT
        tc.project_id,
        tc.id AS test_case_id,
        tc.test_key,
        tc.name AS test_name,
        r.device_id,
        d.device_key,
        d.display_name AS device_name,
        r.run_uid,
        r.commit_sha,
        r.branch,
        r.started_at,
        tr.status,
        tr.duration_ms,
        tr.message,
        row_number() OVER (
            PARTITION BY tc.project_id,
            tc.id,
            r.device_id
            ORDER BY
                r.started_at DESC,
                r.id DESC,
                tr.attempt DESC
        ) AS position
    FROM
        test_results tr
        JOIN runs r ON r.id = tr.run_id
        JOIN test_cases tc ON tc.id = tr.test_case_id
        JOIN devices d ON d.id = r.device_id
)
SELECT
    project_id,
    test_case_id,
    test_key,
    test_name,
    device_id,
    device_key,
    device_name,
    run_uid,
    commit_sha,
    branch,
    started_at,
    STATUS,
    duration_ms,
    message
FROM
    ranked
WHERE
    position = 1;
