<script lang="ts">
  import CodePreview from './CodePreview.svelte';

  let { value }: { value: unknown } = $props();
  let mode = $state<'json' | 'parsed'>('parsed');

  type LogEvent = {
    raw: Record<string, unknown>;
    time: string;
    level: string;
    message: string;
    attributes: Array<[string, unknown]>;
  };

  function records(input: unknown): Array<Record<string, unknown>> {
    const collect = (parsed: unknown): Array<Record<string, unknown>> => {
      if (Array.isArray(parsed)) {
        return parsed.filter(
          (entry): entry is Record<string, unknown> =>
            entry !== null && typeof entry === 'object' && !Array.isArray(entry)
        );
      }
      return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)
        ? [parsed as Record<string, unknown>]
        : [];
    };
    if (typeof input !== 'string') return collect(input);
    try {
      const complete = collect(JSON.parse(input));
      if (complete.length) return complete;
    } catch {
      // JSONL is parsed one record per line below.
    }
    const parsed: Array<Record<string, unknown>> = [];
    for (const line of input.split('\n')) {
      if (!line.trim()) continue;
      try {
        const lineRecords = collect(JSON.parse(line));
        if (!lineRecords.length) return [];
        parsed.push(...lineRecords);
      } catch {
        return [];
      }
    }
    return parsed;
  }

  function firstText(raw: Record<string, unknown>, keys: string[]): string {
    for (const key of keys) {
      const entry = raw[key];
      if (typeof entry === 'string' && entry.trim()) return entry;
    }
    return '';
  }

  function parseEvents(input: unknown): LogEvent[] {
    return records(input).map((raw) => ({
      raw,
      time: firstText(raw, ['time', 'timestamp', 'ts', 'Time']),
      level: firstText(raw, ['level', 'severity', 'lvl', 'Level']).toLowerCase() || 'info',
      message:
        firstText(raw, [
          'msg',
          'message',
          'event',
          'Action',
          'action',
          'Test',
          'test',
          'name',
          'type',
          'output'
        ]) || Object.keys(raw).slice(0, 3).join(' · '),
      attributes: Object.entries(raw)
    }));
  }

  const events = $derived(parseEvents(value));
  const canParse = $derived(events.length > 0);
  const displayTime = (value: string) => {
    const parsed = new Date(value);
    return value && !Number.isNaN(parsed.getTime())
      ? parsed.toLocaleString([], { dateStyle: 'short', timeStyle: 'medium' })
      : value || '—';
  };
</script>

<section class="log-preview">
  {#if canParse}
    <nav aria-label="Log display mode">
      <button class:active={mode === 'parsed'} onclick={() => (mode = 'parsed')}>Parsed log</button>
      <button class:active={mode === 'json'} onclick={() => (mode = 'json')}>JSON</button>
      <span>{events.length} events</span>
    </nav>
  {/if}

  {#if canParse && mode === 'parsed'}
    <div class="events">
      {#each events as event, index (index)}
        <details
          class:flagged={['warn', 'error', 'fatal', 'panic', 'critical'].includes(event.level)}
        >
          <summary>
            <time title={event.time}>{displayTime(event.time)}</time>
            <span class="level" data-level={event.level}>{event.level}</span>
            <strong>{event.message}</strong>
            <i>{event.attributes.length}</i>
          </summary>
          <dl>
            {#each event.attributes as [key, entry] (key)}
              <div>
                <dt>{key}</dt>
                <dd>{typeof entry === 'object' ? JSON.stringify(entry) : String(entry)}</dd>
              </div>
            {/each}
          </dl>
          <CodePreview value={event.raw} />
        </details>
      {/each}
    </div>
  {:else}
    <CodePreview {value} />
  {/if}
</section>

<style>
  .log-preview {
    border: 1px solid var(--border);
    background: var(--base);
  }
  nav {
    display: flex;
    align-items: center;
    min-height: 38px;
    padding: 4px;
    border-bottom: 1px solid var(--border);
  }
  nav button {
    min-height: 28px;
    padding: 0 10px;
    border: 0;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    font: inherit;
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
  }
  nav button.active {
    background: var(--overlay);
    color: var(--text);
    box-shadow: inset 0 -2px 0 var(--focus);
  }
  nav span {
    margin-left: auto;
    padding-right: 8px;
    color: var(--muted);
    font-size: 9px;
    text-transform: uppercase;
  }
  .events {
    max-height: 480px;
    overflow: auto;
  }
  details {
    border-bottom: 1px solid color-mix(in srgb, var(--border) 65%, transparent);
  }
  details.flagged {
    background: color-mix(in srgb, var(--love) 5%, transparent);
  }
  summary {
    display: grid;
    grid-template-columns: 148px 62px minmax(0, 1fr) 24px;
    gap: 8px;
    align-items: center;
    min-height: 36px;
    padding: 0 10px;
    cursor: pointer;
    list-style: none;
  }
  summary:hover {
    background: color-mix(in srgb, var(--focus) 7%, transparent);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  time,
  .level,
  summary i {
    color: var(--muted);
    font-size: 9px;
    font-style: normal;
    text-transform: uppercase;
  }
  time {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .level[data-level='warn'] {
    color: var(--gold);
  }
  .level[data-level='error'],
  .level[data-level='fatal'],
  .level[data-level='panic'],
  .level[data-level='critical'] {
    color: var(--love);
  }
  summary strong {
    overflow: hidden;
    font-size: 11px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  dl {
    margin: 0;
    padding: 8px 12px 12px 230px;
  }
  dl div {
    display: grid;
    grid-template-columns: minmax(90px, 0.3fr) minmax(0, 1fr);
    gap: 12px;
    padding: 4px 0;
  }
  dt {
    color: var(--muted);
  }
  dd {
    min-width: 0;
    margin: 0;
    overflow-wrap: anywhere;
  }
  details :global(.code-preview) {
    margin: 0 12px 12px 230px;
    max-height: 260px;
  }
  @media (max-width: 720px) {
    summary {
      grid-template-columns: 96px 50px minmax(0, 1fr);
    }
    summary i {
      display: none;
    }
    dl,
    details :global(.code-preview) {
      margin-left: 10px;
      padding-left: 10px;
    }
  }
</style>
