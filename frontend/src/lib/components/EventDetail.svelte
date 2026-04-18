<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import { formatEventDateLong } from '$lib/dateFormat';
  import MetaPill from './MetaPill.svelte';
  import GlitchText from './GlitchText.svelte';
  import ShareSheet from './ShareSheet.svelte';
  import AppCTA from './AppCTA.svelte';
  import LiveCounter from './LiveCounter.svelte';

  let { event, origin }: { event: PublicEvent; origin: string } = $props();
  const url = $derived(`${origin}/${event.id}`);
  const cancelled = $derived(event.status === 'cancelled');
  const category = $derived(event.category?.toUpperCase() ?? '');
</script>

<article class="detail" class:cancelled>
  <header class="cover" style={event.photo_url ? `background-image: url(${event.photo_url})` : ''}>
    <div class="overlay"></div>
    <nav class="back-nav"><a href="/">← АФИША</a></nav>
    <div class="hd">
      <h1><GlitchText text={event.title} /></h1>
      <div class="pills">
        {#if cancelled}<MetaPill text="Отменено" variant="warning" />{/if}
        {#if category}<MetaPill text={category} />{/if}
        {#if event.max_attendees}<MetaPill text={`LIMIT ${event.max_attendees}`} />{/if}
      </div>
    </div>
  </header>

  <section class="info">
    <div class="row"><div class="k">КОГДА</div><div class="v">{formatEventDateLong(event.start_time)}</div></div>
    {#if event.location}
      <div class="row"><div class="k">ГДЕ</div><div class="v">{event.location}</div></div>
    {/if}
    {#if event.attendee_count > 0}
      <div class="row"><div class="k">КТО ИДЁТ</div><div class="v"><LiveCounter value={event.attendee_count} label="идут" /></div></div>
    {/if}
  </section>

  {#if event.description}
    <section class="desc">{event.description}</section>
  {/if}

  <section class="share-sec">
    <div class="section-label">ПОДЕЛИТЬСЯ</div>
    <ShareSheet {url} title={event.title} />
  </section>

  <section class="cta-sec">
    <AppCTA eventId={event.id} />
  </section>
</article>

<style>
  .detail.cancelled { opacity: 0.65; filter: grayscale(1); }
  .cover {
    position: relative;
    min-height: 320px;
    padding: var(--sp-4);
    background: linear-gradient(135deg, var(--accent-pink) 0%, #1a0014 100%);
    background-size: cover; background-position: center;
    display: flex; flex-direction: column; justify-content: space-between;
  }
  .overlay { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(0,0,0,0.1) 0%, rgba(0,0,0,0.85) 100%); }
  .back-nav, .hd { position: relative; z-index: 1; }
  .back-nav a { color: var(--fg); font-size: 11px; letter-spacing: 1px; }
  .hd h1 { font-family: var(--font-display); font-size: clamp(36px, 9vw, 88px); line-height: 0.9; text-transform: uppercase; font-weight: 400; letter-spacing: -1.5px; }
  .pills { display: flex; gap: var(--sp-1); flex-wrap: wrap; margin-top: var(--sp-3); }
  .info { padding: var(--sp-4); display: flex; flex-direction: column; gap: 0; }
  .row { display: flex; justify-content: space-between; padding: var(--sp-3) 0; border-bottom: 1px solid var(--border); font-size: 12px; }
  .row .k { color: var(--mute); letter-spacing: 1.5px; font-size: 10px; text-transform: uppercase; }
  .desc { padding: 0 var(--sp-4) var(--sp-4); font-size: 13px; line-height: 1.6; color: #ccc; }
  .share-sec, .cta-sec { padding: var(--sp-3) var(--sp-4); }
  .section-label { font-size: 10px; letter-spacing: 2px; color: var(--accent-green); margin-bottom: var(--sp-2); }
</style>
