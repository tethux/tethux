<script lang="ts">
  type Repository = {
    name: string;
    url: string;
  };

  type Props = {
    hash: string;
    repositories: Repository[];
    labelLength?: number;
  };

  let { hash, repositories, labelLength = 8 }: Props = $props();

  const popoverId = `commit-${crypto.randomUUID()}`;
  const anchorName = `--${popoverId}`;

  let shortHash = $derived(hash.slice(0, labelLength));
  let copied = $state(false);

  function commitUrl(repository: string) {
    return `${repository.replace(/\/$/, '')}/commit/${encodeURIComponent(hash)}`;
  }

  async function copyHash() {
    try {
      await navigator.clipboard.writeText(hash);
      copied = true;

      window.setTimeout(() => {
        copied = false;
      }, 1500);
    } catch {
      copied = false;
    }
  }
</script>

<button
  type="button"
  class="hash"
  popovertarget={popoverId}
  aria-label={`Open commit ${shortHash}`}
  style:anchor-name={anchorName}
>
  <code>{shortHash}</code>
</button>

<div id={popoverId} class="commit-popover" popover="auto" style:position-anchor={anchorName}>
  <div class="popover-header">
    <strong>Commit</strong>
    <code>{hash}</code>
  </div>

  <div class="repository-links">
    {#each repositories as repository (repository.url)}
      <!-- External repository URL, not an internal SvelteKit route. -->
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
      <a href={commitUrl(repository.url)} target="_blank" rel="noopener noreferrer">
        View on {repository.name}
      </a>
    {/each}
  </div>

  <button type="button" class="copy" onclick={copyHash}>
    {copied ? 'Copied' : 'Copy hash'}
  </button>
</div>

<style>
  .hash {
    width: max-content;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
  }

  .hash:hover code {
    text-decoration: underline;
  }

  .hash:focus-visible {
    outline: 2px solid currentColor;
    outline-offset: 3px;
  }

  .commit-popover {
    position: fixed;
    inset: auto;
    bottom: anchor(top);
    left: anchor(left);
    width: min(272px, calc(100vw - 32px));
    margin: 0 0 8px;
    padding: 12px;
    border: 1px solid #999;
    background: #fff;
    box-shadow: 0 10px 24px rgb(0 0 0 / 16%);
    position-try-fallbacks: flip-block, flip-inline;
  }

  .popover-header {
    display: grid;
    gap: 6px;
  }

  .popover-header code {
    color: #666;
    overflow-wrap: anywhere;
  }

  .repository-links {
    display: grid;
    gap: 8px;
    margin-top: 14px;
  }

  .repository-links a,
  .copy {
    padding: 8px 10px;
    border: 1px solid #aaa;
    background: #fff;
    color: inherit;
    font: inherit;
    text-align: left;
    text-decoration: none;
    cursor: pointer;
  }

  .repository-links a:hover,
  .copy:hover {
    background: #f3f3f3;
  }

  .copy {
    width: 100%;
    margin-top: 8px;
  }
</style>
