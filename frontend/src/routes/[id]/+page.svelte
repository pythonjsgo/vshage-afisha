<script lang="ts">
  import EventDetail from '$lib/components/EventDetail.svelte';
  import { page } from '$app/state';
  let { data } = $props();
  const origin = $derived(page.url.origin);
</script>

<svelte:head>
  <title>{data.event.title} · Афиша Вшаге</title>
  <meta name="description" content={data.event.description?.slice(0, 160) ?? 'Событие Вшаге'} />
  <meta property="og:title" content={data.event.title} />
  <meta property="og:description" content={data.event.description?.slice(0, 160) ?? 'Событие Вшаге'} />
  <meta property="og:image" content={`${origin}/api/og/${data.event.id}`} />
  <meta property="og:url" content={`${origin}/${data.event.id}`} />
  <meta property="og:type" content="website" />
  <meta name="twitter:card" content="summary_large_image" />
</svelte:head>

<EventDetail event={data.event} {origin} />
