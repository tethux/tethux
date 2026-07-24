export type NullString = {
  String: string;
  Valid: boolean;
};

export function nullStringValue(value: NullString): string | null {
  return value.Valid ? value.String : null;
}

export type Test = {
  id: number;
  test_key: string;
  name: string;
  suite: NullString;
  result_kind: string;
  result_count: number;
  passed_count: number;
  failed_count: number;
  last_finished_at: string | null;
};

export type Run = {
  run_uid: string;
  status: string;
  branch: NullString;
  commit_sha: string;
  started_at: string;
  duration_ms: number;
  total_count: number;
  passed_count: number;
  failed_count: number;
  skipped_count: number;
  errored_count: number;
  project_key: string;
  device_key: string;
};

export type RunRow = {
  id: number;
  run_uid: string;
  schema_version: number;
  archive_id: number;
  project_id: number;
  device_id: number;

  source_type: NullString;
  source_provider: NullString;
  workflow: NullString;
  job: NullString;
  trigger_name: NullString;
  source_attempt: number;

  commit_sha: string;
  branch: NullString;
  tag: NullString;
  git_dirty: 0 | 1 | null;
  commit_timestamp: string | null;

  started_at: string;
  finished_at: string;
  duration_ms: number;
  status: 'passed' | 'failed' | 'error' | 'cancelled';

  total_count: number;
  passed_count: number;
  failed_count: number;
  skipped_count: number;
  errored_count: number;
  cancelled_count: number;

  software_json: string;
  environment_json: string;
  labels_json: string;
  manifest_json: string;

  imported_at: string;
};

export type TestResult = {
  id: number;
  attempt: number;
  status: 'passed' | 'failed' | 'skipped' | 'error' | 'cancelled';
  started_at: NullString;
  finished_at: NullString;
  duration_ms: { Int64: number; Valid: boolean };
  message: NullString;
  failure_kind: NullString;
  stack_trace: NullString;
  test_key: string;
  test_name: string;
  suite: NullString;
  result_kind: string;
  parameters_json: string;
  metrics_json: string;
  labels_json: string;
  details_json: string;
  source_file: NullString;
  source_symbol: NullString;
};

export type ArchiveFile = {
  id: number;
  archive_path: string;
  file_type: string;
  media_type: string;
  size_bytes: number;
  sha256: string;
  is_public: number;
};

export type RunDetail = { run: RunRow; tests: TestResult[]; files: ArchiveFile[] };

export interface ExecuteQueryRequest {
  sql: string;
}

export interface QueryColumn {
  name: string;
  type: string;
}

export interface ExecuteQueryResponse {
  columns: QueryColumn[];
  rows: Record<string, unknown>[];
  row_count: number;
  duration_ms: number;
  truncated: boolean;
}
