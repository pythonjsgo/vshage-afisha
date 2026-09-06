<script lang="ts">
  import type { EventKind } from '$lib/api';

  let {
    basePath,
    counts,
    active = null,
    showCounts = true
  }: {
    /** Путь раздела без параметров: `/msk` или `/msk/exhibition`. */
    basePath: string;
    /**
     * Сколько событий в каждой полосе ЭТОЙ страницы; null — числа неизвестны,
     * рисуем ряд БЕЗ цифр, а не с нулями.
     *
     * Числа приходят готовыми, и компонент не знает, откуда они, — намеренно.
     * Пока он брал их из фасетов, он печатал числа ГОРОДА рядом со ссылкой в
     * РАЗДЕЛ (фасеты просятся без фильтра раздела): на `/msk/concert` пилюля
     * обещала «ИДЁТ СЕЙЧАС · 88» и открывала пустую сетку. Число рядом с
     * элементом — обещание того, что по нему откроется, и держать это
     * обещание может только тот, кто делал оба запроса, то есть загрузчик.
     */
    counts: { timed: number; running: number } | null;
    /** Выбранная полоса; null — показаны обе. */
    active?: EventKind | null;
    /** false — счётчики занижены молчащим источником: рисуем без цифр. */
    showCounts?: boolean;
  } = $props();

  // Ноль и «неизвестно» — разные вещи. Написать «Идёт сейчас 0», когда
  // счётчик просто не приехал, значит соврать: человек прочтёт это как «сейчас
  // ничего не идёт» и не откроет полосу, в которой восемьдесят восемь выставок.
  // Правило дословно то же, что в FilterRow и CategoryTiles.
  const shown = $derived(counts && showCounts ? counts : null);

  /**
   * Обе полосы пусты — ряд не рисуется вовсе: у пустого раздела переключать
   * нечего, а два нуля над «ПОКА ПУСТО» ничего не сообщают.
   *
   * Числа — тоталы полос этой самой страницы, поэтому их сумма и есть всё, что
   * на ней откроется: ноль здесь означает пустой раздел, а не «полосы не
   * разложены». Второй случай — бэкенд, который полос не знает, — сюда не
   * доходит: его гасит `laneSplit` в Listing, на уровне данных, где он и виден.
   *
   * Когда чисел нет (`counts === null`), ряд рисуется без цифр, как «быстрый
   * выбор». Прятать его в этом случае нельзя: на выбранной полосе, которая
   * оказалась пуста, ряд — единственная дорога в соседнюю.
   */
  const known = $derived(counts ? counts.running + counts.timed : null);
  const visible = $derived(known === null || known > 0);

  // Повторное нажатие на активную полосу снимает её: ссылка ведёт на адрес
  // раздела без параметра. Пилюля-ссылка, ведущая сама на себя, была бы
  // тупиком — человеку пришлось бы искать «Все» там, где её нет.
  const lanes = $derived([
    {
      kind: 'timed' as EventKind,
      label: 'ПО ДАТЕ И ВРЕМЕНИ',
      count: shown?.timed ?? null
    },
    {
      kind: 'running' as EventKind,
      label: 'ИДЁТ СЕЙЧАС',
      count: shown?.running ?? null
    }
  ]);
</script>

<!-- Полоса — это раздел доски, а не фильтр: событие принадлежит РОВНО одной
     из двух, и вместе они покрывают доску целиком. Поэтому пилюли не
     складываются друг с другом и не складываются с рубрикой сверх того, что
     уже умеет адрес: `?kind=` живёт параметром именно затем, чтобы не удваивать
     21 адрес раздела до 42 почти одинаковых страниц. -->
{#if visible}
  <section class="kinds" aria-label="Полосы афиши">
    <div class="row">
      {#each lanes as lane (lane.kind)}
        {@const on = lane.kind === active}
        <a
          class="pill"
          class:on
          href={on ? basePath : `${basePath}?kind=${lane.kind}`}
          aria-current={on ? 'page' : undefined}
        >
          <span>{lane.label}</span>
          {#if lane.count !== null}<span class="n">{lane.count}</span>{/if}
        </a>
      {/each}
    </div>
  </section>
{/if}

<style>
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
  .pill:hover { border-color: var(--accent-pink); color: var(--accent-pink); }
  /* Активная полоса залита зелёным, а «быстрый выбор» — розовым: это разные
     оси, и одинаковая заливка читалась бы как один переключатель на две
     строки. Зелёный здесь тот же, что у метки сетки под ним, — глаз связывает
     выбранную полосу с тем, что под ней нарисовано. */
  .pill.on {
    background: var(--accent-green);
    border-color: var(--accent-green);
    color: #000;
    font-weight: 700;
  }
  .pill.on:hover { color: #000; }
  .n { font-size: 10px; color: var(--mute); }
  .pill.on .n { color: #000; opacity: 0.65; }
</style>
