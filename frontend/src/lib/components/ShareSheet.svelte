<script lang="ts">
  import { onMount } from 'svelte';
  import { documentEventLocale, eventCopy, type EventLocale } from '$lib/event-copy';
  let { url, title }: { url: string; title: string } = $props();
  let copied = $state(false);
  let failed = $state(false);
  let locale = $state<EventLocale>('ru');
  let timer: ReturnType<typeof setTimeout> | undefined;
  onMount(() => { locale = documentEventLocale(); return () => clearTimeout(timer); });

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      copied = true;
      failed = false;
      clearTimeout(timer);
      timer = setTimeout(() => copied = false, 2000);
    } catch { copied = false; failed = true; clearTimeout(timer); }
  }
  const tg = $derived(`https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(title)}`);
</script>

<details class="share-options">
  <summary>{eventCopy('share', locale)}</summary>
  <div class="share">
    <button type="button" onclick={copy} class:on={copied}>{eventCopy(copied ? 'copied' : 'copyLink', locale)}</button>
    <a href={tg} target="_blank" rel="noreferrer">{eventCopy('telegramShare', locale)}</a>
  </div>
  {#if failed}<p role="status">{eventCopy('copyFailed', locale)} <a href={url}>{url}</a></p>{/if}
</details>

<style>
  .share-options { color: var(--mute); font-size: 12px; }
  summary { cursor: pointer; width: fit-content; padding: 8px 0; }
  summary:hover { color: var(--fg); }
  .share { margin-top: var(--sp-1); }
  .share-options p { overflow-wrap: anywhere; }
  .share-options a:focus-visible, .share-options button:focus-visible, summary:focus-visible { outline: 2px solid var(--accent-green); outline-offset: 3px; }
  .share { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-2); }
  .share button, .share a {
    display: block;
    padding: var(--sp-2);
    text-align: center;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg);
    font-family: var(--font-mono);
    font-size: 12px;
    letter-spacing: 1px;
    cursor: pointer;
    text-decoration: none;
  }
  .share button.on { background: var(--accent-green); color: #000; border-color: var(--accent-green); }
  @media (max-width: 480px) { .share { grid-template-columns: 1fr; } }
</style>
