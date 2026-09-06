<script lang="ts">
  import { CATEGORY_SECTIONS } from '$lib/taxonomy';
  import type { CategoryFacet } from '$lib/api';

  let {
    citySlug,
    categories,
    cityTotal,
    activeSlug = null,
    allActive = false,
    showCounts = true
  }: {
    citySlug: string;
    /** Фасеты как их прислал бэкенд: только count > 0, по убыванию count. */
    categories: CategoryFacet[];
    /**
     * Мы на доске города целиком. Отдельный признак, а не `!activeSlug`:
     * на `/msk/segodnya` активной рубрики нет, но и «Все» — не текущая
     * страница, а ссылка. Прежняя проверка подписывала плитку «Все»
     * как aria-current на всех разделах времени и цены — то есть врала и
     * человеку, и скринридеру о том, где он находится.
     */
    allActive?: boolean;
    /** false — счётчики занижены молчащим источником: рисуем без цифр. */
    showCounts?: boolean;
    /** Сколько всего событий в городе — цифра у плитки «Все». */
    cityTotal: number;
    activeSlug?: string | null;
  } = $props();

  const BY_CODE = new Map(CATEGORY_SECTIONS.map((s) => [s.code, s]));

  // Порядок берём у бэкенда (по убыванию count), а не канонический из
  // словаря: плитка из пятнадцати рубрик читается сверху вниз, и человеку
  // важнее «где много», чем алфавит. Код, которого нет в словаре фронта,
  // ПРОПУСКАЕМ и кричим в лог: нарисовать его сырым кодом — это ровно та
  // ошибка, из-за которой на карточке ленты однажды висело
  // «feed.category.campus», а маршрут на него всё равно ответил бы 404.
  const tiles = $derived(
    categories.flatMap((c) => {
      const s = BY_CODE.get(c.code);
      if (!s) {
        console.warn('afisha: рубрика без словаря на фронте, плитка её не рисует:', c.code);
        return [];
      }
      return [{ slug: s.slug, label: s.tile, count: c.count }];
    })
  );
</script>

<nav class="tiles" aria-label="Разделы афиши">
  <div class="row">
    {#if !allActive}
      <a class="tile" href={`/${citySlug}`}>
        <span class="t">Все</span>
        {#if showCounts}<span class="n">{cityTotal}</span>{/if}
      </a>
    {:else}
      <span class="tile on" aria-current="page">
        <span class="t">Все</span>
        {#if showCounts}<span class="n">{cityTotal}</span>{/if}
      </span>
    {/if}

    {#each tiles as t (t.slug)}
      {#if t.slug === activeSlug}
        <span class="tile on" aria-current="page">
          <span class="t">{t.label}</span>
          {#if showCounts}<span class="n">{t.count}</span>{/if}
        </span>
      {:else}
        <a class="tile" href={`/${citySlug}/${t.slug}`}>
          <span class="t">{t.label}</span>
          {#if showCounts}<span class="n">{t.count}</span>{/if}
        </a>
      {/if}
    {/each}
  </div>
</nav>

<style>
  .tiles {
    /* Прокрутка живёт ВНУТРИ своего контейнера. Ряд из пятнадцати рубрик,
       отпущенный наружу, растянул бы страницу целиком, и на телефоне уехал
       бы горизонтальный скролл у всего документа. */
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: thin;
    -webkit-overflow-scrolling: touch;
    /* Полоса упирается в края экрана, а не в отступ main — обрезанная
       плитка у кромки читается как «дальше есть ещё». */
    margin: 0 calc(-1 * var(--sp-4));
    padding: 0 var(--sp-4);
  }
  .row {
    display: flex;
    flex-wrap: nowrap;
    gap: var(--sp-2);
    width: max-content;
    padding-bottom: var(--sp-2);
  }
  .tile {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    padding: 6px 10px;
    background: var(--bg-elev);
    border: 1px solid var(--border);
    color: var(--fg);
    font-size: 11px;
    letter-spacing: 0.5px;
    white-space: nowrap;
    transition: border-color var(--dur-fast) var(--ease-out),
                color var(--dur-fast) var(--ease-out);
  }
  a.tile:hover { border-color: var(--accent-pink); color: var(--accent-pink); }
  .tile.on {
    background: var(--accent-green);
    border-color: var(--accent-green);
    color: #000;
    font-weight: 700;
  }
  .n { font-size: 10px; color: var(--mute); }
  .tile.on .n { color: #000; opacity: 0.6; }

  /* На большом экране прокрутка не нужна — ряд переносится и виден целиком. */
  @media (min-width: 720px) {
    .tiles { overflow-x: visible; margin: 0; padding: 0; }
    .row { flex-wrap: wrap; width: auto; padding-bottom: 0; }
  }
</style>
