<script lang="ts">
  import type { PageData } from './$types';
  import type { ArchiveFile, TestResult } from '$lib/api/types';
  import { sourceRepositories } from '$lib/repositories';
  import CommitLink from '$lib/components/CommitLink.svelte';
  import VirtualList from '@humanspeak/svelte-virtual-list';
  import { SvelteSet } from 'svelte/reactivity';

  let { data }: { data: PageData } = $props();
  let selected = $state<TestResult | null>(null);
  $effect(() => {
    if (!selected && data.detail?.tests[0]) selected = data.detail.tests[0];
  });
  let tab = $state<'overview' | 'files' | 'manifest'>('overview');
  let openFile = $state<ArchiveFile | null>(null);
  let fileContent = $state<unknown>(null);
  let fileAvailable = $state(false);
  let fileLoading = $state(false);
  let manifestMode = $state<'structured' | 'json'>('structured');
  let testSearch = $state('');
  let providerFilter = $state('all');
  const statusFilters = new SvelteSet<string>();

  const fmtDuration = (ms: number) => (ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms}ms`);
  const fmtTime = (value: string) =>
    new Date(value).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  const value = (v: { String: string; Valid: boolean } | null | undefined) =>
    v?.Valid ? v.String : null;
  const intValue = (v: { Int64: number; Valid: boolean } | null | undefined) =>
    v?.Valid ? v.Int64 : 0;
  const sourceUrl = (repo: string, test: TestResult) => {
    const file = value(test.source_file);
    if (!data.detail) return repo;
    if (!file) {
      const symbol = value(test.source_symbol) ?? test.test_name;
      return `${repo.replace(/\/$/, '')}/search?q=${encodeURIComponent(symbol)}&type=code`;
    }
    const route = repo.includes('codeberg.org') ? 'src/commit' : 'blob';
    return `${repo.replace(/\/$/, '')}/${route}/${data.detail.run.commit_sha}/${file}`;
  };
  const manifest = $derived(data.detail ? JSON.parse(data.detail.run.manifest_json) : {});
  const testProvider = (test: TestResult) => {
    const parameters = JSON.parse(test.parameters_json || '{}');
    return parameters.provider ?? parameters.runtime ?? parameters.backend ?? 'other';
  };
  const providers = $derived([...new Set((data.detail?.tests ?? []).map(testProvider))].sort());
  const filteredTests = $derived(
    (data.detail?.tests ?? []).filter((test) => {
      const query = testSearch.trim().toLowerCase();
      const matchesText =
        !query ||
        `${test.test_name} ${test.test_key} ${value(test.suite) ?? ''}`
          .toLowerCase()
          .includes(query);
      return (
        matchesText &&
        (statusFilters.size === 0 || statusFilters.has(test.status)) &&
        (providerFilter === 'all' || testProvider(test) === providerFilter)
      );
    })
  );
  const resultPreview = $derived(
    openFile?.archive_path === 'results.json' &&
      fileContent &&
      typeof fileContent === 'object' &&
      'tests' in fileContent &&
      Array.isArray(fileContent.tests)
      ? (fileContent.tests as TestResult[])
      : null
  );
  const metricPoints = (test: TestResult) => {
    const metrics = JSON.parse(test.metrics_json || '{}');
    const points = Object.entries(metrics)
      .filter(
        ([key, item]) => typeof item === 'number' && /(ms|latency|rtt|ping|duration)/i.test(key)
      )
      .map(([key, item]) => ({ label: key.replaceAll('_', ' '), value: Number(item) }));
    return points.length ? points : [{ label: 'test duration', value: intValue(test.duration_ms) }];
  };
  const metricPeak = (test: TestResult) =>
    Math.max(...metricPoints(test).map((point) => point.value), 1);
  const displayValue = (entry: unknown) =>
    typeof entry === 'boolean' ? (entry ? 'Yes' : 'No') : entry === null ? '—' : String(entry);
  type ManifestFile = {
    path: string;
    type: string;
    media_type: string;
    size_bytes: number;
    sha256: string;
    public: boolean;
  };
  const formatBytes = (bytes: number) =>
    bytes >= 1024 * 1024
      ? `${(bytes / 1024 / 1024).toFixed(1)} MB`
      : `${(bytes / 1024).toFixed(1)} KB`;
  const indexedFile = (path: string) =>
    data.detail?.files.find((file) => file.archive_path === path);
  const flatEntries = (entry: unknown, prefix = ''): Array<[string, unknown]> => {
    if (entry === null || typeof entry !== 'object') return [[prefix || 'value', entry]];
    return Object.entries(entry).flatMap(([key, child]) => {
      const path = prefix ? `${prefix}.${key}` : key;
      return child !== null && typeof child === 'object'
        ? flatEntries(child, path)
        : [[path, child]];
    });
  };
  async function viewFile(file: ArchiveFile) {
    openFile = file;
    fileContent = null;
    fileLoading = true;
    try {
      const response = await fetch(`/api/v1/file/${file.id}`);
      const payload = await response.json();
      fileAvailable = Boolean(payload.available);
      fileContent = payload.content;
    } catch {
      fileAvailable = false;
      fileContent = { message: 'The file preview could not be loaded.' };
    } finally {
      fileLoading = false;
    }
  }
  function toggleStatus(status: string) {
    if (statusFilters.has(status)) statusFilters.delete(status);
    else statusFilters.add(status);
  }
</script>

<svelte:head><title>Run #{data.detail?.run.id ?? ''} · CI results</title></svelte:head>

{#if data.detail}
  {@const run = data.detail.run}
  <div class="crumb">Runs <span>/</span> Run #{run.id}</div>
  <header class="hero">
    <div class:bad={run.status !== 'passed'} class="status-icon">
      {run.status === 'passed' ? '✓' : '×'}
    </div>
    <div>
      <div class="title-line">
        <h1>Run #{run.id}</h1>
        <span class:failed={run.status !== 'passed'} class="badge">{run.status}</span>
      </div>
      <p>
        {value(run.workflow) ?? 'CI workflow'} <span>·</span>
        {value(run.job) ?? value(run.source_provider) ?? 'imported run'} <span>·</span> Attempt {run.source_attempt}
      </p>
    </div>
  </header>

  <section class="summary-card">
    <div>
      <span class="label">Commit</span><strong class="accent"
        ><CommitLink hash={run.commit_sha} repositories={sourceRepositories} /></strong
      ><small>{value(run.branch) ?? 'detached'}</small>
    </div>
    <div>
      <span class="label">Duration</span><strong>{fmtDuration(run.duration_ms)}</strong><small
        >{fmtTime(run.started_at)} – {fmtTime(run.finished_at)}</small
      >
    </div>
    <div>
      <span class="label">Tests</span><strong class="success"
        >{run.passed_count} / {run.total_count} passed</strong
      ><small>{run.failed_count} failed · {run.skipped_count} skipped</small>
    </div>
    <div>
      <span class="label">Provider</span><strong>{value(run.source_provider) ?? 'CI'}</strong><small
        >{value(run.trigger_name) ?? 'completed result'}</small
      >
    </div>
  </section>

  <nav class="tabs" aria-label="Run sections">
    {#each ['overview', 'files', 'manifest'] as item (item)}
      <button class:active={tab === item} onclick={() => (tab = item as typeof tab)}
        >{item === 'files' ? `Files (${data.detail?.files.length})` : item}</button
      >
    {/each}
  </nav>

  {#if tab === 'overview'}
    <div class="workspace">
      <section class="test-panel">
        <div class="panel-title">
          <div>
            <h2>Tests</h2>
            <p>Select a test to inspect its result and source</p>
          </div>
          <span>{filteredTests.length} of {data.detail.tests.length} results</span>
        </div>
        <div class="test-tools">
          <label class="search"
            ><span>⌕</span><input
              bind:value={testSearch}
              placeholder="Search tests, suites, or keys…"
              aria-label="Search tests"
            /></label
          >
          <details class="filter-select">
            <summary>Status {statusFilters.size ? `(${statusFilters.size})` : ''}</summary>
            <div>
              {#each ['passed', 'failed', 'error', 'skipped', 'cancelled'] as status (status)}
                <label
                  ><input
                    type="checkbox"
                    checked={statusFilters.has(status)}
                    onchange={() => toggleStatus(status)}
                  /> <span class={`filter-dot ${status}`}></span>{status}</label
                >
              {/each}
              {#if statusFilters.size}<button type="button" onclick={() => statusFilters.clear()}
                  >Clear filters</button
                >{/if}
            </div>
          </details>
          <label class="provider-select"
            ><span>Provider</span><select bind:value={providerFilter}
              ><option value="all">All providers</option
              >{#each providers as provider (provider)}<option value={provider}>{provider}</option
                >{/each}</select
            ></label
          >
        </div>
        <div class="test-list virtual">
          {#snippet testRow(test: TestResult)}
            <button
              type="button"
              class:selected={selected?.id === test.id}
              onclick={() => (selected = test)}
            >
              <span class:bad-dot={test.status !== 'passed'} class="dot"
                >{test.status === 'passed' ? '✓' : '×'}</span
              >
              <span class="test-name"
                ><strong>{test.test_name}</strong><small>{value(test.suite) ?? test.test_key}</small
                ></span
              >
              <span class="duration">{fmtDuration(intValue(test.duration_ms))}</span><span
                class="chevron">›</span
              >
            </button>
          {/snippet}
          <VirtualList
            items={filteredTests}
            defaultEstimatedItemHeight={63}
            bufferSize={8}
            hasMore={false}
            viewportLabel="Run test results"
          >
            {#snippet renderItem(test: TestResult)}{@render testRow(test)}{/snippet}
          </VirtualList>
        </div>
      </section>

      {#if selected}
        <aside class="inspector">
          <div class="inspector-head">
            <small>TEST RESULT</small><button
              aria-label="Close test details"
              onclick={() => (selected = null)}>×</button
            >
          </div>
          <h2>{selected.test_name}</h2>
          <span class:failed={selected.status !== 'passed'} class="badge"
            >{selected.status === 'passed' ? '✓' : '×'} {selected.status}</span
          >

          <dl>
            <div>
              <dt>Duration</dt>
              <dd>{fmtDuration(intValue(selected.duration_ms))}</dd>
            </div>
            <div>
              <dt>Attempt</dt>
              <dd>{selected.attempt}</dd>
            </div>
            <div>
              <dt>Kind</dt>
              <dd>{selected.result_kind.replaceAll('_', ' ')}</dd>
            </div>
          </dl>

          <div class="source-block">
            <small>SOURCE</small>
            <strong>{value(selected.source_symbol) ?? selected.test_name}</strong>
            <code
              >{value(selected.source_file) ?? 'Source location not included in this result'}</code
            >
            <div class="repo-links">
              {#each sourceRepositories as repo (repo.url)}
                <!-- External repository URL, not an internal SvelteKit route. -->
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
                <a href={sourceUrl(repo.url, selected)} target="_blank" rel="noreferrer"
                  >Open in {repo.name} ↗</a
                >
              {/each}
            </div>
          </div>

          <div class="metrics-block">
            <div class="metrics-title">
              <small>LATENCY & TIMING</small><span>milliseconds</span>
            </div>
            <div class="metric-chart">
              {#each metricPoints(selected) as point (point.label)}<div class="metric-row">
                  <span>{point.label}</span>
                  <div>
                    <i style:width={`${Math.max((point.value / metricPeak(selected)) * 100, 2)}%`}
                    ></i>
                  </div>
                  <strong>{point.value.toFixed(1)} ms</strong>
                </div>{/each}
            </div>
          </div>

          {#if value(selected.message)}<div class="message">
              <small>MESSAGE</small>
              <pre>{value(selected.message)}</pre>
            </div>{/if}
        </aside>
      {/if}
    </div>
  {:else if tab === 'files'}
    <section class="table-card">
      <div class="panel-title">
        <div>
          <h2>Archive files</h2>
          <p>Files captured after this run completed</p>
        </div>
      </div>
      {#each data.detail.files as file (file.id)}<button
          type="button"
          class="file-row"
          onclick={() => viewFile(file)}
        >
          <span>▧</span><strong>{file.archive_path}</strong><small>{file.file_type}</small><code
            >{(file.size_bytes / 1024).toFixed(1)} KB</code
          >
          <span class="chevron">›</span>
        </button>{/each}
    </section>
  {:else}
    <section class="manifest">
      <div class="panel-title">
        <div>
          <h2>Manifest</h2>
          <p>Immutable data captured by CI</p>
        </div>
        <div class="view-toggle">
          <button
            class:active={manifestMode === 'structured'}
            onclick={() => (manifestMode = 'structured')}>Details</button
          ><button class:active={manifestMode === 'json'} onclick={() => (manifestMode = 'json')}
            >JSON</button
          >
        </div>
      </div>
      {#if manifestMode === 'structured'}<div class="manifest-grid">
          {#each Object.entries(manifest) as [section, sectionValue] (section)}
            {#if section === 'files' && Array.isArray(sectionValue)}
              <article class="manifest-files">
                <div class="manifest-files-head">
                  <h3>Files</h3>
                  <span>{sectionValue.length} indexed entries</span>
                </div>
                <div class="manifest-file-list">
                  {#each sectionValue as rawFile (rawFile.path)}
                    {@const file = rawFile as ManifestFile}
                    {@const stored = indexedFile(file.path)}
                    <button
                      type="button"
                      disabled={!stored}
                      onclick={() => stored && viewFile(stored)}
                    >
                      <span class={`file-kind ${file.type}`}
                        >{file.type === 'log'
                          ? '≡'
                          : file.type === 'artifact'
                            ? '◇'
                            : file.type === 'config'
                              ? '⚙'
                              : '{ }'}</span
                      >
                      <span class="manifest-file-name"
                        ><strong>{file.path}</strong><small>{file.media_type}</small></span
                      >
                      <span class="visibility" class:private={!file.public}
                        >{file.public ? 'Public' : 'Private'}</span
                      >
                      <span class="file-size">{formatBytes(file.size_bytes)}</span>
                      <code title={file.sha256}>{file.sha256.slice(0, 9)}…</code><span
                        class="chevron">›</span
                      >
                    </button>
                  {/each}
                </div>
              </article>
            {:else}<article>
                <h3>{section.replaceAll('_', ' ')}</h3>
                <dl>
                  {#each flatEntries(sectionValue) as [key, item] (key)}
                    <div>
                      <dt>{key.replaceAll('_', ' ')}</dt>
                      <dd>{displayValue(item)}</dd>
                    </div>
                  {/each}
                </dl>
              </article>{/if}
          {/each}
        </div>{:else}<pre class="json-view">{JSON.stringify(manifest, null, 2)}</pre>{/if}
    </section>
  {/if}

  {#if openFile}
    <div
      class="modal-backdrop"
      role="presentation"
      onclick={(event) => event.currentTarget === event.target && (openFile = null)}
    >
      <div class="file-modal" role="dialog" aria-modal="true" aria-labelledby="file-title">
        <header>
          <div>
            <small>{openFile.file_type} · {(openFile.size_bytes / 1024).toFixed(1)} KB</small>
            <h2 id="file-title">{openFile.archive_path}</h2>
          </div>
          <button aria-label="Close file preview" onclick={() => (openFile = null)}>×</button>
        </header>
        {#if fileLoading}<div class="loading">Loading file data…</div>{:else}
          {#if !fileAvailable}<p class="notice">
              Preview metadata is shown because this file’s raw bytes are not retained in SQLite.
            </p>{/if}
          {#if resultPreview}<div class="results-preview">
              <div class="results-head">
                <strong>{resultPreview.length} test results</strong><span
                  >{resultPreview.filter((item) => item.status === 'passed').length} passed</span
                >
              </div>
              {#each resultPreview as result (result.id)}<article>
                  <span class:bad-dot={result.status !== 'passed'} class="dot"
                    >{result.status === 'passed' ? '✓' : '×'}</span
                  >
                  <div><strong>{result.test_name}</strong><small>{result.test_key}</small></div>
                  <span class:failed={result.status !== 'passed'} class="result-status"
                    >{result.status}</span
                  ><code>{fmtDuration(intValue(result.duration_ms))}</code>
                </article>{/each}
            </div>{:else}<pre>{typeof fileContent === 'string'
                ? fileContent
                : JSON.stringify(fileContent, null, 2)}</pre>{/if}
        {/if}
      </div>
    </div>
  {/if}
{:else}<p>{data.error ?? 'Run not found'}</p>{/if}

<style>
  :global(main) {
    width: min(1440px, 100%) !important;
    padding: 28px 34px 70px !important;
  }
  :global(body) {
    background: #f5f7f9 !important;
    color: #1c2733 !important;
    font-family: 'IBM Plex Sans', 'Aptos', sans-serif !important;
  }
  .crumb {
    color: #71808e;
    font-size: 13px;
    margin-bottom: 20px;
  }
  .crumb span {
    padding: 0 9px;
    color: #adb7c0;
  }
  .hero {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .status-icon {
    display: grid;
    place-items: center;
    width: 38px;
    height: 38px;
    border-radius: 50%;
    background: #dcf5e4;
    color: #14813b;
    font-size: 23px;
    font-weight: 700;
  }
  .status-icon.bad {
    background: #fee7e5;
    color: #c8372d;
  }
  .title-line {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .title-line h1 {
    font-size: 29px;
    letter-spacing: -0.03em;
  }
  .hero p {
    margin: 5px 0 0;
    color: #667582;
  }
  .hero p span {
    padding: 0 5px;
    color: #b2bac1;
  }
  .badge {
    padding: 4px 9px;
    border-radius: 5px;
    background: #def4e4;
    color: #18763a;
    text-transform: capitalize;
    font-size: 12px;
    font-weight: 700;
  }
  .badge.failed {
    background: #fde6e4;
    color: #b92f27;
  }
  .summary-card {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    margin-top: 24px;
    background: white;
    border: 1px solid #dce2e7;
    border-radius: 8px;
    box-shadow: 0 2px 5px #1a2a3a0b;
  }
  .summary-card > div {
    display: grid;
    gap: 4px;
    padding: 18px 20px;
    border-right: 1px solid #e5e9ed;
  }
  .summary-card > div:last-child {
    border: 0;
  }
  .label,
  .source-block > small,
  .message > small {
    font-size: 11px;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: #7b8995;
  }
  .summary-card strong {
    font-size: 16px;
  }
  .summary-card small {
    color: #6d7a86;
  }
  .accent {
    color: #2867bd;
  }
  .success {
    color: #218445;
  }
  .tabs {
    display: flex;
    gap: 28px;
    margin-top: 22px;
    border-bottom: 1px solid #d5dde3;
  }
  .tabs button {
    padding: 12px 2px;
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    color: #53616d;
    text-transform: capitalize;
    cursor: pointer;
  }
  .tabs button.active {
    color: #195eaa;
    border-color: #2e78ca;
    font-weight: 700;
  }
  .workspace {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 390px;
    gap: 14px;
    margin-top: 16px;
    align-items: start;
  }
  .test-panel,
  .inspector,
  .table-card,
  .manifest {
    background: #fff;
    border: 1px solid #dce2e7;
    border-radius: 7px;
    box-shadow: 0 2px 6px #1c2e3e0b;
    overflow: hidden;
  }
  .panel-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 18px;
    border-bottom: 1px solid #e4e8eb;
  }
  .panel-title h2 {
    margin: 0;
    font-size: 15px;
  }
  .panel-title p {
    margin: 3px 0 0;
    color: #788590;
    font-size: 12px;
  }
  .panel-title > span {
    color: #77848f;
    font-size: 12px;
  }
  .test-tools {
    display: flex;
    gap: 10px;
    padding: 10px 14px;
    border-bottom: 1px solid #e4e8eb;
    background: #fafbfc;
  }
  .search {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border: 1px solid #d4dce2;
    border-radius: 5px;
    background: #fff;
    color: #77848f;
  }
  .search input {
    width: 100%;
    border: 0;
    outline: 0;
    background: transparent;
    color: #25313c;
  }
  .filter-select {
    position: relative;
  }
  .filter-select summary {
    min-width: 104px;
    padding: 7px 10px;
    border: 1px solid #d4dce2;
    border-radius: 5px;
    background: #fff;
    cursor: pointer;
    list-style: none;
  }
  .filter-select summary::after {
    content: '⌄';
    float: right;
    margin-left: 12px;
  }
  .filter-select > div {
    position: absolute;
    z-index: 30;
    right: 0;
    top: calc(100% + 5px);
    width: 170px;
    padding: 8px;
    background: #fff;
    border: 1px solid #cad4dc;
    border-radius: 6px;
    box-shadow: 0 12px 30px #1d2c3a24;
  }
  .filter-select label {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 7px;
    text-transform: capitalize;
    cursor: pointer;
  }
  .filter-select button {
    width: 100%;
    margin-top: 5px;
    padding: 7px;
    border: 0;
    border-top: 1px solid #e4e8eb;
    background: #fff;
    color: #2867bd;
    cursor: pointer;
  }
  .provider-select {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 0 9px;
    border: 1px solid #d4dce2;
    border-radius: 5px;
    background: #fff;
    color: #71808c;
    font-size: 12px;
  }
  .provider-select select {
    min-width: 112px;
    border: 0;
    outline: 0;
    background: #fff;
    color: #263542;
    text-transform: capitalize;
  }
  .filter-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #8995a0;
  }
  .filter-dot.passed {
    background: #20a04b;
  }
  .filter-dot.failed,
  .filter-dot.error {
    background: #d9443b;
  }
  .filter-dot.skipped {
    background: #d19a27;
  }
  .test-list :global(button) {
    width: 100%;
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr) auto 16px;
    align-items: center;
    gap: 10px;
    padding: 13px 16px;
    border: 0;
    border-bottom: 1px solid #edf0f2;
    background: #fff;
    text-align: left;
    cursor: pointer;
  }
  .test-list :global(button:hover),
  .test-list :global(button.selected) {
    background: #f2f7fc;
  }
  .test-list :global(button.selected) {
    box-shadow: inset 3px 0 #2d76c5;
  }
  .dot {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: #def4e4;
    color: #16813b;
    font-weight: 800;
  }
  .dot.bad-dot {
    background: #fee4e1;
    color: #c9362d;
  }
  .test-name {
    display: grid;
    gap: 3px;
    min-width: 0;
  }
  .test-name strong {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .test-name small,
  .duration,
  .chevron {
    color: #7b8791;
  }
  .chevron {
    font-size: 22px;
  }
  .test-list.virtual {
    height: 440px;
    overflow: hidden;
  }
  .inspector {
    padding: 18px;
    position: sticky;
    top: 20px;
  }
  .inspector-head {
    display: flex;
    justify-content: space-between;
    color: #7a8792;
  }
  .inspector-head button {
    border: 0;
    background: none;
    color: #6e7b85;
    font-size: 22px;
    cursor: pointer;
  }
  .inspector h2 {
    font-size: 18px;
    margin: 8px 0 10px;
    overflow-wrap: anywhere;
  }
  .inspector dl {
    margin: 20px 0;
    border-top: 1px solid #e2e7ea;
  }
  .inspector dl div {
    display: flex;
    justify-content: space-between;
    padding: 10px 0;
    border-bottom: 1px solid #e9ecef;
  }
  .inspector dt {
    color: #75828d;
  }
  .inspector dd {
    margin: 0;
    font-weight: 600;
    text-transform: capitalize;
  }
  .source-block {
    display: grid;
    gap: 8px;
    margin-top: 20px;
    padding: 14px;
    background: #f6f8fa;
    border: 1px solid #e0e5e9;
    border-radius: 5px;
  }
  .source-block code {
    color: #396eaa;
    overflow-wrap: anywhere;
  }
  .metrics-block {
    margin-top: 16px;
    padding: 14px;
    border: 1px solid #e0e5e9;
    border-radius: 5px;
    background: #fbfcfd;
  }
  .metrics-title {
    display: flex;
    justify-content: space-between;
    margin-bottom: 12px;
    color: #788590;
  }
  .metrics-title small {
    letter-spacing: 0.06em;
  }
  .metrics-title span {
    font-size: 11px;
  }
  .metric-chart {
    display: grid;
    gap: 10px;
  }
  .metric-row {
    display: grid;
    grid-template-columns: 95px minmax(60px, 1fr) 72px;
    align-items: center;
    gap: 8px;
    font-size: 11px;
  }
  .metric-row > span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-transform: capitalize;
  }
  .metric-row > div {
    height: 8px;
    overflow: hidden;
    border-radius: 4px;
    background: #e6ebef;
  }
  .metric-row i {
    display: block;
    height: 100%;
    border-radius: 4px;
    background: linear-gradient(90deg, #4d91d7, #1c6dbc);
  }
  .metric-row strong {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .repo-links {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 7px;
    margin-top: 4px;
  }
  .repo-links a {
    padding: 8px;
    border: 1px solid #cbd7e1;
    border-radius: 4px;
    background: #fff;
    color: #2365ad;
    text-align: center;
    text-decoration: none;
    font-size: 12px;
    font-weight: 700;
  }
  .repo-links a:hover {
    background: #eaf3fc;
  }
  .message {
    margin-top: 16px;
  }
  .message pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    background: #f7f8f9;
    border: 1px solid #e2e5e8;
    padding: 12px;
    font-size: 12px;
  }
  .table-card,
  .manifest {
    margin-top: 16px;
  }
  .file-row {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) 100px 80px 16px;
    gap: 12px;
    width: 100%;
    align-items: center;
    padding: 13px 18px;
    border-bottom: 1px solid #edf0f2;
    border-top: 0;
    border-left: 0;
    border-right: 0;
    background: #fff;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .file-row:hover {
    background: #f2f7fc;
  }
  .file-row small {
    color: #75828d;
  }
  .manifest-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 14px;
    padding: 16px;
  }
  .manifest-grid article {
    border: 1px solid #e0e5e9;
    border-radius: 6px;
    overflow: hidden;
  }
  .manifest-files {
    grid-column: 1/-1;
  }
  .manifest-files-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: #f5f7f9;
    border-bottom: 1px solid #e0e5e9;
  }
  .manifest-files-head h3 {
    border: 0;
  }
  .manifest-files-head span {
    padding-right: 14px;
    color: #74818c;
    font-size: 11px;
  }
  .manifest-file-list button {
    width: 100%;
    display: grid;
    grid-template-columns: 30px minmax(220px, 1fr) 62px 70px 92px 16px;
    align-items: center;
    gap: 10px;
    padding: 11px 14px;
    border: 0;
    border-bottom: 1px solid #e9edef;
    background: #fff;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .manifest-file-list button:last-child {
    border-bottom: 0;
  }
  .manifest-file-list button:hover {
    background: #f2f7fc;
  }
  .manifest-file-list button:disabled {
    cursor: default;
    opacity: 0.72;
  }
  .file-kind {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border-radius: 5px;
    background: #edf3f8;
    color: #326d9f;
    font:
      700 11px/1 ui-monospace,
      monospace;
  }
  .file-kind.log {
    background: #f1f1ee;
    color: #6d6d64;
  }
  .file-kind.artifact {
    background: #e9f5ed;
    color: #26804a;
  }
  .file-kind.config {
    background: #fff3dc;
    color: #956819;
  }
  .manifest-file-name {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .manifest-file-name strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .manifest-file-name small {
    color: #7a8791;
  }
  .visibility {
    width: max-content;
    padding: 3px 6px;
    border-radius: 10px;
    background: #e4f4e9;
    color: #247644;
    font-size: 10px;
    font-weight: 700;
  }
  .visibility.private {
    background: #edf0f2;
    color: #66737d;
  }
  .file-size,
  .manifest-file-list code {
    color: #697681;
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .manifest-file-list code {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .manifest-grid h3 {
    margin: 0;
    padding: 11px 14px;
    background: #f5f7f9;
    border-bottom: 1px solid #e0e5e9;
    font-size: 13px;
    text-transform: capitalize;
  }
  .manifest-grid dl {
    margin: 0;
  }
  .manifest-grid dl div {
    display: grid;
    grid-template-columns: minmax(110px, 0.8fr) 1.2fr;
    gap: 12px;
    padding: 9px 14px;
    border-bottom: 1px solid #edf0f2;
  }
  .manifest-grid dl div:last-child {
    border: 0;
  }
  .manifest-grid dt {
    color: #71808c;
    overflow-wrap: anywhere;
  }
  .manifest-grid dd {
    margin: 0;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .view-toggle {
    display: flex;
    padding: 2px;
    border: 1px solid #d2dae1;
    border-radius: 5px;
    background: #f3f5f7;
  }
  .view-toggle button {
    padding: 5px 10px;
    border: 0;
    border-radius: 3px;
    background: transparent;
    color: #677580;
    cursor: pointer;
  }
  .view-toggle button.active {
    background: #fff;
    color: #245f9f;
    box-shadow: 0 1px 3px #1e2c381c;
    font-weight: 700;
  }
  .json-view {
    margin: 16px;
    max-height: 650px;
    padding: 18px;
    overflow: auto;
    border: 1px solid #dfe5e9;
    border-radius: 5px;
    background: #f7f9fa;
    color: #263644;
    font:
      12px/1.65 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
  }
  .modal-backdrop {
    position: fixed;
    inset: 0;
    z-index: 2000;
    display: grid;
    place-items: center;
    padding: 24px;
    background: #17243166;
    backdrop-filter: blur(3px);
  }
  .file-modal {
    width: min(820px, 100%);
    max-height: min(760px, 90vh);
    display: flex;
    flex-direction: column;
    background: #fff;
    border: 1px solid #cad3da;
    border-radius: 9px;
    box-shadow: 0 24px 70px #14202c3d;
    overflow: hidden;
  }
  .file-modal header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 17px 20px;
    border-bottom: 1px solid #dde3e7;
  }
  .file-modal h2 {
    margin: 3px 0 0;
    font-size: 17px;
    overflow-wrap: anywhere;
  }
  .file-modal header small {
    color: #76838e;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .file-modal header button {
    border: 0;
    background: none;
    font-size: 25px;
    color: #667580;
    cursor: pointer;
  }
  .file-modal pre {
    margin: 0;
    padding: 20px;
    overflow: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    background: #f8fafb;
    color: #24313d;
    font:
      12px/1.65 ui-monospace,
      SFMono-Regular,
      Menlo,
      monospace;
  }
  .results-preview {
    overflow: auto;
  }
  .results-head {
    display: flex;
    justify-content: space-between;
    padding: 12px 18px;
    background: #f6f8f9;
    border-bottom: 1px solid #e0e5e9;
  }
  .results-head span {
    color: #238047;
  }
  .results-preview article {
    display: grid;
    grid-template-columns: 24px minmax(0, 1fr) 70px 70px;
    align-items: center;
    gap: 10px;
    padding: 12px 18px;
    border-bottom: 1px solid #e8ecef;
  }
  .results-preview article > div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .results-preview article small {
    color: #74818c;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .result-status {
    text-transform: capitalize;
    color: #278049;
  }
  .result-status.failed {
    color: #bd3830;
  }
  .results-preview article > code {
    text-align: right;
    color: #65727c;
  }
  .loading,
  .notice {
    margin: 0;
    padding: 20px;
    color: #6b7883;
  }
  .notice {
    padding: 10px 20px;
    background: #fff7dd;
    border-bottom: 1px solid #eadb9b;
    color: #725d1b;
  }
  @media (max-width: 950px) {
    .summary-card {
      grid-template-columns: 1fr 1fr;
    }
    .summary-card > div:nth-child(2) {
      border-right: 0;
    }
    .workspace {
      grid-template-columns: 1fr;
    }
    .inspector {
      position: static;
    }
    .repo-links {
      grid-template-columns: 1fr;
    }
    .manifest-grid {
      grid-template-columns: 1fr;
    }
    .manifest-file-list button {
      grid-template-columns: 30px minmax(150px, 1fr) 60px 16px;
    }
    .manifest-file-list code,
    .manifest-file-list .visibility {
      display: none;
    }
  }
  @media (max-width: 620px) {
    :global(main) {
      padding: 20px 14px 50px !important;
    }
    .summary-card {
      grid-template-columns: 1fr;
    }
    .summary-card > div {
      border-right: 0;
      border-bottom: 1px solid #e5e9ed;
    }
    .tabs {
      gap: 14px;
      overflow: auto;
    }
    .hero {
      align-items: flex-start;
    }
    .file-row {
      grid-template-columns: 20px 1fr;
    }
    .file-row small,
    .file-row code {
      display: none;
    }
  }
</style>
