<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { getRuns } from '$lib/api/runs';
  import { getSummary } from '$lib/api/summary';
  import { nullStringValue, type Run, type ViewerSummary } from '$lib/api/types';
  import CommitLink from '$lib/components/CommitLink.svelte';
  import { sourceRepositories } from '$lib/repositories';

  let summary = $state<ViewerSummary | null>(null);
  let runs = $state<Run[]>([]);
  let error = $state('');
  let loading = $state(true);
  let focusedRun = $state<Run | null>(null);

  const recent = $derived(runs.slice(0, 12));
  const durationPeak = $derived(Math.max(...recent.map((run) => run.duration_ms), 1));
  const successRate = $derived(
    recent.length
      ? Math.round((recent.filter((run) => run.status === 'passed').length / recent.length) * 100)
      : 0
  );
  const medianDuration = $derived.by(() => {
    if (!recent.length) return 0;
    const values = recent.map((run) => run.duration_ms).sort((a, b) => a - b);
    return values[Math.floor(values.length / 2)];
  });
  const passedRuns = $derived(recent.filter((run) => run.status === 'passed').length);
  const totalTests = $derived(recent.reduce((sum, run) => sum + run.total_count, 0));
  const passedTests = $derived(recent.reduce((sum, run) => sum + run.passed_count, 0));
  const failedTests = $derived(
    recent.reduce((sum, run) => sum + run.failed_count + run.errored_count, 0)
  );

  onMount(async () => {
    const [summaryResult, runsResult] = await Promise.all([getSummary(fetch), getRuns(fetch)]);
    summaryResult.match(
      (value) => (summary = value),
      (apiError) => (error = apiError.message)
    );
    runsResult.match(
      (value) => (runs = value),
      (apiError) => (error = apiError.message)
    );
    loading = false;
  });

  const duration = (ms: number) =>
    ms >= 60_000 ? `${(ms / 60_000).toFixed(1)}m` : `${(ms / 1000).toFixed(1)}s`;
  const relative = (value: string) => {
    const hours = (new Date(value).getTime() - Date.now()) / 3_600_000;
    return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(
      Math.round(Math.abs(hours) >= 48 ? hours / 24 : hours),
      Math.abs(hours) >= 48 ? 'day' : 'hour'
    );
  };
</script>

<svelte:head><title>Summary · CI results</title></svelte:head>

<header class="page-header dashboard-header">
  <div>
    <p class="kicker">Archive health</p>
    <h1>Project pulse</h1>
    <p class="lede">The smallest useful picture of build health, speed, and recent change.</p>
  </div>
  <a class="query-link" href={resolve('/query')}>Explore data <span>⌘</span></a>
</header>

