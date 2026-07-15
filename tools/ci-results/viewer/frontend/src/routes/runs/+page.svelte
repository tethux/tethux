<script lang="ts">
  import VirtualList from '@humanspeak/svelte-virtual-list';
  import CommitLink from '$lib/components/CommitLink.svelte';
  import { sourceRepositories } from '$lib/repositories';
  import type { Run } from '$lib/api/types';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<svelte:head>
  <title>Runs · CI results</title>
</svelte:head>

<header class="page-header">
  <h1>Runs</h1>
  <p class="lede">Virtualized history of {data.runs.length} ingested runs.</p>
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
          <article>
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
                <span>{run.branch.Valid ? run.branch.String : 'detached'}</span>
              </span>
            </div>

            <div>
              <strong>{run.passed_count}/{run.total_count}</strong>

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
  article {
    display: grid;
    grid-template-columns: 1.1fr 2fr 1.5fr;
    gap: 16px;
  }

  .columns {
    padding: 9px 14px;
    color: #777;
    border-bottom: 1px solid #ccc;
    font-size: 12px;
  }

  /*
   * The virtualizer must be inside an element with a real height.
   * This replaces your old viewportHeight = 532.
   */
  .virtual-list {
    width: 100%;
    height: 532px;
    overflow: hidden;
  }

  article {
    box-sizing: border-box;
    width: 100%;
    min-height: 76px;
    align-items: center;
    padding: 10px 14px;
    border-bottom: 1px solid #ddd;
    background: #fff;
  }

  article div {
    min-width: 0;
    display: grid;
    gap: 3px;
  }

  article span {
    color: #777;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  article b {
    width: max-content;
    color: #315c2b;
  }

  article b.failed {
    color: #8a3028;
  }

  .error {
    color: #8a3028;
  }

  .empty {
    padding: 20px;
    color: #777;
  }

  @media (max-width: 700px) {
    .columns {
      display: none;
    }

    .virtual-list {
      height: 70vh;
      min-height: 400px;
    }

    article {
      grid-template-columns: 1fr;
      min-height: 112px;
    }
  }
  .source {
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
  }
</style>
