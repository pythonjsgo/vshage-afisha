<script lang="ts">
  import { openInApp } from '$lib/deeplink';
  let { eventId }: { eventId: string } = $props();
</script>

<button class="cta" onclick={() => openInApp(eventId)}>
  ↓ ОТКРЫТЬ В ПРИЛОЖЕНИИ ВШАГЕ
</button>

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
    transition: background var(--dur-fast);
  }
  .cta:hover { background: var(--accent-green); }
</style>
