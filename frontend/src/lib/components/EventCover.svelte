<script lang="ts">
  import { onMount } from 'svelte';
  import { registerCoverVideo, coverMotionSnapshot, subscribeCoverMotion, toggleCoverMotion } from '$lib/cover-motion';
  let { poster, video, eager = false }: { poster?: string; video?: string; eager?: boolean } = $props();
  let motion = $state(0);
  const labels = {
    ru: { pause: 'Остановить анимацию', play: 'Включить анимацию' },
    en: { pause: 'Pause animation', play: 'Enable animation' }
  };
  let language = $state<'ru' | 'en'>('ru');
  onMount(() => {
    language = document.documentElement.lang.startsWith('en') ? 'en' : 'ru';
    const unsubscribe = subscribeCoverMotion(() => { motion = coverMotionSnapshot(); });
    motion = coverMotionSnapshot();
    return unsubscribe;
  });
  function motionVideo(node: HTMLVideoElement, source: string) {
    let release = registerCoverVideo(node, source);
    return { update(next: string) { release(); release = registerCoverVideo(node, next); }, destroy() { release(); } };
  }
</script>

<div class="event-cover" data-event-cover>
  {#if poster}<img src={poster} alt="" loading={eager ? 'eager' : 'lazy'} decoding="async" />{/if}
  {#if video}
    <!-- Decorative: the poster and event title carry the information. No audio track or player controls. -->
    <video use:motionVideo={video} {poster} muted loop playsinline preload="none" disablepictureinpicture disableremoteplayback aria-hidden="true" data-cover-video></video>
    {#if !(motion & 6)}
      <button type="button" class="motion-toggle" aria-label={motion ? labels[language].play : labels[language].pause}
        title={motion ? labels[language].play : labels[language].pause} onclick={toggleCoverMotion}>
        <svg viewBox="0 0 16 16" aria-hidden="true"><path d={motion ? 'M5 3 13 8 5 13Z' : 'M4 3h3v10H4zm5 0h3v10H9z'} /></svg>
      </button>
    {/if}
  {/if}
</div>

<style>
  .event-cover { position: absolute; inset: 0; overflow: hidden; }
  img, video { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
  video { position: absolute; inset: 0; opacity: 0; pointer-events: none; transition: opacity 180ms ease; }
  video[data-ready='true'] { opacity: 1; }
  .motion-toggle { position: absolute; z-index: 5; right: 12px; bottom: 12px; display: grid; place-items: center; width: 36px; height: 36px; padding: 0; border: 1px solid rgba(255,255,255,.3); border-radius: 50%; color: white; background: rgba(0,0,0,.5); cursor: pointer; }
  .motion-toggle svg { width: 14px; height: 14px; fill: currentColor; }
  .motion-toggle:focus-visible { outline: 2px solid var(--accent-green); outline-offset: 3px; }
  video[data-failed='true'] + .motion-toggle { display: none; }
  @media (prefers-reduced-motion: reduce) { video { transition: none; } }
</style>
