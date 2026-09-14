<script lang="ts">
  import { registerCoverVideo } from '$lib/cover-motion';
  import { documentEventLocale, eventCopy } from '$lib/event-copy';
  let { poster, video, eager = false, fit = 'cover', playable = false }: { poster?: string; video?: string; eager?: boolean; fit?: 'cover' | 'contain'; playable?: boolean } = $props();
  let playRequested = $state(false);
  function motionVideo(node: HTMLVideoElement, source: string) {
    let release = registerCoverVideo(node, source);
    return { update(next: string) { release(); release = registerCoverVideo(node, next); }, destroy() { release(); } };
  }
</script>

<div class="event-cover" data-event-cover>
  {#if playable && video}
    {#if !playRequested}
      <button class="play-poster" onclick={() => { playRequested = true; }} aria-label={eventCopy('watchTeaser', documentEventLocale())}>
        <img src={poster} alt="" style:object-fit={fit} />
        <span class="play-label">▶ {eventCopy('watchTeaser', documentEventLocale())}</span>
      </button>
    {:else}
    <!-- Organizer-supplied media can have no caption track. -->
    <!-- svelte-ignore a11y_media_has_caption -->
    <video class="player" src={video} {poster} controls playsinline autoplay preload="metadata" style:object-fit={fit}></video>
    {/if}
  {:else}
  {#if poster}<img src={poster} alt="" loading={eager ? 'eager' : 'lazy'} decoding="async" style:object-fit={fit} />{/if}
  {#if video}
    <!-- Decorative: the poster and event title carry the information. No audio track or player controls. -->
    <video use:motionVideo={video} {poster} muted loop playsinline preload="none" disablepictureinpicture disableremoteplayback aria-hidden="true" data-cover-video></video>
  {/if}
  {/if}
</div>

<style>
  .event-cover { position: absolute; inset: 0; overflow: hidden; }
  img, video { width: 100%; height: 100%; object-fit: cover; object-position: center; display: block; }
  video { position: absolute; inset: 0; opacity: 0; pointer-events: none; transition: opacity 180ms ease; }
  video.player { position: static; opacity: 1; pointer-events: auto; background: #0a0a0a; }
  .play-poster { position: relative; display: block; width: 100%; height: 100%; padding: 0; border: 0; cursor: pointer; background: #0a0a0a; }
  .play-label { position: absolute; top: 20px; right: 20px; padding: 8px; color: white; background: #0a0a0acc; font-size: 11px; }
  .play-poster:focus-visible { outline: 2px solid var(--accent-green); outline-offset: -2px; }
  /* The action writes this attribute at runtime. Without :global Svelte
     removes the selector as unused and the playing video stays transparent. */
  video:global([data-ready='true']) { opacity: 1; }
  @media (prefers-reduced-motion: reduce) { video { transition: none; } }
</style>
