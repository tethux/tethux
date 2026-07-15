import { errAsync, ok } from 'neverthrow';
import { fetchJson, type ApiError, type Fetch } from './http';
import type { Test } from './types';

export function getTests(fetcher: Fetch) {
  return fetchJson<unknown>(fetcher, '/api/v1/tests').andThen((data) => {
    if (!Array.isArray(data)) {
      return errAsync<Test[], ApiError>({
        type: 'invalid-response',
        message: 'Expected the tests API to return an array'
      });
    }

    return ok(data as Test[]);
  });
}
