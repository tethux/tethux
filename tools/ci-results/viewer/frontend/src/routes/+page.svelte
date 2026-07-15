<script lang="ts">
  import { onMount } from 'svelte';

  type Summary = {
    run_count: number;
    test_count: number;
    passed_count: number;
    failed_count: number;
  };
  let summary = $state<Summary | null>(null);
  let error = $state('');
  let query = $state('');

  onMount(async () => {
    try {
      const response = await fetch('/api/v1/summary');
      if (!response.ok) throw new Error(`request failed (${response.status})`);
      summary = await response.json();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'request failed';
    }
  });
</script>

<svelte:head><title>Summary · CI results</title></svelte:head>

<header class="page-header">
  <h1>Summary</h1>
  <p class="lede">Overview of the ingested CI database.</p>
</header>

<section class="counts panel" aria-label="Result totals">
  <div><span>Runs</span><strong>{summary?.run_count ?? '—'}</strong></div>
  <div><span>Tests</span><strong>{summary?.test_count ?? '—'}</strong></div>
  <div><span>Passed</span><strong>{summary?.passed_count ?? '—'}</strong></div>
  <div><span>Other</span><strong>{summary?.failed_count ?? '—'}</strong></div>
</section>

<section class="query panel">
  <label for="query">Query builder</label>
  <p>This is the input you can wire to your query builder later.</p>
  <div>
    <input id="query" bind:value={query} placeholder="status = passed and branch = main" /><button
      disabled>Run query</button
    >
  </div>
</section>

<section class="output panel">
  <label for="output">Output</label>
  <textarea
    id="output"
    readonly
    value={error ? `Error: ${error}` : JSON.stringify(summary, null, 2)}></textarea>
</section>

<style>
  .counts {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
  }
  .counts div {
    padding: 16px;
    border-right: 1px solid #ddd;
  }
  .counts div:last-child {
    border: 0;
  }
  .counts span {
    display: block;
    color: #777;
    font-size: 12px;
  }
  .counts strong {
    display: block;
    margin-top: 8px;
    font-size: 24px;
  }
  .query,
  .output {
    padding: 20px;
  }
  label {
    font-weight: 600;
  }
  .query p {
    color: #666;
  }
  .query div {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 8px;
  }
  input,
  textarea {
    width: 100%;
    padding: 10px;
    border: 1px solid #aaa;
    background: #fff;
  }
  button {
    padding: 0 18px;
    border: 1px solid #bbb;
    color: #888;
  }
  textarea {
    min-height: 210px;
    margin-top: 16px;
    resize: vertical;
  }
  @media (max-width: 700px) {
    .counts {
      grid-template-columns: repeat(2, 1fr);
    }
    .query div {
      grid-template-columns: 1fr;
    }
  }
</style>
