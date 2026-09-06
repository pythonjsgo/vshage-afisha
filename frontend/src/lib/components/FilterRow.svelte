<script lang="ts">
  import { EXTRA_SECTIONS, type Section } from '$lib/taxonomy';
  import type { Facets } from '$lib/api';

  let {
    citySlug,
    facets,
    activeSlug = null,
    showCounts = true
  }: {
    citySlug: string;
    /** null — фасеты не ответили: рисуем ряд БЕЗ цифр, а не с нулями. */
    facets: Facets | null;
    activeSlug?: string | null;
    /** false — счётчики занижены молчащим источником: рисуем без цифр. */
    showCounts?: boolean;
  } = $props();

  // Ноль и «неизвестно» — разные вещи. Написать «Сегодня 0», когда счётчик
  // просто не приехал, значит соврать: человек прочтёт это как «сегодня
  // ничего нет» и не откроет раздел, в котором сорок событий.
  function countFor(s: Section): number | null {
    if (!facets || !showCounts) return null;
    if (s.kind === 'price') return facets.free;
    if (s.code === 'today') return facets.when.today;
    if (s.code === 'tomorrow') return facets.when.tomorrow;
    if (s.code === 'weekend') return facets.when.weekend;
    return null;
  }

  const items = $derived(
    EXTRA_SECTIONS.map((s) => ({ section: s, count: countFor(s) }))
  );
</script>

<!-- «Быстрый выбор», а не «фильтры»: каждая пилюля — самостоятельный раздел
     со своим адресом, и переход в неё с /msk/concert снимает рубрику. Назвать
     это фильтром значило бы пообещать, что «Концерты» + «Сегодня» сложатся, —
     схема адресов такого не умеет, и обещание было бы ложным. -->
<section class="filters" aria-label="Быстрый выбор">
  <div class="label">БЫСТРЫЙ ВЫБОР</div>
  <div class="row">
    {#each items as it (it.section.slug)}
      {#if it.section.slug === activeSlug}
        <span class="pill on" aria-current="page">
          <span>{it.section.tile}</span>
          {#if it.count !== null}<span class="n">{it.count}</span>{/if}
        </span>
      {:else}
        <a class="pill" href={`/${citySlug}/${it.section.slug}`}>
          <span>{it.section.tile}</span>
          {#if it.count !== null}<span class="n">{it.count}</span>{/if}
        </a>
      {/if}
    {/each}
  </div>
</section>

<style>
  .label {
    font-size: 10px;
    letter-spacing: 2px;
    color: var(--mute);
    padding-bottom: var(--sp-2);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-2);
  }
  .pill {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg);
    font-size: 11px;
    letter-spacing: 1px;
    text-transform: uppercase;
    white-space: nowrap;
    transition: border-color var(--dur-fast) var(--ease-out),
                color var(--dur-fast) var(--ease-out);
  }
  a.pill:hover { border-color: var(--accent-pink); color: var(--accent-pink); }
  .pill.on {
    background: var(--accent-pink);
    border-color: var(--accent-pink);
    color: #000;
    font-weight: 700;
  }
  .n { font-size: 10px; color: var(--mute); }
  .pill.on .n { color: #000; opacity: 0.65; }
</style>
