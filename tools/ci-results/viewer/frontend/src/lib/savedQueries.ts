export const SAVED_QUERIES_KEY = 'ci-results:saved-queries:v1';

export type SavedQuery = {
  id: string;
  name: string;
  sql: string;
  createdAt: string;
  updatedAt: string;
};

type SavedQueryDocument = {
  version: 1;
  queries: SavedQuery[];
};

function validQuery(value: unknown): value is SavedQuery {
  if (!value || typeof value !== 'object') return false;
  const item = value as Record<string, unknown>;
  return ['id', 'name', 'sql', 'createdAt', 'updatedAt'].every(
    (key) => typeof item[key] === 'string'
  );
}

export function readSavedQueries(storage: Storage): SavedQuery[] {
  try {
    const parsed = JSON.parse(storage.getItem(SAVED_QUERIES_KEY) ?? '') as SavedQueryDocument;
    if (parsed.version !== 1 || !Array.isArray(parsed.queries)) return [];
    return parsed.queries.filter(validQuery).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
  } catch {
    return [];
  }
}

export function writeSavedQueries(storage: Storage, queries: SavedQuery[]): void {
  const document: SavedQueryDocument = { version: 1, queries };
  storage.setItem(SAVED_QUERIES_KEY, JSON.stringify(document));
}
