import { errAsync, ok } from 'neverthrow';
import { fetchJson, type ApiError, type Fetch } from './http';
import type { ExecuteQueryRequest, ExecuteQueryResponse } from './types';
import type { SchemaInfo } from '../schema_types';

export function getSchemaInfo(fetcher: Fetch) {
  return fetchJson<unknown>(fetcher, '/api/v1/schema/info').andThen((data) => {
    if (!data) {
      return errAsync<SchemaInfo, ApiError>({
        type: 'invalid-response',
        message: 'Expected the schema info API to return an object'
      });
    }

    return ok(data as SchemaInfo);
  });
}

export function executeQuery(fetcher: Fetch, sql: string) {
  return fetchJson<ExecuteQueryResponse>(fetcher, '/api/v1/query', {
    method: 'POST',
    headers: {
      'content-type': 'application/json'
    },
    body: JSON.stringify({
      sql
    } satisfies ExecuteQueryRequest)
  });
}
