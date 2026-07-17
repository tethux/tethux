import { errAsync, ok } from 'neverthrow';
import { fetchJson, type ApiError, type Fetch } from './http';
import type { RunDetail } from './types';

export function getRunRow(id: string, fetcher: Fetch) {
  return fetchJson<unknown>(fetcher, `/api/v1/run/${id}`).andThen((data) => {
    if (Array.isArray(data) || data === null || typeof data !== 'object' || !('run' in data)) {
      return errAsync<RunDetail, ApiError>({
        type: 'invalid-response',
        message: 'Expected the runs API to return a run object'
      });
    }

    return ok(data as RunDetail);
  });
}
