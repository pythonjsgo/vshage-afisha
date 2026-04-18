<script lang="ts">
  let { text = '', variant = 'default' }: { text: string; variant?: 'default' | 'live' | 'warning' } = $props();
</script>

<span class="pill" class:live={variant === 'live'} class:warn={variant === 'warning'}>
  {text}
</span>

<style>
  .pill {
    display: inline-block;
    padding: 2px 6px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 1px;
    text-transform: uppercase;
    background: var(--bg-elev);
    border: 1px solid var(--border);
    color: var(--fg);
  }
  .live { background: var(--accent-pink); color: #000; border-color: var(--accent-pink); }
  .warn { background: var(--warning); color: #000; border-color: var(--warning); }
</style>
