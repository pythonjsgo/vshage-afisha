<script lang="ts">
  import '../app.css';
  import Scanlines from '$lib/components/Scanlines.svelte';
  let { children, data } = $props();
</script>

<svelte:head>
  <!-- Метки владения сайтом. Значение приезжает из окружения (см.
       +layout.server.ts); пустое — тега нет вовсе. -->
  {#if data?.verification?.yandex}
    <meta name="yandex-verification" content={data.verification.yandex} />
  {/if}
  {#if data?.verification?.google}
    <meta name="google-site-verification" content={data.verification.google} />
  {/if}
</svelte:head>

<Scanlines />
{@render children?.()}
