<script lang="ts">
  let { value }: { value: unknown } = $props();

  const text = $derived(
    typeof value === 'string' ? value : value === null ? '' : JSON.stringify(value, null, 2)
  );
  const lines = $derived(text.split('\n'));
  const jsonLike = $derived(typeof value !== 'string' || /^[\s]*[[{]/.test(text));

  type Token = { value: string; kind: string };
  function tokens(line: string): Token[] {
    if (!jsonLike) return [{ value: line, kind: 'plain' }];
    const pattern =
      /("(?:\\.|[^"\\])*")(?=\s*:)|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:e[+-]?\d+)?)/gi;
    const result: Token[] = [];
    let cursor = 0;
    for (const match of line.matchAll(pattern)) {
      const index = match.index ?? 0;
      if (index > cursor) result.push({ value: line.slice(cursor, index), kind: 'plain' });
      result.push({
        value: match[0],
        kind: match[1] ? 'key' : match[2] ? 'string' : match[3] ? 'literal' : 'number'
      });
      cursor = index + match[0].length;
    }
    if (cursor < line.length) result.push({ value: line.slice(cursor), kind: 'plain' });
    return result;
  }
</script>

<div class="code-preview" class:structured={jsonLike}>
  {#each lines as line, index (index)}
    <div class="line">
      <span class="gutter">{index + 1}</span>
      <code
        >{#each tokens(line) as token, tokenIndex (`${tokenIndex}:${token.kind}`)}<span
            class={token.kind}>{token.value}</span
          >{/each}</code
      >
    </div>
  {/each}
</div>

<style>
  .code-preview {
    max-height: 480px;
    overflow: auto;
    border: 1px solid var(--border);
    background: var(--base);
    font-size: 11px;
    line-height: 1.65;
  }
  .line {
    display: grid;
    grid-template-columns: 44px minmax(max-content, 1fr);
    min-height: 20px;
  }
  .line:hover {
    background: color-mix(in srgb, var(--focus) 7%, transparent);
  }
  .gutter {
    position: sticky;
    left: 0;
    padding-right: 10px;
    border-right: 1px solid color-mix(in srgb, var(--border) 65%, transparent);
    background: var(--base);
    color: var(--muted);
    text-align: right;
    user-select: none;
  }
  code {
    padding: 0 12px;
    color: var(--text);
    white-space: pre;
  }
  .key {
    color: var(--iris);
  }
  .string {
    color: var(--foam);
  }
  .literal {
    color: var(--gold);
  }
  code .number {
    color: var(--love);
  }
</style>
