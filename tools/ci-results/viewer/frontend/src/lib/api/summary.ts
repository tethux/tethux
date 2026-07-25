import { errAsync, ok } from 'neverthrow';
import { fetchJson, type ApiError, type Fetch } from './http';
import type { ViewerSummary } from './types';

export function getSummary(fetcher: Fetch) {
  return fetchJson<unknown>(fetcher, '/api/v1/summary').andThen((data) => {
    if (!data || typeof data !== 'object') {
      return errAsync<ViewerSummary, ApiError>({
        type: 'invalid-response',
        message: 'Expected the summary API to return an object'
      });
    }
    return ok(data as ViewerSummary);
  });
}
