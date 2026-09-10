<script lang="ts">
  import { registerCoverVideo } from '$lib/cover-motion';
  let { poster, video, eager = false }: { poster?: string; video?: string; eager?: boolean } = $props();
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
  {/if}
</div>

<style>
  .event-cover { position: absolute; inset: 0; overflow: hidden; }
  img, video { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
  video { position: absolute; inset: 0; opacity: 0; pointer-events: none; transition: opacity 180ms ease; }
  /* The action writes this attribute at runtime. Without :global Svelte
     removes the selector as unused and the playing video stays transparent. */
  video:global([data-ready='true']) { opacity: 1; }
  @media (prefers-reduced-motion: reduce) { video { transition: none; } }
</style>
