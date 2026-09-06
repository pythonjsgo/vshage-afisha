<script lang="ts">
  import HeroFeatured from '$lib/components/HeroFeatured.svelte';
  import EventGrid from '$lib/components/EventGrid.svelte';
  import CategoryTiles from '$lib/components/CategoryTiles.svelte';
  import FilterRow from '$lib/components/FilterRow.svelte';
  import CityPicker from '$lib/components/CityPicker.svelte';
  import type { PublicEvent } from '$lib/types';
  import type { Section } from '$lib/taxonomy';
  import type { City, MetaTags } from '$lib/seo';
  import { eventsCount, itemListJsonLd, breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import {
    getEvents,
    queryForSection,
    PAGE_SIZE,
    MAX_WINDOW,
    type Facets
  } from '$lib/api';

  let {
    origin,
    city,
    facets,
    countsTrusted = true,
    section,
    featured,
    events,
    total,
    meta,
    crumbs
  }: {
    origin: string;
    city: City;
    /** null — эндпоинт фасетов не ответил: плитка и цифры не рисуются. */
    facets: Facets | null;
    /** false — часть счётчиков занижена (молчал источник): цифры не рисуем. */
    countsTrusted?: boolean;
    /** null на главной города. */
    section: Section | null;
    featured: PublicEvent[];
    /** Первая порция, отрисованная на сервере. */
    events: PublicEvent[];
    total: number;
    meta: MetaTags;
    /** Путь, не абсолютный адрес: ссылка обязана остаться своей по протоколу. */
    crumbs: { name: string; path: string }[];
  } = $props();

  // Догруженное хранится ОТДЕЛЬНО от того, что приехало с сервера, и список
  // склеивается производным. Скопировать пропсы в $state было бы соблазнительно
  // и неверно: состояние запомнило бы первое значение навсегда, и страница
  // соседнего раздела показала бы карточки предыдущего. Родитель к тому же
  // оборачивает компонент в {#key} — хвост сбрасывается пересозданием.
  let extra = $state<PublicEvent[]>([]);
  /** total из последней догрузки: доска живая, число между запросами меняется. */
  let freshTotal = $state<number | null>(null);
  const items = $derived([...events, ...extra]);
  const known = $derived(freshTotal ?? total);
  let loading = $state(false);
  let failed = $state(false);
  // Сервер отдал порцию, в которой нет ни одной новой карточки. Кнопку в
  // этом случае убираем: иначе получается кнопка, которая нажимается вечно
  // и ничего не меняет, — а это худший вид отказа, немой.
  let stalled = $state(false);

  const heading = $derived(section ? `${section.heading} ${city.in}` : `Афиша ${city.of}`);
  const capped = $derived(items.length >= MAX_WINDOW && items.length < known);
  const canLoadMore = $derived(!stalled && !capped && items.length < known);

  const gridLabel = $derived(
    items.length < known
      ? `СОБЫТИЯ · ${items.length} ИЗ ${known}`
      : `ВСЕ СОБЫТИЯ · ${known}`
  );

  const itemListLd = $derived(jsonLdScript(itemListJsonLd(items, origin)));
  // В разметке для поисковика адреса обязаны быть абсолютными, а в href —
  // относительными: абсолютный href, собранный из origin, на стенде без
  // явного ORIGIN уехал бы на http и уводил бы человека с https по клику.
  const crumbsLd = $derived(
    crumbs.length > 1
      ? jsonLdScript(
          breadcrumbJsonLd(
            origin,
            crumbs.map((c) => ({ name: c.name, url: `${origin}${c.path}` }))
          )
        )
      : ''
  );

  async function loadMore() {
    if (loading || !canLoadMore) return;
    loading = true;
    failed = false;
    try {
      const res = await getEvents(fetch, {
        ...queryForSection(city.slug, section),
        limit: PAGE_SIZE,
        offset: items.length
      });
      // Дедуп по id обязателен, а не «на всякий случай»: EventGrid перечисляет
      // события с ключом, и повтор id роняет отрисовку. Повтор приедет в тот
      // день, когда сервер перестанет уважать offset, — верхний ограничитель,
      // которого требует контракт, состоит ровно из этой проверки и из stalled.
      const seen = new Set(items.map((e) => e.id));
      const fresh = res.all.filter((e) => !seen.has(e.id));
      // total мог измениться между запросами (доска живая) — берём свежий,
      // иначе подпись «N из M» разъедется с тем, что реально листается.
      freshTotal = res.total;
      if (fresh.length === 0) {
        stalled = true;
        console.error(
          'afisha: «показать ещё» вернуло 0 новых карточек при offset',
          items.length,
          'и total',
          res.total
        );
      } else {
        extra = [...extra, ...fresh];
      }
    } catch (e) {
      failed = true;
      console.error('afisha: не удалось догрузить порцию событий:', e);
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head>
  <title>{meta.title}</title>
  <meta name="description" content={meta.description} />
  <link rel="canonical" href={meta.canonical} />
  <meta property="og:type" content={meta.ogType} />
  <meta property="og:title" content={meta.title} />
  <meta property="og:description" content={meta.description} />
  <meta property="og:url" content={meta.canonical} />
  <meta property="og:image" content={meta.ogImage} />
  <meta name="twitter:card" content="summary_large_image" />
  <!-- Разметку вкладываем строкой: Svelte разбирает содержимое <script> как
       код компонента, а jsonLdScript уже экранировал «<» и «&» — заголовок
       события приезжает из телеграма и закрыл бы тег своим текстом. -->
  {@html `<script type="application/ld+json">${itemListLd}</script>`}
  {#if crumbsLd}
    {@html `<script type="application/ld+json">${crumbsLd}</script>`}
  {/if}
</svelte:head>

<header class="nav">
  <a href={`/${city.slug}`} class="logo">АФИША_ВШАГЕ</a>
  <div class="city"><CityPicker current={city} cities={facets?.cities ?? [city]} /></div>
  <nav class="links">
    <a class="cta" href="#app">↓ В ПРИЛОЖЕНИИ</a>
  </nav>
</header>

<main>
  {#if crumbs.length > 1}
    <nav class="crumbs" aria-label="Хлебные крошки">
      {#each crumbs as c, i (c.path)}
        {#if i < crumbs.length - 1}
          <a href={c.path}>{c.name}</a><span class="sep">/</span>
        {:else}
          <span class="here">{c.name}</span>
        {/if}
      {/each}
    </nav>
  {/if}

  <div class="head">
    <h1>{heading}</h1>
    <p class="sub">
      {eventsCount(known)}{#if section}&nbsp;· {section.blurb}{/if}
    </p>
  </div>

  {#if featured.length > 0}
    <HeroFeatured events={featured} />
  {/if}

  {#if facets}
    <CategoryTiles
      showCounts={countsTrusted}
      citySlug={city.slug}
      categories={facets.categories}
      cityTotal={facets.total}
      activeSlug={section?.kind === 'category' ? section.slug : null}
      allActive={!section}
    />
  {/if}

  <FilterRow
    showCounts={countsTrusted}
    citySlug={city.slug}
    {facets}
    activeSlug={section && section.kind !== 'category' ? section.slug : null}
  />

  <EventGrid events={items} label={gridLabel} />

  {#if canLoadMore}
    <button class="more" onclick={loadMore} disabled={loading}>
      {#if loading}
        ЗАГРУЖАЮ…
      {:else if failed}
        НЕ ЗАГРУЗИЛОСЬ · ПОВТОРИТЬ
      {:else}
        ПОКАЗАТЬ ЕЩЁ · ОСТАЛОСЬ {known - items.length}
      {/if}
    </button>
  {:else if stalled}
    <p class="note">
      Сервер больше ничего не отдал, хотя событий в разделе {known}. Обновите
      страницу — если повторится, это наша поломка, а не пустой раздел.
    </p>
  {:else if capped}
    <p class="note">
      Показаны первые {items.length} из {known}. Глубже лента не листается —
      выберите раздел выше, там событий меньше и они точнее.
    </p>
  {/if}
</main>

<footer>
  <p>ВШАГЕ · {new Date().getFullYear()}</p>
</footer>

<style>
  .nav {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
    gap: var(--sp-2);
    padding: var(--sp-3) var(--sp-4);
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: rgba(10, 10, 10, 0.9);
    backdrop-filter: blur(8px);
    z-index: 10;
  }
  .logo {
    font-family: var(--font-display);
    font-size: 14px;
    color: var(--accent-pink);
    letter-spacing: 1px;
  }
  /* На узком экране город уезжает на свою строку целиком — вместе с подписью
     «другие города — скоро», прятать которую нельзя. */
  .city { order: 3; flex: 1 1 100%; }
  .links { display: flex; align-items: center; gap: var(--sp-3); }
  .cta {
    background: var(--accent-green);
    color: #000;
    padding: 4px 10px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 1px;
  }
  @media (min-width: 720px) {
    .city { order: 0; flex: 0 1 auto; }
  }

  main {
    padding: var(--sp-4);
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
    max-width: 1280px;
    margin: 0 auto;
    /* Плитка прокручивается внутри себя — контейнеру нужен предел ширины,
       иначе flex-элемент раздувается под содержимое и уносит страницу. */
    min-width: 0;
  }
  .crumbs {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-2);
    font-size: 10px;
    letter-spacing: 1px;
    color: var(--mute);
    text-transform: uppercase;
    margin-bottom: calc(-1 * var(--sp-2));
  }
  .crumbs a:hover { color: var(--accent-pink); }
  .crumbs .sep { color: var(--border); }
  .crumbs .here { color: var(--fg); }

  .head { display: flex; flex-direction: column; gap: var(--sp-2); }
  h1 {
    font-family: var(--font-display);
    font-weight: 400;
    font-size: clamp(28px, 6vw, 56px);
    line-height: 0.95;
    letter-spacing: -1px;
    text-transform: uppercase;
  }
  .sub { font-size: 11px; color: var(--mute); letter-spacing: 0.5px; }

  .more {
    align-self: center;
    padding: var(--sp-3) var(--sp-6);
    background: transparent;
    border: 1px solid var(--accent-green);
    color: var(--accent-green);
    font-family: var(--font-mono);
    font-size: 11px;
    letter-spacing: 2px;
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease-out),
                color var(--dur-fast) var(--ease-out);
  }
  .more:hover:not(:disabled) { background: var(--accent-green); color: #000; }
  .more:disabled { opacity: 0.5; cursor: default; }
  .note {
    text-align: center;
    color: var(--mute);
    font-size: 11px;
    line-height: 1.6;
    max-width: 46ch;
    margin: 0 auto;
  }
  footer {
    text-align: center;
    padding: var(--sp-8) 0;
    color: var(--mute);
    font-size: 10px;
    letter-spacing: 2px;
  }
</style>
