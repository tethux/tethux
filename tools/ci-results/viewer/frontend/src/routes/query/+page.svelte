<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import type { PageData } from './$types';
  import type { SchemaInfo, Suggestion } from '$lib/schema_types';
  import type { ExecuteQueryResponse } from '$lib/api/types';
  import { executeQuery } from '$lib/api/querys';
  import QueryResults from '$lib/components/QueryResults.svelte';
  import { getSuggestions, querySource, tokenizeSql } from './sql';

  let { data }: { data: PageData } = $props();

  let query = $state('');
  let queryInput: HTMLInputElement;
  let queryHighlight = $state<HTMLDivElement>();

  let suggestions = $state<Suggestion[]>([]);
  let selectedSuggestionIndex = $state(0);
  let suggestionsOpen = $state(false);

  let schemaOpen = $state(false);
  let schemaWidth = $state(50);
  let workspace: HTMLElement;

  const schemaInfo: SchemaInfo = $derived(data.schemaInfo ?? { objects: [] });

  const schemaError = $derived(data.error);
  const highlightedQuery = $derived(tokenizeSql(query));

  function handleInput(): void {
    const cursor = queryInput.selectionStart ?? query.length;
    const sqlBeforeCursor = query.slice(0, cursor);

    suggestions = getSuggestions(sqlBeforeCursor, schemaInfo);
    selectedSuggestionIndex = 0;
    suggestionsOpen = suggestions.length > 0;
  }

  function syncHighlightScroll(): void {
    if (queryHighlight) queryHighlight.scrollLeft = queryInput.scrollLeft;
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (!suggestionsOpen || suggestions.length === 0) {
      return;
    }

    if (event.key === 'ArrowDown') {
      event.preventDefault();

      selectedSuggestionIndex = (selectedSuggestionIndex + 1) % suggestions.length;
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();

      selectedSuggestionIndex =
        (selectedSuggestionIndex - 1 + suggestions.length) % suggestions.length;
    } else if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault();

      const suggestion = suggestions[selectedSuggestionIndex];

      if (suggestion) {
        insertSuggestion(suggestion);
      }
    } else if (event.key === 'Escape') {
      suggestionsOpen = false;
    }
  }

  function insertSuggestion(suggestion: Suggestion): void {
    const cursor = queryInput.selectionStart ?? query.length;
    const beforeCursor = query.slice(0, cursor);
    const afterCursor = query.slice(cursor);

    const currentWord = beforeCursor.match(/[A-Za-z0-9_]*$/)?.[0] ?? '';

    const replacementStart = beforeCursor.length - currentWord.length;

    query = beforeCursor.slice(0, replacementStart) + suggestion.insertText + afterCursor;

    const nextCursor = replacementStart + suggestion.insertText.length;

    suggestionsOpen = false;

    requestAnimationFrame(() => {
      queryInput.focus();
      queryInput.setSelectionRange(nextCursor, nextCursor);
    });
  }

  function startResize(event: PointerEvent): void {
    event.preventDefault();

    const resize = (move: PointerEvent): void => {
      const bounds = workspace.getBoundingClientRect();

      schemaWidth = Math.min(
        70,
        Math.max(30, ((bounds.right - move.clientX) / bounds.width) * 100)
      );
    };

    const stop = (): void => {
      window.removeEventListener('pointermove', resize);
      window.removeEventListener('pointerup', stop);
    };

    window.addEventListener('pointermove', resize);
    window.addEventListener('pointerup', stop);
  }

  let result = $state<ExecuteQueryResponse | null>(null);
  let error = $state<string | null>(null);

  async function runQuery() {
    error = null;

    await executeQuery(fetch, query).match(
      (response) => {
        result = response;
      },
      (apiError) => {
        error = apiError.message;
      }
    );
  }
</script>

<svelte:head><title>Query builder · CI results</title></svelte:head>

<div
  class:split={schemaOpen}
  class="query-workspace"
  bind:this={workspace}
  style:--schema-width={`${schemaWidth}%`}
