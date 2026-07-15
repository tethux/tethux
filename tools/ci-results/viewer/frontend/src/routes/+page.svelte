<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';

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

  let output = $derived(
    error ? `Error: ${error}` : summary ? JSON.stringify(summary, null, 2) : 'Loading…'
  );
</script>

<svelte:head><title>CI results</title></svelte:head>

<div class="app">
  <aside>
    <strong>CI results</strong>
    <nav aria-label="Viewer sections">
      <a class="active" href={resolve('/')}>Summary</a>
      <span>Runs</span>
      <span>Tests</span>
      <span>Artifacts</span>
    </nav>
    <small>Local SQLite viewer</small>
  </aside>

  <main>
    <header>
      <div>
        <h1>Results viewer</h1>
        <p>A small starting point for browsing ingested test data.</p>
      </div>
      <span class:error>{error ? 'offline' : 'connected'}</span>
    </header>

    <section class="counts" aria-label="Result totals">
      <div><span>Runs</span><strong>{summary?.run_count ?? '—'}</strong></div>
      <div><span>Tests</span><strong>{summary?.test_count ?? '—'}</strong></div>
      <div><span>Passed</span><strong>{summary?.passed_count ?? '—'}</strong></div>
      <div><span>Other</span><strong>{summary?.failed_count ?? '—'}</strong></div>
    </section>

    <section class="panel">
      <label for="query">Query builder</label>
      <p>This input is only a placeholder. It does not execute anything yet.</p>
      <div class="query-row">
        <input id="query" bind:value={query} placeholder="status = passed and branch = main" />
        <button disabled title="Query execution is not implemented">Run query</button>
      </div>
    </section>

    <section class="panel">
      <label for="output">Output</label>
      <p>Raw response from <code>GET /api/v1/summary</code>.</p>
      <textarea id="output" readonly value={output}></textarea>
    </section>
  </main>
</div>

<style>
  :global(*) {
    box-sizing: border-box;
  }
  :global(body) {
    margin: 0;
    color: #222;
    background: #f5f5f3;
    font:
      14px/1.5 ui-monospace,
      SFMono-Regular,
      Menlo,
      Consolas,
      monospace;
  }
  :global(button),
  :global(input),
  :global(textarea) {
    font: inherit;
  }
  .app {
    min-height: 100vh;
    display: grid;
    grid-template-columns: 210px 1fr;
  }
  aside {
    padding: 24px 18px;
    border-right: 1px solid #ccc;
    background: #ececea;
    display: flex;
    flex-direction: column;
  }
  aside strong {
    font-size: 16px;
  }
  nav {
    display: grid;
    gap: 4px;
    margin-top: 30px;
  }
  nav a,
  nav span {
    padding: 7px 9px;
    color: #777;
    text-decoration: none;
    border-radius: 3px;
  }
  nav .active {
    color: #111;
    background: #dcdcd8;
  }
  aside small {
    margin-top: auto;
    color: #777;
  }
  main {
    width: min(960px, 100%);
    padding: 36px 42px 70px;
  }
  header {
    display: flex;
    justify-content: space-between;
    gap: 24px;
    align-items: flex-start;
    border-bottom: 1px solid #ccc;
    padding-bottom: 20px;
  }
  h1 {
    margin: 0;
    font-size: 25px;
  }
  header p,
  .panel p {
    color: #666;
    margin: 7px 0 0;
  }
  header > span {
    padding: 4px 8px;
    background: #dbead7;
    color: #315c2b;
    border: 1px solid #b9d0b4;
    border-radius: 3px;
    font-size: 12px;
  }
  header > span.error {
    background: #f0d8d5;
    color: #7b3028;
    border-color: #deb9b4;
  }
  .counts {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    border: 1px solid #ccc;
    margin-top: 26px;
    background: #fff;
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
    font-weight: 600;
  }
  .panel {
    margin-top: 24px;
    padding: 20px;
    border: 1px solid #ccc;
    background: #fff;
  }
  .panel label {
    font-weight: 600;
  }
  .query-row {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 8px;
    margin-top: 16px;
  }
  input,
  textarea {
    width: 100%;
    border: 1px solid #aaa;
    border-radius: 3px;
    background: #fff;
    color: #222;
    padding: 10px;
  }
  input:focus,
  textarea:focus {
    outline: 2px solid #8ba9c7;
    outline-offset: 1px;
  }
  button {
    border: 1px solid #bbb;
    border-radius: 3px;
    padding: 0 18px;
    color: #888;
    background: #eee;
  }
  textarea {
    min-height: 210px;
    resize: vertical;
    margin-top: 16px;
  }
  code {
    padding: 1px 4px;
    background: #eee;
  }
  @media (max-width: 700px) {
    .app {
      grid-template-columns: 1fr;
    }
    aside {
      border-right: 0;
      border-bottom: 1px solid #ccc;
    }
    aside small {
      display: none;
    }
    nav {
      grid-template-columns: repeat(4, auto);
      margin-top: 18px;
      overflow-x: auto;
    }
    main {
      padding: 24px 18px 50px;
    }
    .counts {
      grid-template-columns: repeat(2, 1fr);
    }
    .counts div:nth-child(2) {
      border-right: 0;
    }
    .counts div:nth-child(-n + 2) {
      border-bottom: 1px solid #ddd;
    }
    .query-row {
      grid-template-columns: 1fr;
    }
    button {
      min-height: 40px;
    }
  }
</style>
