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
>
  <circle cx="10.8" cy="10.8" r="6.3"></circle>
  <path d="m15.5 15.5 4.1 4.1"></path>
</svg>
