<script lang="ts">
  import { APP_STORE_URL, openInApp } from '$lib/deeplink';
  let { eventId }: { eventId: string } = $props();
</script>

<a class="cta" href={APP_STORE_URL} onclick={(event) => {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  event.preventDefault();
  openInApp(eventId);
}}>
  ↓ ОТКРЫТЬ В ПРИЛОЖЕНИИ ВШАГЕ
</a>

<style>
  .cta {
    display: block;
    width: 100%;
    padding: var(--sp-3);
    background: var(--accent-pink);
    color: #000;
    border: none;
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 1.5px;
    cursor: pointer;
    text-align: center;
    transition: background var(--dur-fast);
  }
  .cta:hover { background: var(--accent-green); }
</style>
