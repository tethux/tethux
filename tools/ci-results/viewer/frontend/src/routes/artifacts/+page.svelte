<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { getArtifact, listArtifacts } from '$lib/api/artifacts';
  import type { Artifact, ArtifactPreview } from '$lib/api/types';
  import { nullStringValue } from '$lib/api/types';
  import CommitLink from '$lib/components/CommitLink.svelte';
  import CodePreview from '$lib/components/CodePreview.svelte';
  import LogSearch from '$lib/components/LogSearch.svelte';
  import LogPreview from '$lib/components/LogPreview.svelte';
  import SearchIcon from '$lib/components/SearchIcon.svelte';
  import ChevronIcon from '$lib/components/ChevronIcon.svelte';
  import { sourceRepositories } from '$lib/repositories';

  let artifacts = $state<Artifact[]>([]);
  let nextCursor = $state('');
  let loading = $state(true);
  let loadingMore = $state(false);
  let searching = $state(false);
  let error = $state('');
  let q = $state('');
  let fileType = $state('');
  let visibility = $state('');
  let availability = $state('');
  let selected = $state<Artifact | null>(null);
  let detail = $state<ArtifactPreview | null>(null);
  let detailLoading = $state(false);
  let requestID = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  };

  const fileName = (path: string) => path.split('/').at(-1) ?? path;
  const isImage = (media: string) => media.startsWith('image/');
  const isSearchableLog = (file: Artifact) =>
    file.file_type === 'log' ||
    /(?:text|json|yaml|toml|xml|javascript)/i.test(file.media_type) ||
    /\.(?:log|txt|jsonl?)$/i.test(file.archive_path);

  async function load(reset = true): Promise<void> {
    const id = ++requestID;
    if (reset && artifacts.length === 0) loading = true;
    else if (reset) searching = true;
    else loadingMore = true;
    error = '';
    const result = await listArtifacts(fetch, {
      q: q.trim(),
      type: fileType,
      visibility,
      availability,
      cursor: reset ? '' : nextCursor,
      limit: 50
    });
    if (id !== requestID) return;
    result.match(
      (page) => {
        artifacts = reset ? page.items : [...artifacts, ...page.items];
        nextCursor = page.next_cursor;
      },
      (apiError) => {
        error = apiError.message;
      }
    );
    loading = false;
    loadingMore = false;
    searching = false;
  }

  function scheduleSearch(): void {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => load(), 220);
  }

  function submitSearch(event: SubmitEvent): void {
    event.preventDefault();
    clearTimeout(searchTimer);
    void load();
  }

  async function inspect(file: Artifact): Promise<void> {
    selected = file;
    detail = null;
    detailLoading = true;
    const result = await getArtifact(fetch, file.id);
    result.match(
      (preview) => (detail = preview),
      (apiError) => (error = apiError.message)
    );
    detailLoading = false;
  }

  function closeDetail(): void {
    selected = null;
    detail = null;
  }

  onMount(() => load());
</script>

<svelte:window onkeydown={(event) => event.key === 'Escape' && closeDetail()} />
<svelte:head><title>Artifacts · CI results</title></svelte:head>

<header class="artifact-header">
  <div>
    <p class="eyebrow">Archive index</p>
    <h1>Artifacts</h1>
    <p>Exact files retained with their runs, checksums, and provenance.</p>
  </div>
  <div class="counter"><strong>{artifacts.length}</strong><span>loaded</span></div>
</header>

<section class="artifact-tools" aria-label="Artifact filters">
  <form class="search" onsubmit={submitSearch}>
    <label>
      <span><SearchIcon size={17} /></span>
      <input bind:value={q} oninput={scheduleSearch} placeholder="Search paths, media, runs…" />
    </label>
    <button aria-label="Search artifacts" disabled={searching}>{searching ? '···' : '↵'}</button>
  </form>
  <label>
    <span>Kind</span>
    <select bind:value={fileType} onchange={() => load()}>
      <option value="">All kinds</option>
      <option value="artifact">Artifact</option>
      <option value="log">Log</option>
      <option value="config">Config</option>
      <option value="results">Results</option>
      <option value="packet_capture">Packet capture</option>
    </select>
  </label>
  <label>
    <span>Access</span>
    <select bind:value={visibility} onchange={() => load()}>
      <option value="">Public + private</option>
      <option value="public">Public</option>
      <option value="private">Private</option>
    </select>
  </label>
  <label>
    <span>Storage</span>
    <select bind:value={availability} onchange={() => load()}>
      <option value="">Any availability</option>
      <option value="available">Bytes retained</option>
      <option value="unavailable">Needs backfill</option>
    </select>
  </label>
