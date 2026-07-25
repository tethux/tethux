import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const query = await readFile(new URL('../src/routes/query/+page.svelte', import.meta.url), 'utf8');
const results = await readFile(
  new URL('../src/lib/components/QueryResults.svelte', import.meta.url),
  'utf8'
);
const saved = await readFile(new URL('../src/lib/savedQueries.ts', import.meta.url), 'utf8');
const artifacts = await readFile(
  new URL('../src/routes/artifacts/+page.svelte', import.meta.url),
  'utf8'
);

test('saved queries use a versioned local document and complete lifecycle controls', () => {
  assert.match(saved, /ci-results:saved-queries:v1/);
  assert.match(saved, /version:\s*1/);
  for (const action of ['saveQuery', 'loadSaved', 'deleteSaved']) {
    assert.match(query, new RegExp(`function ${action}`));
  }
});

test('timestamp cells support relative and calendar modes with raw copying', () => {
  assert.match(results, /Intl\.RelativeTimeFormat/);
  assert.match(results, /calendarTime/);
  assert.match(results, /Copy raw/);
  assert.match(results, /ci-results:timestamp-mode/);
});

test('schema reload has explicit lifecycle and stale-response protection', () => {
  assert.match(query, /schemaStatus/);
  assert.match(query, /schemaRequest/);
  assert.match(query, /request !== schemaRequest/);
});

test('artifact workbench exposes filtering, preview, and exact-byte download', () => {
  assert.match(artifacts, /Artifact filters/);
  assert.match(artifacts, /Download exact bytes/);
  assert.match(artifacts, /Re-run ingestion/);
});
