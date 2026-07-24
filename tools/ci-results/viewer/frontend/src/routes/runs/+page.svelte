<script lang="ts">
  import VirtualList from '@humanspeak/svelte-virtual-list';
  import { resolve } from '$app/paths';
  import CommitLink from '$lib/components/CommitLink.svelte';
  import { sourceRepositories } from '$lib/repositories';
  import { nullStringValue, type Run } from '$lib/api/types';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<svelte:head>
  <title>Runs · CI results</title>
</svelte:head>

<header class="page-header">
  <h1>Runs</h1>

  <p class="lede">
    Virtualized history of {data.runs.length} ingested runs.
  </p>
</header>

{#if data.error}
  <p class="error">{data.error}</p>
{/if}

<section class="run-list panel" aria-label="Run history">
  <div class="columns">
    <span>Status / run</span>
    <span>Source</span>
    <span>Results</span>
  </div>

  {#if data.runs.length === 0}
    <p class="empty">No runs found.</p>
  {:else}
    <div class="virtual-list">
      <VirtualList
        items={data.runs}
        defaultEstimatedItemHeight={76}
        bufferSize={8}
        hasMore={false}
        viewportLabel="Run history"
      >
        {#snippet renderItem(run: Run)}
          <article class="run-row">
            <a
              class="row-link"
              href={resolve(`/run/${run.run_uid}`)}
              aria-label={`Open run ${run.run_uid}`}
            ></a>

            <div>
              <b class:failed={run.status !== 'passed'}>
                {run.status}
              </b>

              <CommitLink hash={run.commit_sha} repositories={sourceRepositories} />
            </div>

            <div>
              <strong>{run.project_key}</strong>

              <span class="source">
                <span>{run.device_key}</span>
                <span>·</span>
                <span>{nullStringValue(run.branch) ?? 'detached'}</span>
              </span>
            </div>

            <div>
              <strong>
                {run.passed_count}/{run.total_count}
              </strong>

              <span>
                {run.duration_ms} ms · {run.started_at}
              </span>
            </div>
          </article>
        {/snippet}
      </VirtualList>
    </div>
  {/if}
</section>

<style>
  .run-list {
    overflow: hidden;
  }

  .columns,
  .run-row {
    display: grid;
    grid-template-columns: 1.1fr 2fr 1.5fr;
    gap: 16px;
  }

  .columns {
    padding: 9px 14px;
    color: var(--subtle);
    border-bottom: 1px solid var(--border);
    font-size: 12px;
  }

  .virtual-list {
    width: 100%;
    height: 532px;
    overflow: hidden;
  }

  .run-row {
    position: relative;
    box-sizing: border-box;
    width: 100%;
    min-height: 76px;
    align-items: center;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
    background: var(--base);
    cursor: pointer;
  }

  .run-row:hover {
    background: var(--hover);
  }

  .row-link {
    position: absolute;
    inset: 0;
    z-index: 1;
  }

  .run-row > div {
    position: relative;
    z-index: 1;
    display: grid;
    min-width: 0;
    gap: 3px;
    pointer-events: none;
  }

  .run-row :global(a:not(.row-link)) {
    position: relative;
    z-index: 2;
    pointer-events: auto;
  }

  .run-row:has(.row-link:focus-visible) {
    outline: 2px solid currentColor;
    outline-offset: -2px;
  }

  .run-row span {
    overflow: hidden;
    color: var(--subtle);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .run-row b {
    width: max-content;
    color: var(--syntax-green);
  }

  .run-row b.failed {
    color: var(--love);
  }

  .source {
    display: flex;
    align-items: center;
    min-width: 0;
    gap: 5px;
  }

  .error {
    color: var(--love);
  }

  .empty {
    padding: 20px;
    color: var(--subtle);
  }

  @media (max-width: 700px) {
    .columns {
      display: none;
    }

    .virtual-list {
      height: 70vh;
      min-height: 400px;
    }

    .run-row {
      grid-template-columns: 1fr;
      min-height: 112px;
    }
  }
</style>