</section>

{#if error}
  <div class="notice error" role="alert">{error}<button onclick={() => load()}>Retry</button></div>
{/if}

<section class="artifact-index" aria-busy={loading}>
  <div class="columns" aria-hidden="true">
    <span>File / run</span><span>Kind</span><span>Integrity</span><span>Size</span>
  </div>
  {#if loading}
    <div class="state"><span class="spinner"></span>Reading artifact index…</div>
  {:else if artifacts.length === 0}
    <div class="state">
      <strong>No artifacts match</strong>
      <span>Try clearing one of the filters above.</span>
    </div>
  {:else}
    <div class="rows">
      {#each artifacts as artifact (artifact.id)}
        <button class:selected={selected?.id === artifact.id} onclick={() => inspect(artifact)}>
          <span class="identity">
            <strong>{fileName(artifact.archive_path)}</strong>
            <small>{artifact.archive_path}</small>
            <small class="run">
              {nullStringValue(artifact.workflow) ?? 'run'}
              <i>·</i>
              {artifact.run_uid.slice(0, 8)}
            </small>
          </span>
          <span>
            <em>{artifact.file_type.replace('_', ' ')}</em>
            {#if !artifact.is_public}<small class="private">private</small>{/if}
          </span>
          <span class="integrity">
            {#if artifact.content_available}
              <b>✓ retained</b>
            {:else}
              <b class="missing">○ metadata only</b>
            {/if}
            <code>{artifact.sha256.slice(0, 12)}</code>
          </span>
          <span class="size"
            >{formatBytes(artifact.size_bytes)}<i><ChevronIcon size={17} /></i></span
          >
        </button>
      {/each}
    </div>
    {#if nextCursor}
      <button class="load-more" disabled={loadingMore} onclick={() => load(false)}>
        {loadingMore ? 'Loading…' : 'Load older artifacts'}
      </button>
    {/if}
  {/if}
</section>

{#if selected}
  <div class="scrim" role="presentation" onclick={closeDetail}></div>
  <aside class="drawer" aria-label="Artifact preview">
    <header>
      <div>
        <span>{selected.file_type.replace('_', ' ')}</span>
        <h2>{fileName(selected.archive_path)}</h2>
        <p>{selected.archive_path}</p>
      </div>
      <button aria-label="Close preview" onclick={closeDetail}>×</button>
    </header>
    <div class="provenance">
      <a href={resolve(`/run/${selected.run_uid}`)}>Run {selected.run_uid.slice(0, 8)}</a>
      <CommitLink hash={selected.commit_sha} repositories={sourceRepositories} />
      <span class:failed={selected.run_status !== 'passed'}>{selected.run_status}</span>
    </div>
    {#if detailLoading}
      <div class="preview-state"><span class="spinner"></span>Loading preview…</div>
    {:else if detail}
      <div class="preview-actions">
        <span>{formatBytes(selected.size_bytes)} · {selected.media_type}</span>
        {#if detail.available}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
          <a href={detail.raw_url} download={fileName(selected.archive_path)}
            >Download exact bytes</a
          >
        {/if}
      </div>
      {#if !detail.available}
        <div class="unavailable">
          <strong>Bytes not retained</strong>
          <p>Re-run ingestion against the original archive to backfill this legacy entry.</p>
        </div>
      {:else if isImage(selected.media_type)}
        <div class="image-preview">
          <img src={detail.raw_url} alt={fileName(selected.archive_path)} />
        </div>
      {:else if detail.preview !== null}
        <div class="drawer-code">
          {#if isSearchableLog(selected)}
            <LogPreview value={detail.preview} />
          {:else}
            <CodePreview value={detail.preview} />
          {/if}
        </div>
        {#if detail.truncated}<p class="truncated">
            Preview truncated. Download for the complete file.
          </p>{/if}
      {:else}
        <div class="unavailable">
          <strong>Binary artifact</strong>
          <p>Preview is unavailable for this media type. The exact bytes are ready to download.</p>
        </div>
      {/if}
      {#if detail.available && isSearchableLog(selected)}
        <LogSearch fileId={selected.id} fileName={selected.archive_path} />
      {/if}
      <dl>
        <div>
          <dt>SHA-256</dt>
          <dd><code>{selected.sha256}</code></dd>
        </div>
        <div>
          <dt>Visibility</dt>
          <dd>{selected.is_public ? 'Public' : 'Private / local only'}</dd>
        </div>
        <div>
          <dt>Stored</dt>
          <dd>{detail.synthesized ? 'Synthesized compatibility view' : 'Exact archive bytes'}</dd>
        </div>
      </dl>
    {/if}
  </aside>
{/if}

<style>
  .artifact-header {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 32px;
    padding-bottom: 24px;
    border-bottom: 1px solid var(--border);
  }
  .eyebrow {
    margin: 0 0 4px;
    color: var(--focus);
    font-size: 10px;
    font-weight: 800;
    letter-spacing: 0.16em;
    text-transform: uppercase;
  }
  .artifact-header h1 {
    font-size: 34px;
    letter-spacing: -0.04em;
  }
  .artifact-header p:not(.eyebrow) {
    margin: 6px 0 0;
    color: var(--subtle);
  }
  .counter {
    display: grid;
    min-width: 82px;
    text-align: right;
  }
  .counter strong {
    font-size: 26px;
  }
  .counter span {
    color: var(--muted);
    font-size: 11px;
    text-transform: uppercase;
  }
  .artifact-tools {
    display: grid;
    grid-template-columns: minmax(240px, 1fr) repeat(3, auto);
    gap: 10px;
    padding: 18px 0;
  }
  .artifact-tools label {
    display: grid;
    gap: 4px;
  }
  .artifact-tools label > span {
    color: var(--muted);
    font-size: 9px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }
  .artifact-tools input,
  .artifact-tools select {
    height: 38px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--text);
  }
  .artifact-tools input {
    width: 100%;
    padding: 0 12px 0 34px;
  }
  .artifact-tools select {
    padding: 0 28px 0 10px;
  }
  .search {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1fr) 38px;
    gap: 5px;
    align-self: end;
  }
  .search label {
    position: relative;
  }
  .search label > span {
    position: absolute;
    z-index: 1;
    top: 50%;
    left: 11px;
    transform: translateY(-58%);
    color: var(--focus) !important;
    font-size: 17px !important;
  }
  .search button {
    height: 38px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--focus);
    cursor: pointer;
  }
  .artifact-index {
    border: 1px solid var(--border);
    background: var(--surface);
  }
  .columns,
  .rows button {
    display: grid;
    grid-template-columns: minmax(260px, 2.1fr) minmax(110px, 0.8fr) minmax(140px, 1fr) 80px;
    gap: 16px;
    align-items: center;
  }
  .columns {
    padding: 9px 14px;
    border-bottom: 1px solid var(--border);
    color: var(--muted);
    font-size: 10px;
    text-transform: uppercase;
  }
  .rows button {
    width: 100%;
    min-height: 82px;
    padding: 12px 14px;
    border: 0;
    border-bottom: 1px solid var(--border);
    background: var(--base);
    color: var(--text);
    text-align: left;
  }
  .rows button:hover,
  .rows button.selected {
    background: var(--hover);
  }
  .rows span {
    display: grid;
    gap: 3px;
    min-width: 0;
  }
  .rows strong,
  .rows small {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .rows small {
    color: var(--muted);
    font-size: 10px;
  }
  .rows .run {
    display: flex;
    gap: 5px;
    color: var(--subtle);
  }
  .rows em {
    width: max-content;
    color: var(--syntax-blue);
    font-size: 11px;
    font-style: normal;
    text-transform: capitalize;
  }
  .rows .private {
    width: max-content;
    padding: 1px 5px;
    border: 1px solid var(--gold);
    color: var(--gold);
  }
  .integrity b {
    color: var(--syntax-green);
    font-size: 11px;
  }
  .integrity b.missing {
    color: var(--gold);
  }
  .integrity code {
    color: var(--muted);
    font-size: 10px;
  }
  .size {
    display: flex !important;
    grid-template-columns: 1fr auto;
    color: var(--subtle);
    font-size: 11px;
  }
  .size i {
    color: var(--focus);
    font-size: 20px;
    font-style: normal;
  }
  .state,
  .preview-state {
    display: flex;
    min-height: 190px;
    align-items: center;
    justify-content: center;
    gap: 10px;
    color: var(--subtle);
  }
  .state {
    flex-direction: column;
  }
  .spinner {
    width: 14px;
    height: 14px;
    border: 2px solid var(--border);
    border-top-color: var(--focus);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .load-more {
    width: 100%;
    padding: 13px;
    border: 0;
    background: var(--surface);
    color: var(--focus);
  }
  .notice {
    display: flex;
    justify-content: space-between;
    margin-bottom: 12px;
    padding: 10px 12px;
    border: 1px solid var(--love);
    color: var(--love);
  }
  .scrim {
    position: fixed;
    z-index: 20;
    inset: 0;
    background: rgb(0 0 0 / 32%);
    backdrop-filter: blur(2px);
  }
  .drawer {
    position: fixed;
    z-index: 21;
    inset: 0 0 0 auto;
    width: min(660px, 92vw);
    overflow: auto;
    border-left: 1px solid var(--border);
    background: var(--base);
    box-shadow: -20px 0 60px rgb(0 0 0 / 22%);
  }
  .drawer > header {
    display: flex;
    justify-content: space-between;
    gap: 20px;
    padding: 26px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .drawer header span {
    color: var(--focus);
    font-size: 10px;
    text-transform: uppercase;
  }
  .drawer h2 {
    margin: 5px 0;
    font-size: 22px;
    overflow-wrap: anywhere;
  }
  .drawer header p {
    margin: 0;
    color: var(--muted);
    font-size: 11px;
    overflow-wrap: anywhere;
  }
  .drawer header button {
    width: 34px;
    height: 34px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text);
    font-size: 22px;
  }
  .provenance,
  .preview-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    padding: 13px 26px;
    border-bottom: 1px solid var(--border);
  }
  .provenance a,
  .preview-actions a {
    color: var(--syntax-blue);
  }
  .provenance > span {
    color: var(--syntax-green);
  }
  .provenance > span.failed {
    color: var(--love);
  }
  .preview-actions span {
    color: var(--subtle);
    font-size: 11px;
  }
  .drawer-code {
    margin: 20px 26px;
  }
  .unavailable {
    margin: 24px 26px;
    padding: 22px;
    border: 1px dashed var(--border);
    background: var(--surface);
  }
  .unavailable p,
  .truncated {
    color: var(--subtle);
  }
  .image-preview {
    display: grid;
    margin: 20px 26px;
    place-items: center;
    min-height: 280px;
    background: var(--surface);
  }
  .image-preview img {
    max-width: 100%;
    max-height: 60vh;
  }
  .truncated {
    margin: -8px 26px 20px;
    font-size: 11px;
  }
  dl {
    margin: 26px;
    border-top: 1px solid var(--border);
  }
  dl div {
    display: grid;
    grid-template-columns: 110px 1fr;
    gap: 18px;
    padding: 10px 0;
    border-bottom: 1px solid var(--border);
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  @media (max-width: 850px) {
    .artifact-tools {
      grid-template-columns: 1fr 1fr;
    }
    .search {
      grid-column: 1 / -1;
    }
    .columns {
      display: none;
    }
    .rows button {
      grid-template-columns: 1fr auto;
    }
    .rows .integrity {
      display: none;
    }
  }
</style>
