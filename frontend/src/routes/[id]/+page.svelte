<script lang="ts">
  import EventDetail from '$lib/components/EventDetail.svelte';
  import { page } from '$app/state';
  import { eventJsonLd, eventUrl, jsonLdScript, type City } from '$lib/seo';
  let { data } = $props();
  const origin = $derived(page.url.origin);
  // Сгенерированная карточка /api/og/<uuid> положена ТОЛЬКО событиям общей
  // таблицы: тот роут ходит в /api/events/<id>, где id — UUID. Для слага
  // веб-регистрации он ответит 404, а для импортированного ev_* — 500
  // (Postgres не приведёт «ev_09333…» к uuid), и превью не будет вовсе.
  // Отказ при этом немой: страница открывается, мета-тег на месте, картинка
  // просто не грузится у получателя — а подписанный источник и превью это и
  // есть та половина условия, ради которой мы чужие анонсы показываем.
  // photo_url у импортированных относительный, поэтому origin обязателен:
  // og:image требует абсолютный адрес.
  const ogDescription = $derived(
    data.event.short_description?.trim() ||
      data.event.description?.trim().replace(/\s+/g, ' ').slice(0, 180) ||
      'Событие в афише Вшаге'
  );
  const foreign = $derived(Boolean(data.event.webreg_slug) || data.event.source === 'tg');
  const ogImage = $derived(
    foreign
      ? (data.event.photo_url
          ? (data.event.photo_url.startsWith('http') ? data.event.photo_url : `${origin}${data.event.photo_url}`)
          : `${origin}/og-default.png`)
      : `${origin}/api/og/${data.event.id}`
  );

  // Канонический адрес. На одну карточку ведут ТРИ маршрута — /<id>,
  // /events/<id> (308) и /e/<slug> у веб-регистрации, — и без этой строки
  // поисковик считает их разными страницами с одинаковым текстом и делит вес
  // между ними. Адрес собирает eventUrl и только он: карта сайта зовёт ту же
  // функцию, поэтому разъехаться они не могут.
  //
  // og:url намеренно оставлен прежним (/<id>): мессенджер показывает витрину
  // с обложкой и полным описанием — это разные вопросы. Поисковику мы
  // называем адрес, который индексируем; мессенджеру — тот, которым делимся.
  const canonical = $derived(eventUrl(origin, data.event));

  // Город нужен разметке ради addressLocality: без города адрес события
  // читается роботом как «где-то в России», и в локальной выдаче карточка не
  // участвует. Падежи здесь не нужны — eventJsonLd берёт только name; полная
  // форма города живёт в бэкенде (internal/events/cities.go).
  const cityForLd = $derived<City | undefined>(
    data.event.city ? { slug: '', name: data.event.city, in: '', of: '' } : undefined
  );
  const eventLd = $derived(jsonLdScript(eventJsonLd(data.event, origin, cityForLd)));
</script>

<svelte:head>
  <title>{data.event.title} · Афиша Вшаге</title>
  <meta name="description" content={ogDescription} />
  <link rel="canonical" href={canonical} />
  <meta property="og:site_name" content="Вшаге" />
  <meta property="og:locale" content="ru_RU" />
  <meta property="og:type" content="article" />
  <meta property="og:title" content={data.event.title} />
  <meta property="og:description" content={ogDescription} />
  <meta property="og:image" content={ogImage} />
  <!-- Размеры объявляются ТОЛЬКО у нашей карточки: телеграм верит числам, а
       не файлу, и 1200×630 на вертикальной чужой афише даёт обрезанное
       превью. У импортированных событий обложка своя, пропорций мы не знаем —
       пусть мессенджер измерит сам. -->
  {#if !foreign}
    <meta property="og:image:width" content="1200" />
    <meta property="og:image:height" content="630" />
  {/if}
  <meta property="og:image:alt" content={data.event.title} />
  <meta property="og:url" content={`${origin}/${data.event.id}`} />
  <meta name="twitter:card" content="summary_large_image" />
  <meta name="twitter:title" content={data.event.title} />
  <meta name="twitter:description" content={ogDescription} />
  <meta name="twitter:image" content={ogImage} />
  <!-- schema.org/Event — то, ради чего разметка и нужна: без неё афиша не
       попадает ни в блок мероприятий Google, ни в колдунщик Яндекса.
       Экранирование делает jsonLdScript: заголовок едет из телеграма и может
       содержать закрывающий тег script, который иначе разорвал бы разметку.
       Поэтому здесь {@html} без обработки — вторая только испортила бы JSON. -->
  {@html `<script type="application/ld+json">${eventLd}<\/script>`}
</svelte:head>

<EventDetail event={data.event} {origin} />
