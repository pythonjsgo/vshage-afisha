<script lang="ts">
  import '../app.css';
  import Scanlines from '$lib/components/Scanlines.svelte';
  let { children } = $props();
</script>

<Scanlines />
{@render children?.()}
