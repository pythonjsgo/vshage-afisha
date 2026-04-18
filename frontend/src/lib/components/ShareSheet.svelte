<script lang="ts">
  let { url, title }: { url: string; title: string } = $props();
  let copied = $state(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      copied = true;
      setTimeout(() => copied = false, 2000);
    } catch {}
  }
  const tg = $derived(`https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(title)}`);
  const wa = $derived(`https://wa.me/?text=${encodeURIComponent(title + ' ' + url)}`);
</script>

<div class="share">
  <button onclick={copy} class:on={copied}>{copied ? 'СКОПИРОВАНО' : '⌘ COPY'}</button>
  <a href={tg} target="_blank" rel="noreferrer">TELEGRAM</a>
  <a href={wa} target="_blank" rel="noreferrer">WHATSAPP</a>
</div>

<style>
  .share { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: var(--sp-2); }
  .share button, .share a {
    display: block;
    padding: var(--sp-2);
    text-align: center;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg);
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 1px;
    cursor: pointer;
    text-decoration: none;
  }
  .share button.on { background: var(--accent-green); color: #000; border-color: var(--accent-green); }
</style>
