<script lang="ts">
  import { onMount } from 'svelte';
  import VirtualList from '@humanspeak/svelte-virtual-list';
  import type { ExecuteQueryResponse, QueryColumn } from '$lib/api/types';

  let { result, source }: { result: ExecuteQueryResponse; source: string } = $props();

  let selectedRow = $state<Record<string, unknown> | null>(null);
  let selectedColumns = $state<string[]>([]);
  let columnsOpen = $state(false);
  let detailFilter = $state('');
  let showEmpty = $state(false);
  let detailView = $state<'fields' | 'json'>('fields');
  let hydratedSignature = $state('');

  type JsonToken = {
    value: string;
    kind: 'key' | 'string' | 'number' | 'boolean' | 'null' | 'punctuation' | 'plain';
  };

  const storageKey = $derived(`ci-query-columns:${source.toLowerCase()}`);
  const resultSignature = $derived(
    `${source}:${result.columns.map((column) => column.name).join('|')}`
  );
  const visibleColumns = $derived(
    result.columns.filter((column) => selectedColumns.includes(column.name))
  );
  const filteredFields = $derived(
    selectedRow
      ? result.columns.filter((column) => {
          const value = selectedRow?.[column.name];
          const matches = column.name.toLowerCase().includes(detailFilter.trim().toLowerCase());
          return matches && (showEmpty || !isEmpty(value));
        })
      : []
  );
  const unpackedRow = $derived(selectedRow ? unpackJson(selectedRow) : null);

  $effect(() => {
    if (typeof localStorage === 'undefined' || hydratedSignature === resultSignature) return;
    const available = new Set(result.columns.map((column) => column.name));
    const saved = JSON.parse(localStorage.getItem(storageKey) ?? '[]') as string[];
    selectedColumns = saved.filter((name) => available.has(name));
    if (!selectedColumns.length) selectedColumns = defaultColumns(result.columns);
    hydratedSignature = resultSignature;
  });

  $effect(() => {
    if (
      typeof localStorage !== 'undefined' &&
      hydratedSignature === resultSignature &&
      selectedColumns.length
    ) {
      localStorage.setItem(storageKey, JSON.stringify(selectedColumns));
    }
  });

  onMount(() => {
    if (!selectedColumns.length) selectedColumns = defaultColumns(result.columns);
  });

  function defaultColumns(columns: QueryColumn[]): string[] {
    const priority = [
      /^(timestamp|time|created_at|started_at|finished_at)$/i,
      /^(status|level|severity)$/i,
      /^(name|title|test_name|service_name|service\.name)$/i,
      /^(message|body|summary|error)$/i,
      /^(id|run_uid|test_key)$/i
    ];
    return [...columns]
      .map((column, index) => ({
        name: column.name,
        index,
        score: priority.findIndex((pattern) => pattern.test(column.name))
      }))
      .sort((a, b) => {
        const aScore = a.score < 0 ? priority.length : a.score;
        const bScore = b.score < 0 ? priority.length : b.score;
        return aScore - bScore || a.index - b.index;
      })
      .slice(0, Math.min(4, columns.length))
      .map(({ name }) => name);
  }

  function toggleColumn(name: string): void {
    if (selectedColumns.includes(name)) {
      if (selectedColumns.length > 1)
        selectedColumns = selectedColumns.filter((item) => item !== name);
    } else if (selectedColumns.length < 4) {
      selectedColumns = [...selectedColumns, name];
    }
  }

  function isEmpty(value: unknown): boolean {
    return value === null || value === undefined || value === '';
  }

  function statusKind(value: unknown): 'passed' | 'failed' | 'skipped' | 'cancelled' | null {
    const status = typeof value === 'string' ? value.trim().toLowerCase() : '';
    if (['passed', 'pass', 'success', 'successful', 'ok'].includes(status)) return 'passed';
    if (['failed', 'fail', 'error', 'errored'].includes(status)) return 'failed';
    if (['skipped', 'skip', 'pending'].includes(status)) return 'skipped';
    if (['cancelled', 'canceled'].includes(status)) return 'cancelled';
    return null;
  }

  function formatCell(value: unknown, columnName = ''): string {
    if (value === null) return 'NULL';
    if (value === undefined || value === '') return '—';
    const unpacked = unpackJson(value);
    if (typeof unpacked === 'string' && isTimestampColumn(columnName)) {
      const date = new Date(unpacked);
      if (!Number.isNaN(date.getTime())) {
        return new Intl.DateTimeFormat(undefined, {
          dateStyle: 'medium',
          timeStyle: 'medium'
        }).format(date);
      }
    }
    return typeof unpacked === 'object' ? JSON.stringify(unpacked) : String(unpacked);
  }

  function isTimestampColumn(name: string): boolean {
    return /(?:^|[._])(?:timestamp|time|created_at|updated_at|started_at|finished_at)$/i.test(name);
  }

  function unpackJson(value: unknown): unknown {
    if (typeof value === 'string') {
      const trimmed = value.trim();
      if (
        (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
        (trimmed.startsWith('[') && trimmed.endsWith(']'))
      ) {
        try {
          return unpackJson(JSON.parse(trimmed));
        } catch {
          return value;
        }
      }
      return value;
    }
    if (Array.isArray(value)) return value.map(unpackJson);
    if (value && typeof value === 'object') {
      return Object.fromEntries(
        Object.entries(value as Record<string, unknown>).map(([key, item]) => [
          key,
          unpackJson(item)
        ])
      );
    }
    return value;
  }

  function highlightJson(value: unknown, pretty = true): JsonToken[] {
    const json = JSON.stringify(unpackJson(value), null, pretty ? 2 : 0) ?? String(value);
    const pattern =
      /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false)\b|\b(null)\b|[{}[\],]/g;
    const tokens: JsonToken[] = [];
    let cursor = 0;

    for (const match of json.matchAll(pattern)) {
      const index = match.index;
      if (index > cursor) tokens.push({ value: json.slice(cursor, index), kind: 'plain' });

      if (match[1]) {
        tokens.push({ value: match[1], kind: match[2] ? 'key' : 'string' });
        if (match[2]) tokens.push({ value: match[2], kind: 'punctuation' });
      } else {
        const value = match[0];
        const kind: JsonToken['kind'] = match[3]
          ? 'number'
          : match[4]
            ? 'boolean'
            : match[5]
              ? 'null'
              : 'punctuation';
        tokens.push({ value, kind });
      }
      cursor = index + match[0].length;
    }

    if (cursor < json.length) tokens.push({ value: json.slice(cursor), kind: 'plain' });
    return tokens;
  }

  function closeDetails(): void {
    selectedRow = null;
    detailFilter = '';
  }
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape') {
      if (selectedRow) closeDetails();
      else columnsOpen = false;
    }
  }}
