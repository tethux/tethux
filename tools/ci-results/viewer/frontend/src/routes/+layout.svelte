<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';

  let { children } = $props();
  let dark = $state(false);

  const links = [
    { href: '/', label: 'Summary' },
    { href: '/query', label: 'Query builder' },
    { href: '/runs', label: 'Runs' },
    { href: '/tests', label: 'Tests' },
    { href: '/artifacts', label: 'Artifacts' }
  ] as const;

  onMount(() => {
    dark = localStorage.getItem('ci-results-theme') === 'dark';
    document.documentElement.classList.toggle('dark', dark);
  });

  function toggleTheme(): void {
    dark = !dark;
    document.documentElement.classList.toggle('dark', dark);
    localStorage.setItem('ci-results-theme', dark ? 'dark' : 'light');
  }
</script>

<div class="app">
  <aside>
    <strong>CI results</strong>
    <nav aria-label="Viewer sections">
      {#each links as link (link.href)}
        <a
          href={resolve(link.href)}
          class:active={page.url.pathname === link.href}
          aria-current={page.url.pathname === link.href ? 'page' : undefined}>{link.label}</a
        >
      {/each}
    </nav>
    <footer>
      <a
        href="https://github.com/tethux/tethux/tree/master/tools/ci-results/viewer"
        target="_blank"
        rel="noreferrer"
        aria-label="View CI results viewer source on GitHub"
      >
        <svg aria-hidden="true" viewBox="0 0 24 24">
          <path
            d="M12 .3a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2.1c-3.3.7-4-1.4-4-1.4-.5-1.4-1.3-1.8-1.3-1.8-1.1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1.1 1.8 2.8 1.3 3.5 1 .1-.8.4-1.3.8-1.6-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2-.1-.3-.5-1.5.1-3.2 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0c2.3-1.5 3.3-1.2 3.3-1.2.6 1.7.2 2.9.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.4.4.8 1.1.8 2.2v3.3c0 .3.2.7.8.6A12 12 0 0 0 12 .3Z"
          />
        </svg>
        <span>Source</span>
      </a>
      <button
        type="button"
        aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        title={dark ? 'Switch to light mode' : 'Switch to dark mode'}
        onclick={toggleTheme}
      >
        {#if dark}
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none"
            ><path
              d="M12 3v2.25m6.36.39-1.59 1.59M21 12h-2.25m-.39 6.36-1.59-1.59M12 18.75V21m-4.77-4.23-1.59 1.59M5.25 12H3m4.23-4.77L5.64 5.64M15.75 12a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0Z"
              stroke="currentColor"
              stroke-width="1.5"
            /></svg
          >
        {:else}
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none"
            ><path
              d="M21.75 15A9.72 9.72 0 0 1 18 15.75 9.75 9.75 0 0 1 9 2.25 9.75 9.75 0 1 0 21.75 15Z"
              stroke="currentColor"
              stroke-width="1.5"
            /></svg
          >
        {/if}
      </button>
    </footer>
  </aside>
  <main class:workspace-main={page.url.pathname === '/query'}>{@render children()}</main>
</div>

<style>
  :global(*) {
    box-sizing: border-box;
  }
  :global(:root) {
    color-scheme: light;
    --base: #eff1f5;
    --surface: #e6e9ef;
    --overlay: #ccd0da;
    --text: #4c4f69;
    --subtle: #5c5f77;
    --muted: #9ca0b0;
    --border: #bcc0cc;
    --hover: rgb(234 118 203 / 16%);
    --focus: #ea76cb;
    --love: #d20f39;
    --gold: #df8e1d;
    --syntax-mauve: #8839ef;
    --syntax-green: #40a02b;
    --syntax-peach: #fe640b;
    --syntax-blue: #1e66f5;
    --syntax-overlay: #7c7f93;
  }
  :global(html.dark) {
    color-scheme: dark;
    --base: #1e1e2e;
    --surface: #181825;
    --overlay: #313244;
    --text: #cdd6f4;
    --subtle: #bac2de;
    --muted: #6c7086;
    --border: #45475a;
    --hover: rgb(245 194 231 / 16%);
    --focus: #f5c2e7;
    --love: #f38ba8;
    --gold: #f9e2af;
    --syntax-mauve: #cba6f7;
    --syntax-green: #a6e3a1;
    --syntax-peach: #fab387;
    --syntax-blue: #89b4fa;
    --syntax-overlay: #a6adc8;
  }
  :global(body) {
    margin: 0;
    color: var(--text);
    background: var(--base);
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
  :global(h1) {
    margin: 0;
    font-size: 25px;
  }
  :global(.lede) {
    color: var(--subtle);
    margin: 7px 0 0;
  }
  :global(.page-header) {
    border-bottom: 1px solid var(--border);
    padding-bottom: 20px;
  }
  :global(.panel) {
    margin-top: 24px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  .app {
    min-height: 100vh;
    display: grid;
    grid-template-columns: 210px 1fr;
  }
  aside {
    padding: 24px 18px;
    border-right: 1px solid var(--border);
    background: var(--surface);
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
  nav a {
    padding: 7px 9px;
    color: var(--subtle);
    text-decoration: none;
    border-radius: 3px;
  }
  nav a:hover {
    color: var(--text);
    background: var(--hover);
  }
  nav a.active {
    color: var(--text);
    background: var(--overlay);
  }
  footer {
    margin-top: auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding-top: 18px;
    border-top: 1px solid var(--border);
  }
  footer a {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    color: var(--subtle);
    font-size: 11px;
    text-decoration: none;
  }
  footer a:hover {
    color: var(--focus);
  }
  footer button {
    display: grid;
    width: 32px;
    height: 32px;
    place-items: center;
    padding: 0;
    border: 1px solid transparent;
    border-radius: 6px;
    background: transparent;
    color: var(--subtle);
  }
  footer button:hover {
    border-color: var(--border);
    background: var(--hover);
    color: var(--focus);
  }
  footer svg {
    width: 16px;
    height: 16px;
    fill: currentColor;
  }
  footer button svg {
    fill: none;
  }
  footer a:focus-visible,
  footer button:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  main {
    width: min(1100px, 100%);
    padding: 36px 42px 70px;
    overflow: hidden;
  }
  main.workspace-main {
    width: 100%;
    min-width: 0;
    padding: 0;
  }
  @media (max-width: 700px) {
    .app {
      grid-template-columns: 1fr;
    }
    aside {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
    nav {
      grid-template-columns: repeat(5, auto);
      margin-top: 18px;
      overflow-x: auto;
    }
    main {
      padding: 24px 18px 50px;
    }
  }
</style>
