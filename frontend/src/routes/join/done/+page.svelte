<script lang="ts">
  import { AFISHA_URL, APP_STORE_URL } from '$lib/join';

  let { data } = $props();

  const title = 'Анкету получили — Вшаге';
  // 'unknown' — это в том числе iPad с iPadOS 13+, который представляется
  // Macintosh. Показываем обе кнопки: одна из них человеку точно подойдёт,
  // а угадывать за него — значит половине предложить чужую.
  const showApp = $derived(data.platform === 'ios' || data.platform === 'unknown');
  const showAfisha = $derived(data.platform === 'other' || data.platform === 'unknown');
</script>

<svelte:head>
  <title>{title}</title>
  <meta name="description" content="Анкета получена. Напишем в телеграм в течение двух дней." />
  <link rel="canonical" href="https://vshage.app/join/done" />
</svelte:head>

<main>
  <div class="wordmark">VSHAGE</div>
  <h1>Анкету получили</h1>
  <p class="lead">Напишем в телеграм в течение двух дней.</p>

  <div class="actions">
    {#if showApp}
      <a class="btn" href={APP_STORE_URL} rel="noopener">Поставить Вшаге</a>
    {/if}
    {#if showAfisha}
      <a class="btn" class:ghost={showApp} href={AFISHA_URL}>Открыть афишу</a>
    {/if}
  </div>
</main>

<style>
  main {
    max-width: 560px;
    margin: 0 auto;
    padding: var(--sp-10) var(--sp-4);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: 28px;
    letter-spacing: 1px;
    line-height: 1;
    margin-bottom: var(--sp-6);
  }

  h1 {
    font-size: clamp(26px, 7vw, 38px);
    line-height: 1.1;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: -0.5px;
  }

  .lead {
    margin-top: var(--sp-4);
    color: var(--mute);
    font-size: 15px;
    line-height: 1.6;
  }

  .actions {
    margin-top: var(--sp-8);
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 52px;
    padding: 0 var(--sp-5);
    border: 1px solid var(--fg);
    border-radius: 12px;
    background: var(--fg);
    color: #000;
    font-size: 15px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 1px;
  }
  .btn.ghost {
    background: transparent;
    border-color: var(--border);
    color: var(--fg);
  }
</style>
