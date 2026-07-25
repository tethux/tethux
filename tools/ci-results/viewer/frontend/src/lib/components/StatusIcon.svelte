<script lang="ts">
  let { passed, size = 13 }: { passed: boolean; size?: number } = $props();
</script>

<svg
  aria-hidden="true"
  viewBox="0 0 16 16"
  width={size}
  height={size}
  fill="none"
  stroke="currentColor"
  stroke-width="2.2"
  stroke-linecap="round"
  stroke-linejoin="round"
>
  {#if passed}
    <path d="m3.5 8 3 3 6-6"></path>
  {:else}
    <path d="m4.5 4.5 7 7m0-7-7 7"></path>
  {/if}
</svg>

<style>
  svg {
    display: block;
  }
</style>
