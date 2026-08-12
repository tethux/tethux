<script lang="ts">
  import { resolve } from '$app/paths';
  import type { PageData } from './$types';
  import { nullStringValue } from '$lib/api/types';

  let { data }: { data: PageData } = $props();

  const failures = $derived(data.runs.filter((run) => run.status !== 'passed').slice(0, 20));
  const commits = $derived(
    failures.reduce<Array<{ sha: string; runs: typeof failures }>>((groups, run) => {
      const group = groups.find((entry) => entry.sha === run.commit_sha);
      if (group) group.runs.push(run);
      else groups.push({ sha: run.commit_sha, runs: [run] });
      return groups;
    }, [])
  );

  const duration = (ms: number) =>
    ms >= 60_000 ? `${(ms / 60_000).toFixed(1)}m` : `${(ms / 1000).toFixed(1)}s`;
  const when = (value: string) => new Date(value).toLocaleString();
</script>

<svelte:head><title>Failures · CI results</title></svelte:head>

<header class="page-header">
  <h1>Failures</h1>
  <p class="lede">Open a run. Read the error. Check the logs.</p>
</header>

{#if data.error}<p class="error">Could not load results: {data.error}</p>{/if}

<section class="failure-list" aria-labelledby="failure-heading">
  <header>
    <h2 id="failure-heading">{failures.length} recent</h2>
    <a href={resolve('/runs')}>All runs →</a>
  </header>

  {#each commits as commit (commit.sha)}
    <div class="commit">
      <strong>{commit.sha.slice(0, 8)}</strong><span>{commit.runs.length} jobs</span>
    </div>
    {#each commit.runs as run (run.run_uid)}
      <a class="failure" href={resolve(`/run/${run.run_uid}`)}>
        <span class="identity">
          <strong>{nullStringValue(run.workflow) ?? run.project_key}</strong>
          <small>{run.device_key}</small>
        </span>
        <span class="result">
          <strong
            >{run.failed_count + run.errored_count
              ? `${run.failed_count + run.errored_count} failed`
              : run.status}</strong
          >
          <small>{run.passed_count}/{run.total_count} passed</small>
        </span>
        <span class="time"
          ><strong>{duration(run.duration_ms)}</strong><small>{when(run.started_at)}</small></span
        >
      </a>
    {/each}
  {:else}
    <p class="clear">No failures in the retained run list.</p>
  {/each}
</section>

<style>
  h2 {
    margin: 0;
    font-size: 13px;
  }
  .error {
    padding: 12px 14px;
    border: 1px solid var(--love);
    color: var(--love);
  }
  small {
    color: var(--muted);
    font-size: 11px;
  }
  .failure-list {
    margin-top: 18px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  section > header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 11px 14px;
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
    grid-template-columns: minmax(0, 1fr) auto minmax(140px, auto);
    align-items: center;
    gap: 14px;
    padding: 12px 14px;
    border-bottom: 1px solid var(--border);
    color: inherit;
    text-decoration: none;
  }
  .commit {
    display: flex;
    justify-content: space-between;
    padding: 7px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--base);
    color: var(--muted);
    font-size: 10px;
    text-transform: uppercase;
  }
  .failure:last-child {
    border-bottom: 0;
  }
  .failure:hover {
    background: var(--hover);
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
  @media (max-width: 720px) {
    .failure {
      grid-template-columns: minmax(0, 1fr) auto;
      gap: 10px;
    }
    .time {
      display: none;
    }
  }
  @media (max-width: 380px) {
    .result small {
      display: none;
    }
  }
</style>
