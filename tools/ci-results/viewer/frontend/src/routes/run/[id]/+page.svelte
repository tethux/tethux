<script lang="ts">
  import { resolve } from '$app/paths';
  import type { PageData } from './$types';
  import type { ArchiveFile, NullString, TestResult } from '$lib/api/types';
  import { getArtifact } from '$lib/api/artifacts';
  import CodePreview from '$lib/components/CodePreview.svelte';
  import LogPreview from '$lib/components/LogPreview.svelte';
  import LogSearch from '$lib/components/LogSearch.svelte';

  let { data }: { data: PageData } = $props();
  let query = $state('');
  let showPassed = $state(false);
  let selected = $state<TestResult | null>(null);
  let openFile = $state<ArchiveFile | null>(null);
  let fileContent = $state<unknown>(null);
  let fileLoading = $state(false);
  let fileRawURL = $state('');

  const value = (entry: NullString | null | undefined) => (entry?.Valid ? entry.String : null);
  const failures = $derived(
    (data.detail?.tests ?? []).filter((test) => test.status === 'failed' || test.status === 'error')
  );
  const visibleTests = $derived(
    (showPassed ? (data.detail?.tests ?? []) : failures).filter((test) => {
      const needle = query.trim().toLowerCase();
      return !needle || `${test.test_name} ${test.test_key}`.toLowerCase().includes(needle);
    })
  );
  const logFiles = $derived(
    (data.detail?.files ?? []).filter(
      (file) => file.file_type === 'log' || /(?:log|txt|jsonl?)$/i.test(file.archive_path)
    )
  );

  $effect(() => {
    if (!selected || !visibleTests.includes(selected)) selected = visibleTests[0] ?? null;
  });

  const duration = (ms: number) =>
    ms >= 60_000 ? `${(ms / 60_000).toFixed(1)}m` : `${(ms / 1000).toFixed(2)}s`;
  const testDuration = (test: TestResult) =>
    test.duration_ms.Valid ? duration(test.duration_ms.Int64) : 'no duration';
  const bytes = (size: number) =>
    size >= 1_048_576 ? `${(size / 1_048_576).toFixed(1)} MB` : `${Math.ceil(size / 1024)} KB`;

  async function viewFile(file: ArchiveFile): Promise<void> {
    openFile = file;
    fileContent = null;
    fileRawURL = '';
    fileLoading = true;
    const result = await getArtifact(fetch, file.id);
    result.match(
      (payload) => {
        fileContent = payload.preview;
        fileRawURL = payload.raw_url;
      },
      (error) => {
        fileContent = error.message;
      }
    );
    fileLoading = false;
  }
</script>

<svelte:head
  ><title>{data.detail ? `Run ${data.detail.run.id}` : 'Run'} · CI results</title></svelte:head
>