/>

<div class="results-tools">
  <span>{visibleColumns.length} summary fields</span>
  <div class="column-control">
    <button type="button" class:active={columnsOpen} onclick={() => (columnsOpen = !columnsOpen)}>
      Columns <small>{visibleColumns.length}/4</small>
    </button>
    {#if columnsOpen}
      <div class="column-menu">
        <header>
          <strong>Summary fields</strong>
          <small>Saved for {source}</small>
        </header>
        {#each result.columns as column (column.name)}
          <label>
            <input
              type="checkbox"
              checked={selectedColumns.includes(column.name)}
              disabled={!selectedColumns.includes(column.name) && selectedColumns.length >= 4}
              onchange={() => toggleColumn(column.name)}
            />
            <span>{column.name}</span>
            <small>{column.type}</small>
          </label>
        {/each}
      </div>
    {/if}
  </div>
</div>

<div class="query-results">
  <div
    class="query-columns"
    style={`grid-template-columns: repeat(${visibleColumns.length}, minmax(160px, 1fr));`}
  >
    {#each visibleColumns as column (column.name)}
      <span title={column.type}>{column.name}<small>{column.type}</small></span>
    {/each}
  </div>
  <div class="query-virtual-list">
    <VirtualList
      items={result.rows}
      defaultEstimatedItemHeight={42}
      bufferSize={8}
      hasMore={false}
      viewportLabel="Query results"
    >
      {#snippet renderItem(row: Record<string, unknown>)}
        <button
          class="query-row"
          class:selected={selectedRow === row}
          type="button"
          style={`grid-template-columns: repeat(${visibleColumns.length}, minmax(160px, 1fr));`}
          onclick={() => (selectedRow = row)}
          aria-label="Open row details"
        >
          {#each visibleColumns as column (column.name)}
            <span
              class:empty={isEmpty(row[column.name])}
              class:status-passed={statusKind(row[column.name]) === 'passed'}
              class:status-failed={statusKind(row[column.name]) === 'failed'}
              class:status-skipped={statusKind(row[column.name]) === 'skipped'}
              class:status-cancelled={statusKind(row[column.name]) === 'cancelled'}
              title={String(row[column.name] ?? '')}
            >
              {#if typeof unpackJson(row[column.name]) === 'object'}
                {#each highlightJson(row[column.name], false) as token, index (`${index}:${token.kind}`)}<span
                    class={`json-${token.kind}`}>{token.value}</span
                  >{/each}
              {:else}
                {formatCell(row[column.name], column.name)}
              {/if}
            </span>
          {/each}
        </button>
      {/snippet}
    </VirtualList>
  </div>
</div>

{#if selectedRow}
  <button
    class="detail-backdrop"
    type="button"
    aria-label="Close row details"
    onclick={closeDetails}
  ></button>
  <aside class="detail-panel" aria-label="Row details">
    <header class="detail-header">
      <div>
        <p>Query result</p>
        <h2>Row details</h2>
      </div>
      <button type="button" aria-label="Close row details" onclick={closeDetails}>×</button>
    </header>
    <div class="detail-tabs" role="tablist">
      <button
        class:active={detailView === 'fields'}
        type="button"
        onclick={() => (detailView = 'fields')}>Fields</button
      >
      <button
        class:active={detailView === 'json'}
        type="button"
        onclick={() => (detailView = 'json')}>JSON</button
      >
    </div>
    {#if detailView === 'fields'}
      <div class="detail-controls">
        <label>
          <span aria-hidden="true">⌕</span>
          <input
            bind:value={detailFilter}
            placeholder="Filter fields…"
            aria-label="Filter fields"
          />
        </label>
        <label class="empty-toggle">
          <input type="checkbox" bind:checked={showEmpty} />
          Show empty
        </label>
      </div>
      <div class="field-list">
        {#each filteredFields as column (column.name)}
          <div class="field">
            <div><strong>{column.name}</strong><small>{column.type}</small></div>
            <pre
              class:status-passed={statusKind(selectedRow[column.name]) === 'passed'}
              class:status-failed={statusKind(selectedRow[column.name]) === 'failed'}
              class:status-skipped={statusKind(selectedRow[column.name]) === 'skipped'}
              class:status-cancelled={statusKind(selectedRow[column.name]) ===
                'cancelled'}>{#if typeof unpackJson(selectedRow[column.name]) === 'object'}{#each highlightJson(selectedRow[column.name]) as token, index (`${index}:${token.kind}`)}<span
                    class={`json-${token.kind}`}>{token.value}</span
                  >{/each}{:else}{formatCell(selectedRow[column.name], column.name)}{/if}</pre>
          </div>
        {:else}
          <p class="no-fields">No matching populated fields.</p>
        {/each}
      </div>
    {:else}
      <pre
        class="json-view"
        aria-label="JSON row data">{#each highlightJson(unpackedRow) as token, index (`${index}:${token.kind}`)}<span
            class={`json-${token.kind}`}>{token.value}</span
          >{/each}</pre>
    {/if}
  </aside>
{/if}

<style>
  .results-tools {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    min-height: 38px;
    padding: 0 22px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    color: var(--subtle);
    font-size: 10px;
  }
  .column-control {
    position: relative;
  }
  .column-control > button {
    padding: 5px 9px;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--base);
    color: var(--text);
    font-size: 11px;
  }
  .column-control > button.active {
    border-color: var(--focus);
  }
  .column-control button small {
    color: var(--muted);
  }
  .column-menu {
    position: absolute;
    z-index: 20;
    top: calc(100% + 7px);
    right: 0;
    width: 310px;
    max-height: 360px;
    overflow: auto;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--base);
    box-shadow: 0 16px 40px rgb(31 34 28 / 18%);
  }
  .column-menu header {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    padding: 8px 9px 10px;
    border-bottom: 1px solid var(--border);
  }
  .column-menu header small {
    overflow: hidden;
    color: var(--muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .column-menu label {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 9px;
    padding: 7px 8px;
    color: var(--text);
    cursor: pointer;
  }
  .column-menu label:hover {
    background: var(--hover);
  }
  .column-menu label span {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .column-menu label small {
    color: var(--muted);
    font-size: 9px;
  }
  .query-results {
    flex: 1;
    width: 100%;
    min-height: 0;
    overflow-x: auto;
  }
  .query-columns,
  .query-row {
    display: grid;
    width: max-content;
    min-width: 100%;
  }
  .query-columns {
    position: sticky;
    top: 0;
    z-index: 1;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    font-size: 12px;
  }
  .query-columns > span,
  .query-row > span {
    box-sizing: border-box;
    overflow: hidden;
    padding: 10px 12px;
    border-right: 1px solid var(--border);
    text-align: left;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .query-columns > span {
    display: grid;
    gap: 2px;
    font-weight: 600;
  }
  .query-columns small {
    color: var(--muted);
    font-size: 10px;
    font-weight: normal;
  }
  .query-virtual-list {
    width: max-content;
    min-width: 100%;
    height: 100%;
    min-height: 300px;
  }
  .query-row {
    min-height: 42px;
    padding: 0;
    border: 0;
    border-bottom: 1px solid var(--border);
    background: var(--base);
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }
  .query-row:hover,
  .query-row.selected {
    background: var(--hover);
  }
  .query-row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }
  .query-row .empty {
    color: var(--muted);
  }
  .query-row .status-passed,
  .field .status-passed {
    color: var(--syntax-green);
    background: color-mix(in srgb, var(--syntax-green) 13%, transparent);
    font-weight: 750;
  }
  .query-row .status-failed,
  .field .status-failed {
    color: var(--love);
    background: color-mix(in srgb, var(--love) 13%, transparent);
    font-weight: 750;
  }
  .query-row .status-skipped,
  .field .status-skipped {
    color: var(--gold);
    background: color-mix(in srgb, var(--gold) 13%, transparent);
    font-weight: 750;
  }
  .query-row .status-cancelled,
  .field .status-cancelled {
    color: var(--subtle);
    background: color-mix(in srgb, var(--muted) 13%, transparent);
    font-weight: 750;
  }
  .detail-backdrop {
    position: fixed;
    z-index: 40;
    inset: 0;
    border: 0;
    background: rgb(21 24 20 / 24%);
  }
  .detail-panel {
    position: fixed;
    z-index: 41;
    top: 0;
    right: 0;
    bottom: 0;
    display: flex;
    width: min(680px, calc(100vw - 230px));
    min-width: 360px;
    flex-direction: column;
    border-left: 1px solid var(--border);
    background: var(--base);
    color: var(--text);
    box-shadow: -18px 0 50px rgb(19 22 18 / 18%);
    animation: slide-in 150ms ease-out;
  }
  .detail-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 70px;
    padding: 0 20px;
    border-bottom: 1px solid var(--border);
    background: var(--base);
  }
  .detail-header p {
    margin: 0 0 2px;
    color: var(--muted);
    font-size: 9px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  .detail-header h2 {
    margin: 0;
    font-size: 16px;
  }
  .detail-header button {
    width: 34px;
    height: 34px;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--base);
    color: var(--text);
    font-size: 20px;
  }
  .detail-tabs {
    display: flex;
    padding: 12px 20px 0;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .detail-tabs button {
    padding: 9px 16px;
    border: 1px solid transparent;
    border-bottom: 0;
    background: transparent;
    color: var(--subtle);
  }
  .detail-tabs button.active {
    border-color: var(--border);
    background: var(--base);
    color: var(--text);
  }
  .detail-controls {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    background: var(--base);
  }
  .detail-controls > label:first-child {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 3px;
  }
  .detail-controls input {
    min-width: 0;
    border: 0;
    outline: 0;
    background: transparent;
  }
  .empty-toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--subtle);
    font-size: 11px;
    white-space: nowrap;
  }
  .field-list {
    flex: 1;
    overflow: auto;
    padding: 14px 20px 30px;
  }
  .field {
    display: grid;
    grid-template-columns: minmax(140px, 34%) minmax(0, 1fr);
    border: 1px solid var(--border);
    border-bottom: 0;
    background: var(--base);
  }
  .field:last-child {
    border-bottom: 1px solid var(--border);
  }
  .field > div {
    display: grid;
    align-content: start;
    gap: 3px;
    padding: 12px;
    border-right: 1px solid var(--border);
  }
  .field strong {
    overflow-wrap: anywhere;
    color: var(--syntax-blue);
    font-size: 11px;
  }
  .field small {
    color: var(--muted);
    font-size: 9px;
    text-transform: uppercase;
  }
  .field pre {
    min-width: 0;
    margin: 0;
    padding: 12px;
    overflow: auto;
    color: var(--text);
    font: inherit;
    font-size: 11px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .json-view {
    flex: 1;
    margin: 14px 20px 24px;
    overflow: auto;
    padding: 18px 20px;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--base);
    color: var(--text);
    font: inherit;
    font-size: 12px;
    line-height: 1.7;
    tab-size: 2;
  }
  .json-key {
    color: var(--syntax-blue);
    font-weight: 650;
  }
  .json-string {
    color: var(--syntax-green);
  }
  .json-number {
    color: var(--syntax-peach);
  }
  .json-boolean {
    color: var(--syntax-mauve);
    font-weight: 650;
  }
  .json-null {
    color: var(--syntax-overlay);
    font-style: italic;
  }
  .json-punctuation {
    color: var(--subtle);
  }
  :global(html.dark) .results-tools,
  :global(html.dark) .query-columns {
    border-color: var(--border);
    background: var(--surface);
    color: var(--subtle);
  }
  :global(html.dark) .column-control > button,
  :global(html.dark) .column-menu,
  :global(html.dark) .detail-header,
  :global(html.dark) .detail-controls,
  :global(html.dark) .field,
  :global(html.dark) .json-view {
    border-color: var(--border);
    background: var(--base);
    color: var(--text);
  }
  :global(html.dark) .column-menu header,
  :global(html.dark) .query-columns > span,
  :global(html.dark) .query-row > span,
  :global(html.dark) .detail-tabs,
  :global(html.dark) .field,
  :global(html.dark) .field > div {
    border-color: var(--border);
  }
  :global(html.dark) .column-menu label {
    color: var(--text);
  }
  :global(html.dark) .column-menu label:hover,
  :global(html.dark) .query-row:hover,
  :global(html.dark) .query-row.selected {
    background: var(--hover);
  }
  :global(html.dark) .query-row {
    border-color: var(--border);
    background: var(--base);
    color: var(--text);
  }
  :global(html.dark) .detail-panel {
    border-color: var(--border);
    background: var(--base);
  }
  :global(html.dark) .detail-tabs {
    background: var(--surface);
  }
  :global(html.dark) .detail-tabs button.active {
    border-color: var(--border);
    background: var(--base);
    color: var(--text);
  }
  :global(html.dark) .json-key {
    color: var(--syntax-blue);
  }
  :global(html.dark) .json-string {
    color: var(--syntax-green);
  }
  :global(html.dark) .query-row .status-passed,
  :global(html.dark) .field .status-passed {
    color: var(--syntax-green);
    background: rgb(166 227 161 / 12%);
  }
  :global(html.dark) .query-row .status-failed,
  :global(html.dark) .field .status-failed {
    color: #f38ba8;
    background: rgb(243 139 168 / 12%);
  }
  :global(html.dark) .query-row .status-skipped,
  :global(html.dark) .field .status-skipped {
    color: var(--syntax-peach);
    background: rgb(250 179 135 / 12%);
  }
  :global(html.dark) .query-row .status-cancelled,
  :global(html.dark) .field .status-cancelled {
    color: var(--syntax-overlay);
    background: rgb(166 173 200 / 10%);
  }
  :global(html.dark) .json-number {
    color: var(--syntax-peach);
  }
  :global(html.dark) .json-boolean {
    color: var(--syntax-mauve);
  }
  :global(html.dark) .json-null,
  :global(html.dark) .json-punctuation {
    color: var(--syntax-overlay);
  }
  .no-fields {
    color: var(--subtle);
    text-align: center;
  }
  @keyframes slide-in {
    from {
      transform: translateX(30px);
      opacity: 0.5;
    }
  }
  @media (max-width: 760px) {
    .detail-panel {
      width: 100vw;
      min-width: 0;
    }
    .results-tools {
      padding: 0 16px;
    }
  }
</style>