{#if error}<p class="summary-error">Unable to load dashboard: {error}</p>{/if}

<section class="health-grid" aria-label="CI health summary">
  <article class="health-score">
    <div>
      <span>Recent success</span>
      <strong>{loading ? '—' : `${successRate}%`}</strong>
      <small>last {recent.length} archived runs</small>
    </div>
    <div class="status-track" aria-label={`${successRate}% recent success rate`}>
      {#each recent as run (run.run_uid)}
        <a
          class:failed={run.status !== 'passed'}
          href={resolve(`/run/${run.run_uid}`)}
          title={`${run.status} · ${duration(run.duration_ms)}`}
          aria-label={`Open ${run.status} run`}
        ></a>
      {/each}
    </div>
  </article>
  <article>
    <span>Runs archived</span><strong>{summary?.run_count ?? '—'}</strong><small
      >exact, local history</small
    >
  </article>
  <article>
    <span>Test observations</span><strong>{summary?.test_count ?? '—'}</strong><small
      >{summary?.failed_count ?? '—'} need attention</small
    >
  </article>
  <article>
    <span>Median duration</span><strong>{recent.length ? duration(medianDuration) : '—'}</strong
    ><small>recent archived runs</small>
  </article>
</section>

<div class="dashboard-grid">
  <section class="panel duration-panel" aria-labelledby="duration-title">
    <header>
      <div>
        <p>Performance</p>
        <h2 id="duration-title">Build duration</h2>
      </div>
      <small>latest →</small>
    </header>
    <div class="chart-frame">
      <div class="y-axis" aria-hidden="true">
        <span>{duration(durationPeak)}</span><span>{duration(durationPeak / 2)}</span><span>0</span>
      </div>
      <div class="duration-chart">
        {#each [...recent].reverse() as run (run.run_uid)}
          <a
            href={resolve(`/run/${run.run_uid}`)}
            onmouseenter={() => (focusedRun = run)}
            onmouseleave={() => (focusedRun = null)}
            onfocus={() => (focusedRun = run)}
            onblur={() => (focusedRun = null)}
            aria-label={`Open ${run.status} run lasting ${duration(run.duration_ms)}`}
          >
            <i
              class:failed={run.status !== 'passed'}
              style:height={`${Math.max((run.duration_ms / durationPeak) * 100, 6)}%`}
            ></i>
          </a>
        {/each}
        {#if focusedRun}
          <div class="chart-tooltip" role="status">
            <strong>{duration(focusedRun.duration_ms)}</strong>
            <span>{focusedRun.status} · {focusedRun.device_key}</span>
            <small>{nullStringValue(focusedRun.branch) ?? focusedRun.commit_sha.slice(0, 8)}</small>
          </div>
        {/if}
      </div>
    </div>
    <div class="micro-insights">
      <div
        class="donut"
        style={`--rate: ${successRate * 3.6}deg`}
        aria-label={`${passedRuns} of ${recent.length} runs passed`}
      ><span>{successRate}%</span></div>
      <div class="test-mix">
        <span>Recent test outcomes</span>
        <div aria-label={`${passedTests} passed and ${failedTests} failed tests`}>
          <i style:width={`${totalTests ? (passedTests / totalTests) * 100 : 0}%`}></i>
          <b style:width={`${totalTests ? (failedTests / totalTests) * 100 : 0}%`}></b>
        </div>
        <small>{passedTests} passed · {failedTests} attention</small>
      </div>
    </div>
  </section>

  <section class="panel recent-panel" aria-labelledby="recent-title">
    <header>
      <div>
        <p>Now</p>
        <h2 id="recent-title">Recent runs</h2>
      </div>
      <a class="history-link" href={resolve('/runs')}>View history <span>→</span></a>
    </header>
    {#if loading}
      <div class="skeleton" aria-label="Loading recent runs"></div>
    {:else}
      {#each recent.slice(0, 6) as run (run.run_uid)}
        <article>
          <a
            class="run-target"
            href={resolve(`/run/${run.run_uid}`)}
            aria-label={`Open ${run.status} run`}
          ></a>
          <b class:failed={run.status !== 'passed'}>{run.status === 'passed' ? '✓' : '×'}</b>
          <div>
            <strong><CommitLink hash={run.commit_sha} repositories={sourceRepositories} /></strong>
            <small>{run.project_key} · {run.device_key}</small>
          </div>
          <span>{duration(run.duration_ms)}<small>{relative(run.started_at)}</small></span>
        </article>
      {:else}
        <p class="empty">No archived runs yet.</p>
      {/each}
    {/if}
  </section>
</div>

<style>
  .dashboard-header {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 24px;
  }
  .kicker,
  .duration-panel header p,
  .recent-panel header p {
    margin: 0 0 4px;
    color: var(--muted);
    font-size: 9px;
    font-weight: 750;
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }
  .query-link {
    display: inline-flex;
    align-items: center;
    gap: 14px;
    padding: 9px 11px;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--base);
    color: var(--text);
    text-decoration: none;
  }
  .query-link span {
    color: var(--muted);
  }
  .summary-error {
    padding: 10px 12px;
    border: 1px solid color-mix(in srgb, var(--love) 35%, var(--border));
    color: var(--love);
  }
  .health-grid {
    display: grid;
    grid-template-columns: 1.6fr repeat(3, 1fr);
    margin-bottom: 18px;
    border: 1px solid var(--border);
    background: var(--base);
  }
  .health-grid article {
    display: grid;
    align-content: center;
    min-height: 112px;
    padding: 16px;
    border-right: 1px solid var(--border);
  }
  .health-grid article:last-child {
    border-right: 0;
  }
  .health-grid span,
  .health-grid small {
    color: var(--muted);
    font-size: 10px;
  }
  .health-grid strong {
    margin: 5px 0 3px;
    color: var(--text);
    font-size: 24px;
  }
  .health-score {
    grid-template-columns: auto minmax(100px, 1fr);
    align-items: center;
    gap: 18px;
  }
  .health-score > div:first-child {
    display: grid;
  }
  .status-track {
    display: flex;
    align-items: stretch;
    height: 48px;
    gap: 3px;
  }
  .status-track a {
    flex: 1;
    min-width: 4px;
    border-radius: 1px;
    background: var(--syntax-green);
  }
  .status-track a.failed {
    background: var(--love);
  }
  .dashboard-grid {
    display: grid;
    grid-template-columns: minmax(280px, 0.8fr) minmax(360px, 1.2fr);
    gap: 18px;
  }
  .duration-panel,
  .recent-panel {
    min-width: 0;
    overflow: hidden;
  }
  .duration-panel header,
  .recent-panel > header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 62px;
    padding: 0 16px;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    font-size: 14px;
  }
  .duration-panel header > small,
  .recent-panel header a {
    color: var(--muted);
    font-size: 10px;
  }
  .chart-frame {
    display: grid;
    grid-template-columns: 48px minmax(0, 1fr);
    height: 190px;
  }
  .y-axis {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: 20px 8px 8px 0;
    color: var(--muted);
    font-size: 8px;
    text-align: right;
  }
  .duration-chart {
    position: relative;
    display: flex;
    align-items: end;
    gap: 5px;
    padding: 20px 14px 8px 0;
    background-image: linear-gradient(var(--border) 1px, transparent 1px);
    background-size: 100% 33.333%;
  }
  .duration-chart a {
    display: flex;
    height: 100%;
    flex: 1;
    align-items: end;
  }
  .duration-chart i {
    width: 100%;
    min-height: 4px;
    background: var(--syntax-blue);
    transition: filter 120ms ease;
  }
  .duration-chart a:hover i {
    filter: brightness(1.25);
  }
  .duration-chart a:focus-visible {
    outline: 2px solid var(--syntax-blue);
    outline-offset: 2px;
  }
  .duration-chart i.failed {
    background: var(--love);
  }
  .chart-tooltip {
    position: absolute;
    z-index: 2;
    top: 10px;
    left: 10px;
    display: grid;
    gap: 2px;
    min-width: 132px;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 2px;
    background: var(--surface);
    box-shadow: 0 8px 24px color-mix(in srgb, #000 20%, transparent);
    pointer-events: none;
  }
  .chart-tooltip strong { font-size: 13px; }
  .chart-tooltip span,
  .chart-tooltip small { color: var(--muted); font-size: 9px; }
  .micro-insights {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: center;
    gap: 14px;
    padding: 13px 16px;
    border-top: 1px solid var(--border);
  }
  .donut {
    display: grid;
    width: 48px;
    height: 48px;
    place-items: center;
    border-radius: 50%;
    background: conic-gradient(var(--syntax-green) var(--rate), var(--border) 0);
  }
  .donut::before {
    grid-area: 1 / 1;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: var(--base);
    content: '';
  }
  .donut span {
    z-index: 1;
    font-size: 9px;
    font-weight: 800;
  }
  .test-mix { display: grid; gap: 6px; }
  .test-mix > span,
  .test-mix small { color: var(--muted); font-size: 9px; }
  .test-mix > div {
    display: flex;
    height: 7px;
    overflow: hidden;
    border-radius: 8px;
    background: var(--border);
  }
  .test-mix i { background: var(--syntax-green); }
  .test-mix b { background: var(--love); }
  .history-link {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 7px 9px;
    border: 1px solid var(--border);
    border-radius: 2px;
    background: var(--surface);
    color: var(--text) !important;
    font-weight: 700;
    text-decoration: none;
  }
  .history-link:hover { border-color: var(--syntax-blue); }
  .history-link span { color: var(--syntax-blue); }
  .recent-panel article {
    position: relative;
    display: grid;
    grid-template-columns: 24px minmax(0, 1fr) auto;
    align-items: center;
    min-height: 56px;
    gap: 10px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border);
  }
  .recent-panel article:last-child {
    border-bottom: 0;
  }
  .recent-panel article:hover {
    background: var(--hover);
  }
  .run-target {
    position: absolute;
    inset: 0;
  }
  .recent-panel b {
    display: grid;
    width: 20px;
    height: 20px;
    place-items: center;
    border-radius: 50%;
    background: color-mix(in srgb, var(--syntax-green) 14%, transparent);
    color: var(--syntax-green);
  }
  .recent-panel b.failed {
    background: color-mix(in srgb, var(--love) 14%, transparent);
    color: var(--love);
  }
  .recent-panel article > div,
  .recent-panel article > span {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .recent-panel article > span {
    justify-items: end;
    color: var(--text);
    font-size: 10px;
  }
  .recent-panel small {
    overflow: hidden;
    color: var(--muted);
    font-size: 9px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .recent-panel :global(a:not(.run-target)) {
    position: relative;
    z-index: 1;
  }
  .skeleton {
    height: 336px;
    background: repeating-linear-gradient(
      180deg,
      var(--base) 0,
      var(--base) 55px,
      var(--border) 56px
    );
    animation: pulse 1.2s ease-in-out infinite alternate;
  }
  .empty {
    padding: 24px;
    color: var(--muted);
  }
  @keyframes pulse {
    to {
      opacity: 0.55;
    }
  }
  @media (max-width: 860px) {
    .health-grid {
      grid-template-columns: repeat(2, 1fr);
    }
    .health-grid article:nth-child(2) {
      border-right: 0;
    }
    .health-grid article:nth-child(-n + 2) {
      border-bottom: 1px solid var(--border);
    }
    .dashboard-grid {
      grid-template-columns: 1fr;
    }
  }
  @media (max-width: 560px) {
    .dashboard-header {
      align-items: start;
      flex-direction: column;
    }
    .health-grid {
      grid-template-columns: 1fr;
    }
    .health-grid article {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
    .health-score {
      grid-template-columns: 1fr;
    }
  }
</style>
