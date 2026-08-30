<script lang="ts">
  import EventDetail from '$lib/components/EventDetail.svelte';
  import { page } from '$app/state';
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
  const foreign = $derived(Boolean(data.event.webreg_slug) || data.event.source === 'tg');
  const ogImage = $derived(
    foreign
      ? (data.event.photo_url
          ? (data.event.photo_url.startsWith('http') ? data.event.photo_url : `${origin}${data.event.photo_url}`)
          : `${origin}/og-default.png`)
      : `${origin}/api/og/${data.event.id}`
  );
</script>

<svelte:head>
  <title>{data.event.title} · Афиша Вшаге</title>
  <meta name="description" content={data.event.description?.slice(0, 160) ?? 'Событие Вшаге'} />
  <meta property="og:title" content={data.event.title} />
  <meta property="og:description" content={data.event.description?.slice(0, 160) ?? 'Событие Вшаге'} />
  <meta property="og:image" content={ogImage} />
  <meta property="og:url" content={`${origin}/${data.event.id}`} />
  <meta property="og:type" content="website" />
  <meta name="twitter:card" content="summary_large_image" />
</svelte:head>

<EventDetail event={data.event} {origin} />