{#if data.error}
  <p class="error">Could not load run: {data.error}</p>
{:else if data.detail}
  {@const run = data.detail.run}
  <p class="crumb"><a href={resolve('/runs')}>Runs</a> / {run.id}</p>
  <header class="run-header">
    <div class:failed={run.status !== 'passed'} class="status">{run.status}</div>
    <div>
      <h1>{value(run.workflow) ?? 'CI run'}</h1>
      <p>
        {run.commit_sha.slice(0, 12)} · {value(run.branch) ?? 'detached'} · {value(run.job) ??
          value(run.source_provider) ??
          'local'}
      </p>
    </div>
    <dl>
      <div>
        <dt>Duration</dt>
        <dd>{duration(run.duration_ms)}</dd>
      </div>
      <div>
        <dt>Results</dt>
        <dd>{run.passed_count}/{run.total_count} passed</dd>
      </div>
    </dl>
  </header>

  <section class="tests" aria-labelledby="tests-heading">
    <header>
      <div>
        <p class="eyebrow">Results</p>
        <h2 id="tests-heading">{failures.length} failures</h2>
      </div>
      <div class="controls">
        <label
          ><span class="sr-only">Filter tests</span><input
            bind:value={query}
            placeholder="Filter tests"
          /></label
        >
        <label class="toggle"
          ><input type="checkbox" bind:checked={showPassed} /> Show passing</label
        >
      </div>
    </header>
    <div class="test-layout">
      <div class="test-list">
        {#each visibleTests as test (test.id)}
          <button
            class:active={selected?.id === test.id}
            type="button"
            onclick={() => (selected = test)}
          >
            <span class:passed={test.status === 'passed'} class="test-status"
              >{test.status === 'passed' ? '✓' : '×'}</span
            >
            <span><strong>{test.test_name}</strong><small>{test.test_key}</small></span>
            <small>{testDuration(test)}</small>
          </button>
        {:else}
          <p class="empty">No matching tests.</p>
        {/each}
      </div>
      <article class="test-detail">
        {#if selected}
          <div class="detail-heading">
            <span class:passed={selected.status === 'passed'}>{selected.status}</span><small
              >attempt {selected.attempt}</small
            >
          </div>
          <h3>{selected.test_name}</h3>
          {#if value(selected.message)}<pre>{value(selected.message)}</pre>{/if}
          {#if value(selected.stack_trace)}<pre>{value(selected.stack_trace)}</pre>{/if}
          {#if !value(selected.message) && !value(selected.stack_trace)}<p class="empty">
              No diagnostic text was recorded.
            </p>{/if}
        {:else}
          <p class="empty">Select a test to inspect it.</p>
        {/if}
      </article>
    </div>
  </section>

  {#if logFiles.length}
    <details class="logs">
      <summary>Search {logFiles.length} logs</summary>
      <LogSearch files={logFiles} />
    </details>
  {/if}

  <section class="files" aria-labelledby="files-heading">
    <header>
      <div>
        <p class="eyebrow">Evidence</p>
        <h2 id="files-heading">Artifacts</h2>
      </div>
    </header>
    <div class="file-list">
      {#each data.detail.files as file (file.id)}
        <button type="button" onclick={() => viewFile(file)}>
          <span><strong>{file.archive_path}</strong><small>{file.media_type}</small></span>
          <small>{bytes(file.size_bytes)}</small>
        </button>
      {:else}<p class="empty">No artifacts recorded.</p>{/each}
    </div>
  </section>

  {#if openFile}
    <div
      class="modal-backdrop"
      role="presentation"
      onclick={(event) => {
        if (event.target === event.currentTarget) openFile = null;
      }}
    >
      <div class="modal" role="dialog" aria-modal="true" aria-labelledby="file-title">
        <header>
          <div>
            <h2 id="file-title">{openFile.archive_path}</h2>
            <small>{openFile.media_type} · {bytes(openFile.size_bytes)}</small>
          </div>
          <button type="button" onclick={() => (openFile = null)} aria-label="Close">×</button>
        </header>
        {#if fileLoading}<p class="empty">Loading…</p>
        {:else if typeof fileContent === 'string' && /(?:log|text|json)/i.test(openFile.media_type)}<LogPreview
            value={fileContent}
          />
        {:else}<CodePreview value={fileContent} />{/if}
        {#if fileRawURL}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
          <a class="download" href={fileRawURL} target="_blank">Open exact bytes ↗</a>{/if}
      </div>
    </div>
  {/if}
{/if}

<style>
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }
  .crumb {
    margin: 0 0 16px;
    color: var(--muted);
    font-size: 12px;
  }
  .crumb a {
    color: var(--focus);
  }
  .run-header {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 18px;
    padding-bottom: 24px;
    border-bottom: 1px solid var(--border);
  }
  .run-header p {
    margin: 4px 0 0;
    color: var(--subtle);
  }
  .status {
    padding: 7px 10px;
    border: 1px solid var(--syntax-green);
    color: var(--syntax-green);
    font-size: 12px;
    font-weight: 700;
    text-transform: uppercase;
  }
  .status.failed,
  .detail-heading span:not(.passed) {
    border-color: var(--love);
    color: var(--love);
  }
  dl {
    display: flex;
    gap: 24px;
    margin: 0;
  }
  dl div {
    display: grid;
  }
  dt {
    color: var(--muted);
    font-size: 10px;
    text-transform: uppercase;
  }
  dd {
    margin: 0;
    font-weight: 700;
  }
  h2,
  h3 {
    margin: 0;
  }
  .eyebrow {
    margin: 0 0 4px;
    color: var(--focus);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  .tests,
  .files,
  .logs {
    margin-top: 24px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  section > header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    padding: 15px 17px;
    border-bottom: 1px solid var(--border);
  }
  .controls {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  input {
    min-width: 220px;
    padding: 7px 9px;
    border: 1px solid var(--border);
    background: var(--base);
    color: var(--text);
  }
  .toggle {
    color: var(--subtle);
    font-size: 11px;
    white-space: nowrap;
  }
  .toggle input {
    min-width: 0;
  }
  .test-layout {
    display: grid;
    grid-template-columns: minmax(280px, 42%) minmax(0, 1fr);
    min-height: 320px;
  }
  .test-list {
    max-height: 560px;
    overflow: auto;
    border-right: 1px solid var(--border);
  }
  .test-list button,
  .file-list button {
    width: 100%;
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 11px;
    padding: 10px 14px;
    border: 0;
    border-bottom: 1px solid var(--border);
    background: transparent;
    color: inherit;
    text-align: left;
  }
  .test-list button:hover,
  .test-list button.active,
  .file-list button:hover {
    background: var(--hover);
  }
  .test-list button > span:nth-child(2),
  .file-list button span {
    display: grid;
    min-width: 0;
  }
  .test-list strong,
  .test-list small,
  .file-list strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  small {
    color: var(--muted);
    font-size: 10px;
  }
  .test-status {
    color: var(--love);
    font-size: 17px;
  }
  .test-status.passed,
  .detail-heading .passed {
    color: var(--syntax-green);
  }
  .test-detail {
    min-width: 0;
    padding: 18px;
  }
  .detail-heading {
    display: flex;
    justify-content: space-between;
    text-transform: uppercase;
    font-size: 11px;
    font-weight: 700;
  }
  .test-detail h3 {
    margin-top: 12px;
  }
  pre {
    max-height: 280px;
    overflow: auto;
    padding: 12px;
    background: var(--base);
    border: 1px solid var(--border);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font: inherit;
    font-size: 11px;
  }
  .empty {
    padding: 18px;
    color: var(--muted);
  }
  details.logs {
    padding: 0 17px 17px;
  }
  details.logs summary {
    padding: 15px 0;
    cursor: pointer;
    font-weight: 700;
  }
  .modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: grid;
    place-items: center;
    padding: 24px;
    background: rgb(0 0 0 / 72%);
  }
  .modal {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    width: min(1000px, 100%);
    height: min(760px, 92vh);
    border: 1px solid var(--border);
    background: var(--base);
  }
  .modal header button {
    border: 0;
    background: transparent;
    color: var(--text);
    font-size: 24px;
    cursor: pointer;
  }
  .modal :global(pre) {
    max-height: none;
  }
  .download {
    padding: 12px 17px;
    border-top: 1px solid var(--border);
    color: var(--focus);
    text-decoration: none;
  }
  .error {
    color: var(--love);
  }
  @media (max-width: 760px) {
    .run-header {
      grid-template-columns: auto minmax(0, 1fr);
    }
    .run-header dl {
      grid-column: 1 / -1;
    }
    section > header,
    .controls {
      align-items: stretch;
      flex-direction: column;
    }
    .controls input {
      width: 100%;
      min-width: 0;
    }
    .test-layout {
      grid-template-columns: 1fr;
    }
    .test-list {
      max-height: 360px;
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
  }
</style>
