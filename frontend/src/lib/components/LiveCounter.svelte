<script lang="ts">
  import { onMount } from 'svelte';
  let { value, label = '' }: { value: number; label?: string } = $props();
  let shown = $state(0);
  let el: HTMLSpanElement | undefined = $state();

  onMount(() => {
    if (!el) return;
    const io = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting) {
        const duration = 800;
        const start = performance.now();
        const step = (t: number) => {
          const p = Math.min(1, (t - start) / duration);
          shown = Math.floor(value * (0.5 - Math.cos(Math.PI * p) / 2));
          if (p < 1) requestAnimationFrame(step);
          else shown = value;
        };
        requestAnimationFrame(step);
        io.disconnect();
      }
    });
    io.observe(el);
    return () => io.disconnect();
  });
</script>

<span bind:this={el}>{shown}{label ? ' ' + label : ''}</span>
