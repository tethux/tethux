<script lang="ts">
  import VirtualList from '@humanspeak/svelte-virtual-list';
  import type { Test } from '$lib/api/types';
  import { nullStringValue } from '$lib/api/types';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
  let filter = $state('');

  let shown = $derived(
    data.tests.filter((test) =>
      `${test.test_key} ${test.name} ${nullStringValue(test.suite) ?? ''}`
        .toLowerCase()
        .includes(filter.trim().toLowerCase())
    )
  );
  $effect(() => {
    console.log('tests:', data.tests);
    console.log('first suite:', data.tests[0]?.suite);
  });
</script>

<svelte:head>
  <title>Tests · CI results</title>
</svelte:head>

<header class="page-header">
  <h1>Tests</h1>
  <p class="lede">All known tests and their accumulated result counts.</p>
</header>

<div class="toolbar">
  <input bind:value={filter} placeholder="Filter tests…" aria-label="Filter tests" />

  <span>{shown.length} tests</span>
</div>

{#if data.error}
  <p class="error">{data.error}</p>
{/if}

<section class="tests panel">
  <div class="heading">
    <span>Test</span>
    <span>Kind</span>
    <span>Results</span>
  </div>

  {#if shown.length === 0}
    <p class="empty">No tests match.</p>
  {:else}
    <div class="list">
      <VirtualList
        items={shown}
        defaultEstimatedItemHeight={64}
        bufferSize={10}
        viewportLabel="Test results"
        hasMore={false}
      >
        {#snippet renderItem(test: Test)}
          <article>
            <div>
              <strong>{test.name}</strong>
              <code>{test.test_key}</code>
            </div>

            <div>
              <span>{nullStringValue(test.suite) ?? 'no suite'}</span>
              <!-- <small>{nullStringValue(test.suite) ?? 'no suite'}</small> -->
            </div>

            <div>
              <strong>{test.passed_count} passed</strong>

              <small class:failed={test.failed_count > 0}>
                {test.failed_count} failed · {test.result_count} total
              </small>
            </div>
          </article>
        {/snippet}
      </VirtualList>
    </div>
  {/if}
</section>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 24px;
  }

  .toolbar input {
    flex: 1;
    min-width: 0;
    padding: 9px 11px;
    border: 1px solid #aaa;
    background: #fff;
  }

  .toolbar span {
    color: #777;
  }

  .heading,
  article {
    display: grid;
    grid-template-columns: 2fr 1fr 1fr;
    gap: 16px;
  }

  .heading {
    padding: 9px 14px;
    color: #777;
    border-bottom: 1px solid #ccc;
    font-size: 12px;
  }

  /*
   * Required for virtualization.
   *
   * The VirtualList needs a container with a real height so it can calculate
   * which rows are visible.
   */
  .list {
    width: 100%;
    height: min(70vh, 800px);
    min-height: 320px;
    overflow: hidden;
  }

  article {
    box-sizing: border-box;
    width: 100%;
    padding: 12px 14px;
    border-bottom: 1px solid #ddd;
  }

  article div {
    min-width: 0;
    display: grid;
    gap: 3px;
  }

  code,
  small,
  article span {
    color: #777;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  small.failed,
  .error {
    color: #8a3028;
  }

  .empty {
    padding: 20px;
    color: #777;
  }

  @media (max-width: 700px) {
    .heading {
      display: none;
    }

    .list {
      height: 70vh;
    }

    article {
      grid-template-columns: 1fr;
    }
  }
</style>
