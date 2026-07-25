import { fetchJson, type Fetch } from './http';
import type { ArtifactPage, ArtifactPreview, LogSearchResult } from './types';

export type ArtifactFilters = {
  q?: string;
  type?: string;
  media?: string;
  workflow?: string;
  run?: string;
  visibility?: string;
  availability?: string;
  cursor?: string;
  limit?: number;
};

export function listArtifacts(fetcher: Fetch, filters: ArtifactFilters = {}) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== '') params.set(key, String(value));
  }
  const query = params.size ? `?${params}` : '';
  return fetchJson<ArtifactPage>(fetcher, `/api/v1/artifacts${query}`);
}

export function getArtifact(fetcher: Fetch, id: number) {
  return fetchJson<ArtifactPreview>(fetcher, `/api/v1/file/${id}`);
}

export function searchArtifact(
  fetcher: Fetch,
  id: number,
  filters: { q?: string; regex?: boolean; severity?: string }
) {
  const params = new URLSearchParams();
  if (filters.q) params.set('q', filters.q);
  if (filters.regex) params.set('regex', 'true');
  if (filters.severity && filters.severity !== 'all') params.set('severity', filters.severity);
  return fetchJson<LogSearchResult>(fetcher, `/api/v1/file/${id}/search?${params}`);
}