>
  <section class="builder" aria-labelledby="query-builder-title">
    <header class="builder-header">
      <div class="title-lockup">
        <div>
          <p class="eyebrow">CI results</p>
          <h1 id="query-builder-title">Explorer</h1>
        </div>
      </div>
    </header>

    <div class="builder-body">
      <div class="query-toolbar">
        <div class="view-tabs" aria-label="Explorer view">
          <button
            class="view-tab"
            class:active={!schemaOpen}
            type="button"
            onclick={() => (schemaOpen = false)}
            aria-pressed={!schemaOpen}
          >
            <span aria-hidden="true">⌘</span> Query
          </button>
          <button
            class="view-tab"
            class:active={schemaOpen}
            type="button"
            onclick={() => (schemaOpen = true)}
            aria-pressed={schemaOpen}
            aria-controls="schema-panel"
          >
            <span aria-hidden="true">▧</span>
            Schema
          </button>
        </div>
        <button class="run-query" disabled={!query.trim()} onclick={runQuery}>
          <span aria-hidden="true">▷</span> Run query
          <kbd>↵</kbd>
        </button>
      </div>

      <div class="query-bar">
        <span class="prompt" aria-hidden="true">›</span>
        <div class="query-editor">
          {#if query}
            <div class="query-highlight" bind:this={queryHighlight} aria-hidden="true">
              {#each highlightedQuery as token, index (`${index}:${token.value}`)}
                <span
                  class:token-keyword={token.kind === 'keyword'}
                  class:token-string={token.kind === 'string'}
                  class:token-number={token.kind === 'number'}
                  class:token-operator={token.kind === 'operator'}>{token.value}</span
                >
              {/each}
            </div>
          {/if}
          <input
            id="query"
            bind:this={queryInput}
            bind:value={query}
            aria-label="SQL query"
            placeholder="SELECT * FROM runs WHERE status = 'passed'"
            autocomplete="off"
            spellcheck="false"
            oninput={handleInput}
            onkeydown={handleKeydown}
            onscroll={syncHighlightScroll}
            onblur={() => setTimeout(() => (suggestionsOpen = false), 120)}
          />

          {#if suggestionsOpen}
            <div class="suggestions" role="listbox" aria-label="SQL suggestions">
              {#each suggestions as suggestion, index (`${suggestion.kind}:${suggestion.label}`)}
                <button
                  type="button"
                  role="option"
                  aria-selected={index === selectedSuggestionIndex}
                  class:selected={index === selectedSuggestionIndex}
                  onmousedown={(event) => event.preventDefault()}
                  onclick={() => insertSuggestion(suggestion)}
                >
                  <span class="suggestion-kind">{suggestion.kind}</span>
                  <strong>{suggestion.label}</strong>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      </div>

      <div class="results-heading">
        <div>
          <strong>Query results</strong>
          <span>Run a query to explore ingested test data</span>
        </div>

        <span class="result-count">
          {result ? `${result.row_count} rows` : '— rows'}
        </span>
      </div>

      {#if error}
        <div class="empty-state">
          <pre>{error}</pre>
        </div>
      {:else if result && result.rows.length > 0}
        {@const queryResult = result}

        <QueryResults result={queryResult} source={querySource(query)} />
      {:else}
        <div class="empty-state" aria-label="No query results"></div>
      {/if}
    </div>
  </section>

  {#if schemaOpen}
    <div
      class="resize-handle"
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize schema pane"
      onpointerdown={startResize}
    ></div>
    <aside id="schema-panel" class="schema-panel" aria-label="Database schema">
      <div class="schema-actions">
        <button
          class="icon-button"
          type="button"
          aria-label="Reload schema"
          title="Reload schema"
          onclick={() => invalidateAll()}>↻</button
        >
        <button
          class="icon-button"
          type="button"
          aria-label="Close schema"
          title="Close schema"
          onclick={() => (schemaOpen = false)}>×</button
        >
      </div>
      <div class="schema-content">
        {#if schemaError}
          <div class="schema-message">
            <p class="schema-error">{schemaError}</p>
            <button type="button" onclick={() => invalidateAll()}>Try again</button>
          </div>
        {:else if schemaInfo.objects.length}
          <div class="schema-list">
            {#each schemaInfo.objects as object (object.name)}
              <section class="schema-object">
                <header>
                  <strong>{object.name}</strong>
                  <span>{object.kind}</span>
                </header>
                <ul>
                  {#each object.columns as column (column.name)}
                    <li>
                      <span>{column.name}</span>
                      <small>{column.type || 'unknown'}</small>
                      {#if column.primaryKey}<abbr title="Primary key">PK</abbr>{/if}
                    </li>
                  {/each}
                </ul>
              </section>
            {/each}
          </div>
        {:else}
          <p class="schema-message">No schema objects found.</p>
        {/if}
      </div>
    </aside>
  {/if}
</div>

<style>
  .query-workspace {
    --paper: #f8f8f5;
    --ink: #242621;
    --muted: #74786e;
    --line: #d8d9d2;
    --accent: #315f4a;
    --run-text: #fff;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    height: 100vh;
    overflow: hidden;
    background: var(--paper);
  }
  .query-workspace.split {
    grid-template-columns: minmax(0, calc(100% - var(--schema-width) - 10px)) 10px minmax(
        0,
        var(--schema-width)
      );
  }
  .builder,
  .schema-panel {
    min-width: 0;
    min-height: 0;
  }
  .builder {
    display: flex;
    flex-direction: column;
  }
  .builder-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    min-height: 68px;
    padding: 0 clamp(22px, 3vw, 42px);
    border-bottom: 1px solid var(--line);
    background: #fdfdfb;
  }
  .title-lockup {
    display: flex;
    align-items: center;
  }
  .eyebrow {
    margin: 0 0 1px;
    color: #777a70;
    font-size: 9px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  h1 {
    margin: 0;
    color: var(--ink);
    font-size: 17px;
    line-height: 1.1;
    letter-spacing: -0.02em;
  }
  button {
    font: inherit;
    cursor: pointer;
  }
  .run-query {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    min-height: 38px;
    padding: 0 12px;
    border: 1px solid var(--syntax-blue);
    border-radius: 3px;
    background: var(--syntax-blue);
    color: var(--run-text);
    font-size: 12px;
    font-weight: 650;
    white-space: nowrap;
  }
  .run-query:hover:not(:disabled) {
    border-color: color-mix(in srgb, var(--syntax-blue) 82%, #000);
    background: color-mix(in srgb, var(--syntax-blue) 82%, #000);
  }
  .run-query kbd {
    padding: 1px 5px;
    border: 1px solid rgb(255 255 255 / 25%);
    border-radius: 3px;
    color: rgb(255 255 255 / 75%);
    font: inherit;
  }
  .view-tabs {
    display: flex;
    align-self: stretch;
    gap: 4px;
  }
  .view-tab {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 0 12px;
    border: 0;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
  }
  .view-tab:hover:not(:disabled),
  .view-tab.active {
    color: var(--ink);
  }
  .view-tab.active::after {
    position: absolute;
    right: 10px;
    bottom: -13px;
    left: 10px;
    height: 2px;
    background: var(--accent);
    content: '';
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .builder-body {
    display: flex;
    flex: 1;
    min-height: 0;
    flex-direction: column;
    width: 100%;
  }
  .query-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 62px;
    padding: 11px clamp(22px, 3vw, 42px);
    border-bottom: 1px solid var(--line);
    background: #f2f2ee;
  }
  .query-bar {
    display: flex;
    align-items: center;
    min-height: 58px;
    margin: 14px clamp(22px, 3vw, 42px);
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--base);
    box-shadow: inset 0 1px 0 rgb(24 27 21 / 3%);
  }
  .query-bar:focus-within {
    border-color: #547161;
    box-shadow: 0 0 0 2px rgb(49 95 74 / 10%);
  }
  .prompt {
    padding-left: 16px;
    color: var(--accent);
    font-size: 22px;
  }
  .query-editor {
    position: relative;
    flex: 1;
    min-width: 0;
    height: 56px;
  }
  .query-editor input,
  .query-highlight {
    position: absolute;
    inset: 0;
    overflow: hidden;
    padding: 17px 12px;
    border: 0;
    font: inherit;
    font-size: 14px;
    line-height: 22px;
    text-align: left;
    white-space: pre;
  }
  .query-editor input {
    z-index: 1;
    width: 100%;
    border: 0;
    outline: 0;
    background: transparent;
    color: transparent;
    caret-color: var(--ink);
  }
  .query-editor input::selection {
    background: rgb(49 95 74 / 20%);
    color: transparent;
  }
  .query-editor input::placeholder {
    color: #96998f;
  }
  .query-highlight {
    z-index: 0;
    color: var(--ink);
    pointer-events: none;
  }
  .token-keyword {
    color: #9a651c;
    font-weight: 650;
  }
  .token-string {
    color: #21845d;
  }
  .token-number {
    color: #315fa0;
  }
  .token-operator {
    color: #a04636;
  }
  .suggestions {
    position: absolute;
    z-index: 5;
    top: calc(100% - 2px);
    left: 10px;
    width: max-content;
    min-width: 180px;
    max-width: min(300px, calc(100vw - 330px));
    max-height: 260px;
    overflow-y: auto;
    padding: 5px;
    border: 1px solid #c5c7be;
    border-radius: 3px;
    background: #fff;
    box-shadow: 0 12px 30px rgb(31 34 28 / 14%);
  }
  .suggestions button {
    display: grid;
    grid-template-columns: 54px minmax(0, auto);
    align-items: center;
    width: 100%;
    gap: 9px;
    padding: 7px 9px;
    border: 0;
    background: transparent;
    color: var(--ink);
    text-align: left;
  }
  .suggestions button:hover,
  .suggestions button.selected {
    background: var(--overlay);
    color: var(--text);
  }
  .suggestions button.selected {
    box-shadow: inset 3px 0 0 var(--focus);
  }
  .suggestion-kind {
    color: var(--syntax-mauve);
    font-size: 9px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .suggestions strong {
    overflow: hidden;
    color: var(--text);
    font-size: 12px;
    font-weight: 700;
    text-overflow: ellipsis;
  }
  .results-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 52px;
    padding: 0 clamp(22px, 3vw, 42px);
    border-top: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
    background: #fdfdfb;
  }
  .results-heading div {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  .results-heading strong {
    font-size: 12px;
    font-weight: 650;
  }
  .results-heading span {
    color: var(--muted);
    font-size: 11px;
  }
  .empty-state {
    display: grid;
    flex: 1;
    min-height: 220px;
    place-content: center;
    justify-items: center;
    color: var(--muted);
    text-align: center;
  }
  .resize-handle {
    position: relative;
    z-index: 2;
    cursor: col-resize;
    background: #d8d7d1;
  }
  .resize-handle:hover,
  .resize-handle:active {
    background: #20211e;
  }
  .schema-panel {
    display: flex;
    flex-direction: column;
    position: relative;
    border-left: 1px solid #d8d7d1;
    background: #f0f0eb;
  }
  .schema-actions {
    position: absolute;
    z-index: 1;
    top: 14px;
    right: 14px;
    display: flex;
    gap: 6px;
  }
  .icon-button {
    width: 32px;
    height: 32px;
    border: 1px solid #bfc0b8;
    background: #fbfaf7;
    color: var(--ink);
    font-size: 20px;
    line-height: 1;
  }
  .icon-button:hover:not(:disabled) {
    border-color: var(--ink);
  }
  .schema-content {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 14px;
  }
  .schema-list {
    display: grid;
    gap: 10px;
    padding-top: 48px;
  }
  .schema-object {
    overflow: hidden;
    border: 1px solid #d2d3cc;
    border-radius: 3px;
    background: #fff;
  }
  .schema-object header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 12px;
    border-bottom: 1px solid #dedfd9;
    background: #f7f7f3;
  }
  .schema-object header strong {
    color: var(--ink);
    font-size: 12px;
  }
  .schema-object header span {
    color: var(--muted);
    font-size: 9px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .schema-object ul {
    margin: 0;
    padding: 4px 0;
    list-style: none;
  }
  .schema-object li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 9px;
    padding: 6px 12px;
    color: #4a4d46;
    font-size: 11px;
  }
  .schema-object li:hover {
    background: #f7f8f4;
  }
  .schema-object li small {
    color: #8a8e84;
    font-size: 9px;
    text-transform: uppercase;
  }
  .schema-object abbr {
    border: 0;
    color: #966c28;
    font-size: 8px;
    text-decoration: none;
  }
  .schema-message {
    display: grid;
    justify-items: start;
    gap: 12px;
    color: var(--muted);
  }
  .schema-message p {
    margin: 0;
  }
  .schema-error {
    color: #a3362a;
  }
  .schema-message button {
    border: 1px solid #bfc0b8;
    background: #fff;
    padding: 8px 12px;
  }
  .empty-state pre {
    overflow-x: auto;
    color: #8a3028;
    white-space: pre-wrap;
  }
  :global(html.dark) .query-workspace {
    --paper: var(--base);
    --ink: var(--text);
    --muted: var(--subtle);
    --line: var(--border);
    --accent: #89b4fa;
    --run-text: #1e1e2e;
  }
  :global(html.dark) .builder-header,
  :global(html.dark) .results-heading,
  :global(html.dark) .query-bar,
  :global(html.dark) .suggestions,
  :global(html.dark) .schema-object,
  :global(html.dark) .icon-button,
  :global(html.dark) .schema-message button {
    border-color: var(--border);
    background: var(--base);
    color: var(--text);
  }
  :global(html.dark) .query-toolbar,
  :global(html.dark) .schema-panel,
  :global(html.dark) .schema-object header {
    border-color: var(--border);
    background: var(--surface);
    color: var(--text);
  }
  :global(html.dark) .query-bar:focus-within {
    border-color: var(--focus);
    box-shadow: 0 0 0 2px rgb(245 194 231 / 14%);
  }
  :global(html.dark) .query-highlight {
    color: var(--text);
  }
  :global(html.dark) .token-keyword {
    color: var(--syntax-mauve);
  }
  :global(html.dark) .token-string {
    color: var(--syntax-green);
  }
  :global(html.dark) .token-number {
    color: var(--syntax-peach);
  }
  :global(html.dark) .token-operator {
    color: var(--syntax-overlay);
  }
  :global(html.dark) .suggestions button {
    color: var(--text);
  }
  :global(html.dark) .suggestions button:hover,
  :global(html.dark) .suggestions button.selected,
  :global(html.dark) .schema-object li:hover {
    background: var(--overlay);
  }
  :global(html.dark) .run-query {
    border-color: var(--syntax-blue);
    background: var(--syntax-blue);
    color: var(--run-text);
  }
  :global(html.dark) .resize-handle {
    background: var(--border);
  }
  @media (max-width: 760px) {
    .query-workspace.split {
      grid-template-columns: 1fr;
      height: auto;
      overflow: visible;
    }
    .resize-handle {
      display: none;
    }
    .builder,
    .schema-panel {
      min-height: auto;
    }
    .builder {
      min-height: 72vh;
    }
    .schema-panel {
      border-left: 0;
      border-top: 1px solid #d8d7d1;
      min-height: 48vh;
    }
    .query-toolbar,
    .builder-header,
    .results-heading {
      padding-right: 16px;
      padding-left: 16px;
    }
    .query-bar {
      margin-right: 16px;
      margin-left: 16px;
    }
    .results-heading div span {
      display: none;
    }
    .run-query kbd {
      display: none;
    }
  }
</style>
