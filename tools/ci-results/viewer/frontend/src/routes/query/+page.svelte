<script lang="ts">
  import Prism from 'prismjs';
  import 'prismjs/components/prism-sql';
  import 'prismjs/themes/prism.css';

  let query = $state('');
  let schema = $state('');
  let errorGetSchema = $state('');
  let loadingSchema = $state(false);
  let schemaOpen = $state(false);
  let schemaWidth = $state(50);
  let workspace: HTMLElement;

  const highlightedSchema = $derived(
    schema ? Prism.highlight(schema, Prism.languages.sql, 'sql') : ''
  );

  async function openSchema() {
    schemaOpen = true;
    if (!schema) await fetchSchema();
  }

  async function fetchSchema() {
    if (loadingSchema) return;
    loadingSchema = true;
    errorGetSchema = '';
    try {
      const response = await fetch('/api/v1/schema');
      if (!response.ok) throw new Error(`Request failed (${response.status})`);
      schema = await response.json();
    } catch (cause) {
      errorGetSchema = cause instanceof Error ? cause.message : 'Request failed';
    } finally {
      loadingSchema = false;
    }
  }

  function startResize(event: PointerEvent) {
    event.preventDefault();
    const resize = (move: PointerEvent) => {
      const bounds = workspace.getBoundingClientRect();
      schemaWidth = Math.min(
        70,
        Math.max(30, ((bounds.right - move.clientX) / bounds.width) * 100)
      );
    };
    const stop = () => {
      window.removeEventListener('pointermove', resize);
      window.removeEventListener('pointerup', stop);
    };
    window.addEventListener('pointermove', resize);
    window.addEventListener('pointerup', stop);
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
      <div>
        <p class="eyebrow">CI results / explorer</p>
        <h1 id="query-builder-title">Query builder</h1>
        <p class="lede">Build a focused view of the ingested CI database.</p>
      </div>
    </header>

    <div class="builder-body">
      <label for="query">Filter expression</label>
      <textarea id="query" bind:value={query} placeholder="status = passed and branch = main"
      ></textarea>
      <div class="builder-footer">
        <div class="workspace-tools">
          <span>Local SQLite</span>
          <button class="schema-trigger" onclick={openSchema} disabled={loadingSchema}>
            <span aria-hidden="true">▧</span>
            {loadingSchema ? 'Loading…' : schemaOpen ? 'Schema open' : 'Schema'}
          </button>
        </div>
        <button class="run-query" disabled>Run query <span aria-hidden="true">↵</span></button>
      </div>
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
    <aside class="schema-panel" aria-label="Database schema">
      <div class="schema-actions">
        <button
          class="icon-button"
          aria-label="Reload schema"
          title="Reload schema"
          onclick={fetchSchema}
          disabled={loadingSchema}>↻</button
        >
        <button
          class="icon-button"
          aria-label="Close schema"
          title="Close schema"
          onclick={() => (schemaOpen = false)}>×</button
        >
      </div>
      <div class="schema-content">
        {#if loadingSchema}
          <p class="schema-message">Loading schema…</p>
        {:else if errorGetSchema}
          <div class="schema-message">
            <p class="schema-error">{errorGetSchema}</p>
            <button onclick={fetchSchema}>Try again</button>
          </div>
        {:else if schema}
          <!-- The API serves database DDL, highlighted by Prism before it is rendered. -->
          <!-- eslint-disable-next-line svelte/no-at-html-tags -->
          <pre class="language-sql"><code class="language-sql">{@html highlightedSchema}</code
            ></pre>
        {/if}
      </div>
    </aside>
  {/if}
</div>

<style>
  .query-workspace {
    --paper: #fbfaf7;
    --ink: #20211e;
    --muted: #71746b;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    height: 100vh;
    overflow: hidden;
    background: var(--paper);
  }
  .query-workspace.split {
    grid-template-columns: minmax(0, calc(100% - var(--schema-width))) 10px minmax(
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
    padding: clamp(28px, 5vw, 72px);
  }
  .builder-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    padding-bottom: 34px;
    border-bottom: 1px solid #d8d7d1;
  }
  .eyebrow {
    margin: 0 0 8px;
    color: #777a70;
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  h1 {
    margin: 0;
    color: var(--ink);
    letter-spacing: -0.045em;
  }
  h1 {
    font-size: clamp(32px, 4vw, 52px);
    line-height: 0.95;
  }
  .lede {
    margin: 13px 0 0;
    color: var(--muted);
  }
  button {
    font: inherit;
    cursor: pointer;
  }
  .run-query {
    border: 1px solid #20211e;
    background: #20211e;
    color: #fff;
    padding: 10px 14px;
    font-weight: 700;
    white-space: nowrap;
  }
  .run-query:hover:not(:disabled) {
    background: #3c4038;
  }
  .schema-trigger {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 0;
    border: 0;
    border-bottom: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font-size: 12px;
  }
  .schema-trigger:hover:not(:disabled) {
    border-bottom-color: var(--ink);
    background: transparent;
    color: var(--ink);
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .builder-body {
    display: grid;
    gap: 12px;
    width: min(760px, 100%);
    margin: auto 0;
  }
  label {
    color: #4d5049;
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  textarea {
    min-height: 220px;
    resize: vertical;
    padding: 22px;
    border: 1px solid #bdbeb6;
    border-radius: 2px;
    background: #fff;
    color: var(--ink);
    font:
      16px/1.65 ui-monospace,
      SFMono-Regular,
      Menlo,
      Consolas,
      monospace;
    outline: none;
  }
  textarea:focus {
    border-color: #20211e;
    box-shadow: 4px 4px 0 #deded7;
  }
  .builder-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    color: var(--muted);
    font-size: 12px;
  }
  .workspace-tools {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .workspace-tools > span::after {
    content: '·';
    margin-left: 12px;
    color: #bbbcb5;
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
  pre {
    margin: 0;
    padding: 16px;
    overflow: auto;
    border: 1px solid #dadad3;
    background: #fff;
    font-size: 12px;
    line-height: 1.55;
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
      min-height: 68vh;
    }
    .schema-panel {
      border-left: 0;
      border-top: 1px solid #d8d7d1;
      min-height: 48vh;
    }
    .builder-header {
      flex-direction: column;
    }
    .builder-footer {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
