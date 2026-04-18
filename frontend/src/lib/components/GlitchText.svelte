<script lang="ts">
  import { onMount } from 'svelte';
  let { text = '', class: klass = '' }: { text?: string; class?: string } = $props();
  let el: HTMLSpanElement | undefined = $state();

  onMount(() => {
    if (!el) return;
    let cancelled = false;
    const loop = () => {
      if (cancelled) return;
      const delay = 8000 + Math.random() * 7000;
      setTimeout(() => {
        if (cancelled) return;
        el?.classList.add('glitching');
        setTimeout(() => el?.classList.remove('glitching'), 200);
        loop();
      }, delay);
    };
    loop();
    return () => { cancelled = true; };
  });
</script>

<span bind:this={el} class={klass} data-text={text}>{text}</span>

<style>
  span { position: relative; display: inline-block; }
  span.glitching::before,
  span.glitching::after {
    content: attr(data-text);
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  span.glitching::before {
    color: var(--accent-pink);
    transform: translate(-2px, 0);
    clip-path: inset(40% 0 40% 0);
  }
  span.glitching::after {
    color: var(--accent-green);
    transform: translate(2px, 0);
    clip-path: inset(15% 0 60% 0);
  }
</style>
