<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import EventCard from './EventCard.svelte';
  let {
    events,
    label = '',
    emptyText = 'ПОКА ПУСТО · ЗАГЛЯНИ ЗАВТРА',
    more = null
  }: {
    events: PublicEvent[];
    label?: string;
    /**
     * Что написать вместо пустой сетки. `null` — не писать НИЧЕГО.
     *
     * Это не косметика. На двухполосной доске основная полоса бывает пуста
     * при непустой второй: всё, что идёт в городе, — многодневные программы,
     * а точечных событий на ближайшие дни нет. «ПОКА ПУСТО · ЗАГЛЯНИ ЗАВТРА»
     * над сеткой из восьмидесяти идущих выставок — прямое враньё об афише.
     */
    emptyText?: string | null;
    /** Ссылка справа от метки: «ВСЕ 88 →» у полосы, показанной превью. */
    more?: { href: string; text: string } | null;
  } = $props();
</script>

{#if label || more}
  <div class="grid-head">
    <span class="grid-label">{label}</span>
    {#if more}<a class="grid-more" href={more.href}>{more.text}</a>{/if}
  </div>
{/if}

<div class="grid">
  {#each events as e (e.id)}
    <EventCard event={e} />
  {/each}
</div>

{#if events.length === 0 && emptyText}
  <div class="empty">{emptyText}</div>
{/if}

<style>
  /* Метка и ссылка «все» стоят на одной линии и разъезжаются по краям.
     Отступы и линия сверху переехали сюда со самой метки: они принадлежат
     ряду целиком, иначе ссылка встала бы выше черты. */
  .grid-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--sp-3);
    padding: var(--sp-4) 0 var(--sp-3);
    border-top: 1px solid var(--border);
  }
  .grid-label {
    font-size: 10px;
    letter-spacing: 2px;
    color: var(--accent-green);
  }
  .grid-more {
    font-size: 10px;
    letter-spacing: 1px;
    color: var(--mute);
    white-space: nowrap;
    transition: color var(--dur-fast) var(--ease-out);
  }
  .grid-more:hover { color: var(--accent-pink); }
  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--sp-3);
  }
  @media (min-width: 640px) {
    .grid { grid-template-columns: 1fr 1fr; gap: var(--sp-4); }
  }
  @media (min-width: 1024px) {
    .grid { grid-template-columns: 1fr 1fr 1fr; }
  }
  .empty {
    padding: var(--sp-8);
    text-align: center;
    color: var(--mute);
    font-size: 12px;
    letter-spacing: 2px;
  }
</style>
