<script lang="ts">
  import type { PublicEvent } from '$lib/types';
  import EventCard from './EventCard.svelte';
  let { events, label = '' }: { events: PublicEvent[]; label?: string } = $props();
</script>

{#if label}
  <div class="grid-label">{label}</div>
{/if}

<div class="grid">
  {#each events as e (e.id)}
    <EventCard event={e} />
  {/each}
</div>

{#if events.length === 0}
  <div class="empty">ПОКА ПУСТО · ЗАГЛЯНИ ЗАВТРА</div>
{/if}

<style>
  .grid-label {
    font-size: 10px;
    letter-spacing: 2px;
    color: var(--accent-green);
    padding: var(--sp-4) 0 var(--sp-3);
    border-top: 1px solid var(--border);
  }
  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--sp-3);
  }
  @media (min-width: 640px) {
    .grid { grid-template-columns: 1fr 1fr; gap: var(--sp-4); }
  }
  @media (min-width: 1024px) {
    .grid { grid-template-columns: 1fr 1fr 1fr; }
  }
  .empty {
    padding: var(--sp-8);
    text-align: center;
    color: var(--mute);
    font-size: 12px;
    letter-spacing: 2px;
  }
</style>
