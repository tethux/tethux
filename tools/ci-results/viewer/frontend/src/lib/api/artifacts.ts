import { fetchJson, type Fetch } from './http';
import type { ArtifactPreview } from './types';

export function getArtifact(fetcher: Fetch, id: number) {
  return fetchJson<ArtifactPreview>(fetcher, `/api/v1/file/${id}`);
}
