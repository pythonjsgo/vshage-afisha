<script lang="ts">
  import type { City } from '$lib/seo';

  let {
    current,
    cities
  }: {
    current: City;
    /** Города с непустой доской — как их прислал бэкенд в фасетах. */
    cities: City[];
  } = $props();

  const others = $derived(cities.filter((c) => c.slug !== current.slug));
</script>

<!-- Городов сегодня один. Выпадашка с единственным пунктом — кнопка, которая
     ничего не делает, а такая кнопка хуже отсутствующей: человек нажимает,
     ничего не происходит, и дальше он не верит ни одному нашему элементу
     управления. Поэтому рисуем город текстом и честно говорим, что будет
     дальше. Как только бэкенд вернёт второй город, ветка ниже сама
     превратится в настоящий переключатель — переписывать не придётся. -->
<div class="city">
  {#if others.length > 0}
    <span class="now">{current.name}</span>
    <span class="sep">·</span>
    {#each others as c (c.slug)}
      <a class="other" href={`/${c.slug}`}>{c.name}</a>
    {/each}
  {:else}
    <span class="now">{current.name}</span>
    <span class="soon">другие города — скоро</span>
  {/if}
</div>

<style>
  .city {
    display: flex;
    align-items: baseline;
    gap: var(--sp-2);
    font-size: 11px;
    letter-spacing: 1px;
    min-width: 0;
  }
  .now { color: var(--accent-green); text-transform: uppercase; white-space: nowrap; }
  .sep { color: var(--border); }
  .other { color: var(--mute); text-transform: uppercase; white-space: nowrap; }
  .other:hover { color: var(--accent-pink); }
  /* Подпись НЕ прячем на узком экране: она и есть ответ на вопрос «а мой
     город?», и задать его человеку негде. Место под неё даёт шапка —
     переносом строки, а не сокрытием. */
  .soon {
    color: var(--mute);
    font-size: 10px;
    letter-spacing: 0.5px;
    opacity: 0.75;
  }
</style>
