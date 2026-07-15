import { errAsync, ok } from 'neverthrow';
import { fetchJson, type ApiError, type Fetch } from './http';
import type { Run } from './types';

export function getRuns(fetcher: Fetch) {
  return fetchJson<unknown>(fetcher, '/api/v1/runs').andThen((data) => {
    if (!Array.isArray(data)) {
      return errAsync<Run[], ApiError>({
        type: 'invalid-response',
        message: 'Expected the runs API to return an array'
      });
    }

    return ok(data as Run[]);
  });
}
