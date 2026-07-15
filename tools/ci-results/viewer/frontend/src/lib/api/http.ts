import { errAsync, ResultAsync } from 'neverthrow';

export type ApiError =
  | {
      type: 'network';
      message: string;
      cause: unknown;
    }
  | {
      type: 'http';
      message: string;
      status: number;
    }
  | {
      type: 'json';
      message: string;
      cause: unknown;
    }
  | {
      type: 'invalid-response';
      message: string;
    };

export type Fetch = typeof globalThis.fetch;

export function fetchJson<T>(
  fetcher: Fetch,
  input: RequestInfo | URL,
  init?: RequestInit
): ResultAsync<T, ApiError> {
  return ResultAsync.fromPromise(fetcher(input, init), (cause): ApiError => ({
    type: 'network',
    message: 'Could not connect to the server',
    cause
  })).andThen((response) => {
    if (!response.ok) {
      return errAsync<T, ApiError>({
        type: 'http',
        status: response.status,
        message: `Request failed (${response.status})`
      });
    }

    return ResultAsync.fromPromise(response.json() as Promise<T>, (cause): ApiError => ({
      type: 'json',
      message: 'The server returned invalid JSON',
      cause
    }));
  });
}
