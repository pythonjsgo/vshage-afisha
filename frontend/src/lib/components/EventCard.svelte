<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import { formatEventDate } from '$lib/dateFormat';
  import MetaPill from './MetaPill.svelte';

  let { event }: { event: PublicEvent } = $props();
  // События веб-регистрации адресуются слагом, а не uuid. Ведём на карточку
  // афиши, а не на страницу регистрации: из ленты человек идёт смотреть, что
  // это за событие, и уже оттуда — записываться (директива 17.08).
  const href = $derived(event.webreg_slug ? `/${event.webreg_slug}` : `/${event.id}`);
  const cancelled = $derived(event.status === 'cancelled');
  const category = $derived(event.category?.toUpperCase() ?? '');
</script>

<a {href} class="card" class:cancelled>
  <div
    class="photo"
    class:no-photo={!event.photo_url}
    style={event.photo_url ? `background-image: url(${event.photo_url})` : ''}
  >
    {#if !event.photo_url}
      <div class="ph-pattern"></div>
    {/if}
  </div>
  <div class="body">
    <div class="top">
      {#if cancelled}
        <MetaPill text="Отменено" variant="warning" />
      {:else if category}
        <MetaPill text={category} />
      {/if}
    </div>
    <h3>{event.title}</h3>
    <div class="meta">
      <span class="date">{formatEventDate(event.start_time)}</span>
      {#if event.attendee_count > 0}
        <span class="att">· {event.attendee_count} идут</span>
      {/if}
    </div>
    {#if event.organizer_name}
      <div class="org">by {event.organizer_name}</div>
    {/if}
  </div>
</a>

<style>
  .card {
    display: block;
    background: var(--bg-elev);
    border: 1px solid var(--border);
    color: var(--fg);
    transition: transform var(--dur-fast) var(--ease-out),
                box-shadow var(--dur-fast) var(--ease-out),
                border-color var(--dur-fast) var(--ease-out);
  }
  .card:hover {
    transform: translateY(-2px);
    border-color: var(--accent-pink);
    box-shadow: 0 0 0 1px var(--accent-pink), 0 0 24px rgba(255, 0, 204, 0.25);
  }
  .cancelled { opacity: 0.5; filter: grayscale(1); }
  .photo {
    aspect-ratio: 4 / 3;
    background-color: var(--border);
    background-size: cover;
    background-position: center;
    position: relative;
  }
  /* Без обложки карточка не должна зиять пустотой на пол-экрана —
     это ровно то, что читается как «пустое меро». Сжимаем заглушку до
     узкой полосы, содержимое карточки становится главным. */
  .photo.no-photo {
    aspect-ratio: auto;
    height: 6px;
  }
  .ph-pattern {
    position: absolute; inset: 0;
    background:
      linear-gradient(45deg,
        var(--border) 25%, transparent 25%,
        transparent 50%, var(--border) 50%,
        var(--border) 75%, transparent 75%);
    background-size: 12px 12px;
    opacity: 0.4;
  }
  .body { padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-2); }
  h3 {
    font-family: var(--font-display);
    font-size: 18px;
    line-height: 1;
    text-transform: uppercase;
    font-weight: 400;
    letter-spacing: -0.5px;
  }
  .meta { font-size: 11px; color: var(--mute); }
  .date { color: var(--accent-green); }
  .org {
    font-size: 10px;
    color: var(--mute);
    opacity: 0.6;
    letter-spacing: 0.5px;
    margin-top: 2px;
  }
</style>
