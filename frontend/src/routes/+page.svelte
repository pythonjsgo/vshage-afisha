<script lang="ts">
  import HeroFeatured from '$lib/components/HeroFeatured.svelte';
  import EventGrid from '$lib/components/EventGrid.svelte';
  let { data } = $props();
</script>

<svelte:head>
  <title>АФИША · ВШАГЕ</title>
  <meta name="description" content="Все события Вшаге — паблик афиша" />
</svelte:head>

<header class="nav">
  <a href="/" class="logo">АФИША_ВШАГЕ</a>
  <nav class="links">
    <a class="cta" href="#app">↓ В ПРИЛОЖЕНИИ</a>
  </nav>
</header>

<main>
  {#if data.featured.length > 0}
    <HeroFeatured events={data.featured} />
  {/if}

  <!-- Подпись честная: «ВСЕ СОБЫТИЯ · 300» над тридцатью карточками
       читается как поломка выдачи, а не как пагинация, которой тут нет. -->
  <EventGrid
    events={data.all}
    label={data.all.length < data.total
      ? `СОБЫТИЯ · ${data.all.length} ИЗ ${data.total}`
      : `ВСЕ СОБЫТИЯ · ${data.total}`}
  />
</main>

<footer>
  <p>ВШАГЕ · {new Date().getFullYear()}</p>
</footer>

<style>
  .nav {
    display: flex; justify-content: space-between; align-items: center;
    padding: var(--sp-3) var(--sp-4);
    border-bottom: 1px solid var(--border);
    position: sticky; top: 0; background: rgba(10,10,10,0.9); backdrop-filter: blur(8px); z-index: 10;
  }
  .logo { font-family: var(--font-display); font-size: 14px; color: var(--accent-pink); letter-spacing: 1px; }
  .links { display: flex; align-items: center; gap: var(--sp-3); }
  .cta { background: var(--accent-green); color: #000; padding: 4px 10px; font-size: 10px; font-weight: 700; letter-spacing: 1px; }
  main { padding: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-4); max-width: 1280px; margin: 0 auto; }
  footer { text-align: center; padding: var(--sp-8) 0; color: var(--mute); font-size: 10px; letter-spacing: 2px; }
</style>
