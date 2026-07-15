/** Shapes returned by the CI viewer API. */
export type Test = {
  id: number;
  test_key: string;
  name: string;
  suite: string | null;
  result_kind: string;
  result_count: number;
  passed_count: number;
  failed_count: number;
  last_finished_at: string | null;
};

export type Run = {
  run_uid: string;
  status: string;
  /**
   * `sql.NullString` is encoded by the Go API as an object rather than a JSON
   * string. `Valid` distinguishes a detached run from an empty branch name.
   */
  branch: {
    String: string;
    Valid: boolean;
  };
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
