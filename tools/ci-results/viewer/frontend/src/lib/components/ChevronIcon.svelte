<script lang="ts">
  let { size = 16 }: { size?: number } = $props();
</script>

<svg
  aria-hidden="true"
  viewBox="0 0 24 24"
  width={size}
  height={size}
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
>
  <path d="m9 5 7 7-7 7"></path>
</svg>

<style>
  svg {
    display: block;
    flex: none;
  }
</style>
