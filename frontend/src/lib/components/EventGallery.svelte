<script lang="ts">
  let { photos }: { photos: string[] } = $props();

  let track: HTMLDivElement;
  let active = $state(0);

  function onScroll() {
    if (!track) return;
    const w = track.clientWidth;
    active = Math.round(track.scrollLeft / w);
  }

  function goto(i: number) {
    if (!track) return;
    track.scrollTo({ left: i * track.clientWidth, behavior: 'smooth' });
  }
</script>

{#if photos && photos.length > 0}
  <section class="gallery">
    <div class="label">ФОТО · {String(active + 1).padStart(2, '0')} / {String(photos.length).padStart(2, '0')}</div>
    <div class="track" bind:this={track} onscroll={onScroll}>
      {#each photos as url, i (url + i)}
        <div class="slide">
          <img src={url} alt={`Фото ${i + 1}`} loading={i === 0 ? 'eager' : 'lazy'} />
        </div>
      {/each}
    </div>
    {#if photos.length > 1}
      <div class="pager">
        {#each photos as _, i (i)}
          <button
            class="tick"
            class:on={i === active}
            aria-label={`Фото ${i + 1}`}
            onclick={() => goto(i)}
          ></button>
        {/each}
      </div>
    {/if}
  </section>
{/if}

<style>
  .gallery {
    padding: 0 var(--sp-4) var(--sp-3);
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .label {
    font-size: 10px;
    letter-spacing: 2px;
    color: var(--accent-green);
    font-variant-numeric: tabular-nums;
  }
  .track {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: 100%;
    gap: 0;
    overflow-x: auto;
    scroll-snap-type: x mandatory;
    scroll-behavior: smooth;
    scrollbar-width: none;
    -webkit-overflow-scrolling: touch;
  }
  .track::-webkit-scrollbar { display: none; }
  .slide {
    scroll-snap-align: center;
    scroll-snap-stop: always;
    aspect-ratio: 4 / 3;
    background: var(--bg-elev);
    border: 1px solid var(--border);
    overflow: hidden;
    position: relative;
  }
  .slide img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    user-select: none;
    -webkit-user-drag: none;
  }
  .pager {
    display: flex;
    gap: var(--sp-1);
    padding-top: var(--sp-1);
  }
  .tick {
    flex: 1;
    height: 2px;
    background: var(--border);
    border: none;
    padding: 0;
    cursor: pointer;
    transition: background var(--dur-fast);
  }
  .tick.on { background: var(--accent-pink); }

  @media (min-width: 1024px) {
    .slide { aspect-ratio: 16 / 9; }
  }
</style>
