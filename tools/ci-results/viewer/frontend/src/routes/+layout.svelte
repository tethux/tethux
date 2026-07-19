<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';

  let { children } = $props();

  const links = [
    { href: '/', label: 'Summary' },
    { href: '/query', label: 'Query builder' },
    { href: '/runs', label: 'Runs' },
    { href: '/tests', label: 'Tests' },
    { href: '/artifacts', label: 'Artifacts' }
  ] as const;
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
    <small>Local SQLite viewer</small>
  </aside>
  <main class:workspace-main={page.url.pathname === '/query'}>{@render children()}</main>
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
  :global(h1) {
    margin: 0;
    font-size: 25px;
  }
  :global(.lede) {
    color: #666;
    margin: 7px 0 0;
  }
  :global(.page-header) {
    border-bottom: 1px solid #ccc;
    padding-bottom: 20px;
  }
  :global(.panel) {
    margin-top: 24px;
    border: 1px solid #ccc;
    background: #fff;
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
  nav a {
    padding: 7px 9px;
    color: #777;
    text-decoration: none;
    border-radius: 3px;
  }
  nav a:hover {
    color: #222;
    background: #e2e2df;
  }
  nav a.active {
    color: #111;
    background: #dcdcd8;
  }
  aside small {
    margin-top: auto;
    color: #777;
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
      border-bottom: 1px solid #ccc;
    }
    aside small {
      display: none;
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
