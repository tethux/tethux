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

  onMount(async () => {
    try {
      const response = await fetch('/api/v1/summary');

      if (!response.ok) {
        throw new Error(`Request failed (${response.status})`);
      }

      summary = await response.json();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Request failed';
    }
  });
</script>

<svelte:head><title>Summary · CI results</title></svelte:head>

<header class="page-header">
  <h1>Summary</h1>
  <p class="lede">Overview of the ingested CI database.</p>
</header>

<section class="counts panel" aria-label="Result totals">
  {#if error}
    <p class="summary-error">Unable to load totals: {error}</p>
  {/if}
  <div>
    <span>Runs</span>
    <strong>{summary?.run_count ?? '—'}</strong>
  </div>

  <div>
    <span>Tests</span>
    <strong>{summary?.test_count ?? '—'}</strong>
  </div>

  <div>
    <span>Passed</span>
    <strong>{summary?.passed_count ?? '—'}</strong>
  </div>

  <div>
    <span>Other</span>
    <strong>{summary?.failed_count ?? '—'}</strong>
  </div>
</section>

<style>
  .counts {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .counts div {
    padding: 16px;
    border-right: 1px solid #ddd;
  }

  .counts div:last-child {
    border-right: 0;
  }

  .summary-error {
    grid-column: 1 / -1;
    margin: 0;
    padding: 12px 16px;
    color: #a3362a;
    border-bottom: 1px solid #ddd;
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

  @media (max-width: 700px) {
    .counts {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
