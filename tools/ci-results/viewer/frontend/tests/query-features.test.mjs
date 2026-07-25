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
const sql = await readFile(new URL('../src/routes/query/sql.ts', import.meta.url), 'utf8');
const dashboard = await readFile(new URL('../src/routes/+page.svelte', import.meta.url), 'utf8');
const runDetail = await readFile(
  new URL('../src/routes/run/[id]/+page.svelte', import.meta.url),
  'utf8'
);
const logSearch = await readFile(
  new URL('../src/lib/components/LogSearch.svelte', import.meta.url),
  'utf8'
);
const logPreview = await readFile(
  new URL('../src/lib/components/LogPreview.svelte', import.meta.url),
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
  assert.match(query, /function toggleSchema/);
  assert.match(query, /event\.metaKey \|\| event\.ctrlKey/);
  assert.match(query, /aria-expanded=\{schemaOpen\}/);
});

test('FROM and JOIN completion offers the complete scrollable schema object list', () => {
  assert.match(sql, /FROM\|JOIN/);
  assert.match(sql, /schema\.objects/);
  assert.match(query, /scrollIntoView\(\{ block: 'nearest' \}\)/);
  assert.match(query, /suggestions\.length} matches/);
});

test('query rows and headers share one stable grid', () => {
  assert.doesNotMatch(results, /VirtualList/);
  assert.match(results, /--query-columns/);
  assert.match(results, /grid-template-columns: repeat\(var\(--query-columns\)/);
});

test('artifact workbench exposes filtering, preview, and exact-byte download', () => {
  assert.match(artifacts, /Artifact filters/);
  assert.match(artifacts, /Download exact bytes/);
  assert.match(artifacts, /Re-run ingestion/);
  assert.match(logPreview, /Parsed log/);
  assert.match(logPreview, />JSON</);
  assert.match(logPreview, /<details/);
  assert.doesNotMatch(logPreview, /structured event/);
  assert.match(logPreview, /JSONL is parsed one record per line/);
});

test('diagnostic overview keeps health, duration, recent runs, and workflow steps concise', () => {
  assert.match(dashboard, /Build status/);
  assert.match(dashboard, /Build duration/);
  assert.match(dashboard, /Recent runs/);
  assert.match(dashboard, /class="y-axis"/);
  assert.match(dashboard, /chart-tooltip/);
  for (const chart of ['Pipelines', 'Tests', 'Failures', 'Host duration']) {
    assert.match(dashboard, new RegExp(chart));
  }
  assert.doesNotMatch(dashboard, /class="donut"/);
  assert.match(runDetail, /Workflow steps/);
  assert.match(runDetail, /configs\/workflow\.json/);
});

test('artifact logs expose complete-text search, regex, and severity filtering', () => {
  assert.match(logSearch, /Full log search/);
  assert.match(logSearch, /severity/);
  assert.match(logSearch, /regex/);
  assert.match(artifacts, /<LogSearch/);
  assert.match(runDetail, /<LogSearch/);
  assert.match(runDetail, /Search all run logs/);
  assert.match(runDetail, /Search complete log/);
  assert.match(runDetail, /<ChevronIcon/);
});
