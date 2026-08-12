<script lang="ts">
  import { resolve } from '$app/paths';
  import { barY, defineChart } from '@tanstack/charts';
  import { Chart } from '@tanstack/charts/svelte';
  import { tooltip } from '@tanstack/charts/tooltip';
  import { scaleBand, scaleLinear } from 'd3-scale';
  import type { PageData } from './$types';
  import { nullStringValue } from '$lib/api/types';

  let { data }: { data: PageData } = $props();

  const recent = $derived(data.runs.slice(0, 30));
  const failures = $derived(data.runs.filter((run) => run.status !== 'passed').slice(0, 12));
  const chartRows = $derived(
    [...recent].reverse().map((run) => ({
      id: run.run_uid,
      label: new Date(run.started_at).toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric'
      }),
      failures: run.failed_count + run.errored_count
    }))
  );
  const chart = $derived(
    defineChart(
      {
        marks: [
          barY(chartRows, {
            x: 'label',
            y: 'failures',
            key: 'id',
            fill: 'var(--love)'
          })
        ],
        x: {
          scale: scaleBand()
            .domain(chartRows.map((row) => row.label))
            .padding(0.18),
          ticks: 5
        },
        y: {
          scale: scaleLinear()
            .domain([0, Math.max(...chartRows.map((row) => row.failures), 1)])
            .nice(),
          grid: true,
          ticks: 4
        }
      },
      { tooltip }
    )
  );

  const duration = (ms: number) =>
    ms >= 60_000 ? `${(ms / 60_000).toFixed(1)}m` : `${(ms / 1000).toFixed(1)}s`;
  const when = (value: string) => new Date(value).toLocaleString();
</script>

<svelte:head><title>Failures · CI results</title></svelte:head>

<header class="page-header">
  <p class="eyebrow">CI results</p>
  <h1>What failed?</h1>
  <p class="lede">The latest failing runs, with logs and artifacts one click away.</p>
</header>

{#if data.error}<p class="error">Could not load results: {data.error}</p>{/if}

<section class="counts" aria-label="Archive totals">
  <div><strong>{failures.length}</strong><span>recent failures</span></div>
  <div><strong>{data.summary?.run_count ?? '—'}</strong><span>runs retained</span></div>
  <div><strong>{data.summary?.test_count ?? '—'}</strong><span>test results</span></div>
</section>

<section class="failure-list" aria-labelledby="failure-heading">
  <header>
    <div>
      <p class="eyebrow">Needs attention</p>
      <h2 id="failure-heading">Failing runs</h2>
    </div>
    <a href={resolve('/runs')}>All runs →</a>
  </header>

  {#each failures as run (run.run_uid)}
    <a class="failure" href={resolve(`/run/${run.run_uid}`)}>
      <span class="mark" aria-hidden="true">×</span>
      <span class="identity">
        <strong>{nullStringValue(run.workflow) ?? run.project_key}</strong>
        <small>{run.device_key} · {run.commit_sha.slice(0, 8)}</small>
      </span>
      <span class="result">
        <strong
          >{run.failed_count + run.errored_count
            ? `${run.failed_count + run.errored_count} failed`
            : `${run.status} run`}</strong
        >
        <small>{run.passed_count}/{run.total_count} passed</small>
      </span>
      <span class="time"
        ><strong>{duration(run.duration_ms)}</strong><small>{when(run.started_at)}</small></span
      >
    </a>
  {:else}
    <p class="clear">No failures in the retained run list.</p>
  {/each}
</section>

{#if chartRows.length}
  <section class="history" aria-labelledby="history-heading">
    <header>
      <div>
        <p class="eyebrow">Last {recent.length} runs</p>
        <h2 id="history-heading">Failed tests per run</h2>
      </div>
    </header>
    <Chart definition={chart} height={220} ariaLabel="Failed tests in recent CI runs" />
  </section>
{/if}

<style>
  .eyebrow {
    margin: 0 0 5px;
    color: var(--focus);
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  h2 {
    margin: 0;
    font-size: 17px;
  }
  .error {
    padding: 12px 14px;
    border: 1px solid var(--love);
    color: var(--love);
  }
  .counts {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    margin-top: 24px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  .counts div {
    display: grid;
    gap: 2px;
    padding: 18px 20px;
    border-right: 1px solid var(--border);
  }
  .counts div:last-child {
    border: 0;
  }
  .counts strong {
    font-size: 23px;
  }
  .counts span,
  small {
    color: var(--muted);
    font-size: 11px;
  }
  .failure-list,
  .history {
    margin-top: 24px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  section > header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 18px;
    border-bottom: 1px solid var(--border);
  }
  header a {
    color: var(--focus);
    font-size: 12px;
    text-decoration: none;
  }
  .failure {
    position: relative;
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr) auto minmax(150px, auto);
    align-items: center;
    gap: 14px;
    padding: 13px 18px;
    border-bottom: 1px solid var(--border);
    color: inherit;
    text-decoration: none;
  }
  .failure:last-child {
    border-bottom: 0;
  }
  .failure:hover {
    background: var(--hover);
  }
  .mark {
    color: var(--love);
    font-size: 22px;
    font-weight: 700;
  }
  .identity,
  .result,
  .time {
    display: grid;
    min-width: 0;
  }
  .identity strong,
  .identity small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .result {
    color: var(--love);
    text-align: right;
  }
  .time {
    text-align: right;
  }
  .clear {
    margin: 0;
    padding: 28px 18px;
    color: var(--subtle);
  }
  .history :global(.ts-chart-host) {
    padding: 14px 16px 8px;
    color: var(--text);
  }
  @media (max-width: 720px) {
    .counts {
      grid-template-columns: 1fr;
    }
    .counts div {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
    .failure {
      grid-template-columns: 24px minmax(0, 1fr) auto;
    }
    .time {
      display: none;
    }
  }
</style>
