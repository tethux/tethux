<script lang="ts">
  import { searchArtifact } from '$lib/api/artifacts';
  import type { LogSearchMatch } from '$lib/api/types';

  let { fileId }: { fileId: number } = $props();
  let query = $state('');
  let regex = $state(false);
  let severity = $state('all');
  let matches = $state<LogSearchMatch[]>([]);
  let total = $state(0);
  let truncated = $state(false);
  let loading = $state(false);
  let error = $state('');
  let timer: ReturnType<typeof setTimeout> | undefined;

  const levels = ['all', 'trace', 'debug', 'info', 'notice', 'warn', 'error', 'fatal', 'panic', 'critical'];

  function scheduleSearch() {
    clearTimeout(timer);
    timer = setTimeout(runSearch, 180);
  }

  async function runSearch() {
    loading = true;
    error = '';
    const result = await searchArtifact(fetch, fileId, { q: query, regex, severity });
    result.match(
      (value) => {
        matches = value.matches;
        total = value.total;
        truncated = value.truncated;
      },
      (apiError) => {
        matches = [];
        total = 0;
        error = apiError.message;
      }
    );
    loading = false;
  }
</script>

<section class="log-search" aria-label="Search full log">
  <header>
    <label>
      <span>Full log search</span>
      <input
        bind:value={query}
        oninput={scheduleSearch}
        onkeydown={(event) => event.key === 'Enter' && runSearch()}
        placeholder="error, package, test name…"
      />
    </label>
    <label class="select">
      <span>Severity</span>
      <select bind:value={severity} onchange={runSearch}>
        {#each levels as level (level)}<option value={level}>{level}</option>{/each}
      </select>
    </label>
    <label class="check"><input type="checkbox" bind:checked={regex} onchange={runSearch} /> regex</label>
    <button onclick={runSearch} disabled={loading}>{loading ? 'Searching…' : 'Search'}</button>
  </header>

  {#if error}
    <p class="error">{error}</p>
  {:else if matches.length}
    <div class="result-meta">
      <span>{total} matching {total === 1 ? 'line' : 'lines'}</span>
      {#if truncated}<em>showing first 500</em>{/if}
    </div>
    <ol>
      {#each matches as match (match.line)}
        <li class:flagged={['warn', 'error', 'fatal', 'panic', 'critical'].includes(match.severity)}>
          <a href={`#L${match.line}`} aria-label={`Line ${match.line}`}>{match.line}</a>
          <span class="level" data-level={match.severity}>{match.severity}</span>
          <code>{match.text || ' '}</code>
        </li>
      {/each}
    </ol>
  {:else if query || severity !== 'all'}
    <p class="empty">{loading ? 'Scanning the complete artifact…' : 'No matching lines.'}</p>
  {:else}
    <p class="empty">Searches the complete stored artifact, beyond the bounded preview.</p>
  {/if}
</section>

<style>
  .log-search {
    margin-top: 14px;
    border: 1px solid var(--border);
    background: var(--base);
  }
  header {
    display: grid;
    grid-template-columns: minmax(180px, 1fr) 116px auto auto;
    align-items: end;
    gap: 8px;
    padding: 10px;
    border-bottom: 1px solid var(--border);
  }
  label {
    display: grid;
    gap: 5px;
  }
  label > span {
    color: var(--muted);
    font-size: 9px;
    font-weight: 750;
    letter-spacing: 0.09em;
    text-transform: uppercase;
  }
  input,
  select,
  button {
    min-height: 34px;
    border: 1px solid var(--border);
    border-radius: 2px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
  }
  input {
    min-width: 0;
    padding: 0 9px;
  }
  select,
  button {
    padding: 0 9px;
  }
  button {
    cursor: pointer;
    font-weight: 700;
  }
  .check {
    display: flex;
    align-items: center;
    min-height: 34px;
    color: var(--muted);
    white-space: nowrap;
  }
  .check input {
    min-height: auto;
  }
  .result-meta {
    display: flex;
    justify-content: space-between;
    padding: 7px 10px;
    color: var(--muted);
    font-size: 10px;
  }
  .result-meta em {
    color: var(--gold);
    font-style: normal;
  }
  ol {
    max-height: 360px;
    margin: 0;
    padding: 0;
    overflow: auto;
    list-style: none;
  }
  li {
    display: grid;
    grid-template-columns: 48px 58px minmax(0, 1fr);
    border-top: 1px solid color-mix(in srgb, var(--border) 60%, transparent);
  }
  li.flagged {
    background: color-mix(in srgb, var(--love) 5%, transparent);
  }
  li > a,
  li > span,
  li > code {
    padding: 6px 8px;
  }
  li > a {
    color: var(--muted);
    text-align: right;
    text-decoration: none;
  }
  .level {
    color: var(--muted);
    font-size: 9px;
    text-transform: uppercase;
  }
  .level[data-level='warn'] { color: var(--gold); }
  .level[data-level='error'],
  .level[data-level='fatal'],
  .level[data-level='panic'],
  .level[data-level='critical'] { color: var(--love); }
  code {
    min-width: 0;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .empty,
  .error {
    margin: 0;
    padding: 18px 10px;
    color: var(--muted);
  }
  .error { color: var(--love); }
  @media (max-width: 720px) {
    header { grid-template-columns: 1fr 1fr; }
  }
</style>
