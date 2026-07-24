import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const layout = await readFile(new URL('../src/routes/+layout.svelte', import.meta.url), 'utf8');
const queryResults = await readFile(
  new URL('../src/lib/components/QueryResults.svelte', import.meta.url),
  'utf8'
);
const queryPage = await readFile(
  new URL('../src/routes/query/+page.svelte', import.meta.url),
  'utf8'
);
const runsPage = await readFile(
  new URL('../src/routes/runs/+page.svelte', import.meta.url),
  'utf8'
);
const runPage = await readFile(
  new URL('../src/routes/run/[id]/+page.svelte', import.meta.url),
  'utf8'
);
const testsPage = await readFile(
  new URL('../src/routes/tests/+page.svelte', import.meta.url),
  'utf8'
);

test('light and dark themes expose the complete query palette', () => {
  for (const token of [
    '--base',
    '--surface',
    '--text',
    '--subtle',
    '--muted',
    '--border',
    '--focus',
    '--love',
    '--gold',
    '--syntax-green',
    '--syntax-blue'
  ]) {
    assert.match(layout, new RegExp(`${token}:`), `missing ${token}`);
  }
  assert.match(layout, /:global\(html\.dark\)/);
});

test('query fields consume theme tokens instead of fixed surfaces', () => {
  for (const selector of ['.detail-panel', '.detail-controls', '.field', '.json-view']) {
    const rule = queryResults.match(new RegExp(`\\${selector} \\{[^}]+\\}`))?.[0] ?? '';
    assert.match(rule, /var\(--(?:base|surface|text|border)\)/, `${selector} is not themed`);
  }
});

test('all query status families have semantic theme colors', () => {
  assert.match(queryResults, /status-passed[^}]+var\(--syntax-green\)/);
  assert.match(queryResults, /status-failed[^}]+var\(--love\)/);
  assert.match(queryResults, /status-skipped[^}]+var\(--gold\)/);
  assert.match(queryResults, /status-cancelled[^}]+var\(--subtle\)/);
});

test('query controls keep their responsive interaction contracts', () => {
  assert.match(queryPage, /class:active=\{schemaOpen\}/);
  assert.match(queryPage, /onclick=\{\(\) => \(schemaOpen = true\)\}/);
  assert.match(queryPage, /onclick=\{\(\) => \(schemaOpen = false\)\}/);
  assert.match(queryPage, /aria-controls="schema-panel"/);
  assert.match(queryPage, /calc\(100% - var\(--schema-width\) - 10px\)/);
  assert.match(queryPage, /width: max-content/);
  assert.match(queryPage, /background: var\(--syntax-blue\)/);
  assert.match(queryPage, /\.suggestions button\.selected[^}]+var\(--focus\)/s);
  assert.match(queryResults, /Intl\.DateTimeFormat/);
});

test('run and test routes consume the shared theme contract', () => {
  for (const route of [runsPage, testsPage]) {
    assert.match(route, /var\(--base\)/);
    assert.match(route, /var\(--border\)/);
    assert.match(route, /var\(--subtle\)/);
  }
  assert.match(runPage, /background: var\(--base\) !important/);
  assert.match(runPage, /:global\(html\.dark\) \.test-panel/);
  assert.match(runPage, /:global\(html\.dark\) \.file-modal/);
  assert.match(testsPage, /\.passed[^}]+var\(--syntax-green\)/s);
});
