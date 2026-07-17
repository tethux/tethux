type NullString = {
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
