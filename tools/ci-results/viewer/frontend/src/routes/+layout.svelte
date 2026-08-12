<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';

  let { children } = $props();

  const links = [
    { href: '/', label: 'Failures' },
    { href: '/runs', label: 'Runs' }
  ] as const;
</script>

<header class="shell">
  <a class="brand" href={resolve('/')}>tethux ci</a>
  <nav aria-label="Viewer sections">
    {#each links as link (link.href)}
      <a
        href={resolve(link.href)}
        class:active={page.url.pathname === link.href}
        aria-current={page.url.pathname === link.href ? 'page' : undefined}>{link.label}</a
      >
    {/each}
  </nav>
</header>

<main>{@render children()}</main>

<style>
  :global(*),
  :global(*::before),
  :global(*::after) {
    box-sizing: border-box;
  }
  :global(:root) {
    color-scheme: dark;
    --base: #000;
    --surface: #080808;
    --overlay: #111;
    --text: #f2f2f2;
    --subtle: #aaa;
    --muted: #707070;
    --border: #242424;
    --hover: #101010;
    --focus: #fff;
    --love: #ff4d5f;
    --gold: #e8c66a;
    --syntax-mauve: #d19cff;
    --syntax-green: #70d98b;
    --syntax-peach: #ff9d66;
    --syntax-blue: #79b8ff;
    --syntax-overlay: #999;
  }
  :global(html) {
    max-width: 100%;
    overflow-x: hidden;
    background: var(--base);
  }
  :global(body) {
    min-width: 0;
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
    font-size: clamp(21px, 5vw, 28px);
    line-height: 1.2;
  }
  :global(.lede) {
    margin: 6px 0 0;
    color: var(--muted);
  }
  :global(.page-header) {
    padding-bottom: 18px;
    border-bottom: 1px solid var(--border);
  }
  :global(.panel) {
    margin-top: 18px;
    border: 1px solid var(--border);
    background: var(--surface);
  }
  .shell {
    position: sticky;
    top: 0;
    z-index: 10;
    display: flex;
    min-width: 0;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    height: 54px;
    padding: 0 max(16px, calc((100vw - 960px) / 2));
    border-bottom: 1px solid var(--border);
    background: rgb(0 0 0 / 94%);
    backdrop-filter: blur(10px);
  }
  .brand {
    color: var(--text);
    font-weight: 700;
    text-decoration: none;
    white-space: nowrap;
  }
  nav {
    display: flex;
    align-self: stretch;
    gap: 18px;
  }
  nav a {
    display: grid;
    place-items: center;
    border-bottom: 1px solid transparent;
    color: var(--muted);
    font-size: 12px;
    text-decoration: none;
  }
  nav a:hover,
  nav a.active {
    border-color: var(--text);
    color: var(--text);
  }
  main {
    width: min(960px, 100%);
    min-width: 0;
    margin: 0 auto;
    padding: 34px 20px 72px;
  }
  @media (max-width: 480px) {
    .shell {
      height: 48px;
      padding-inline: 14px;
    }
    nav {
      gap: 14px;
    }
    main {
      padding: 24px 12px 48px;
    }
  }
</style>
