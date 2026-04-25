<script lang="ts">
  import { enhance } from '$app/forms';
  import type { SubmitFunction } from '@sveltejs/kit';
  import { formatEventDateLong } from '$lib/dateFormat';

  let { data, form } = $props();
  let submitting = $state(false);

  const event = $derived(data.event);
  const capacityLeft = $derived(
    typeof event.max_attendees === 'number'
      ? Math.max(0, event.max_attendees - event.attendee_count)
      : null
  );
  const soldOut = $derived(capacityLeft !== null && capacityLeft <= 0);
  const deadlinePassed = $derived(
    event.registration_deadline ? Date.now() > new Date(event.registration_deadline).getTime() : false
  );
  const eventStarted = $derived(Date.now() > new Date(event.start_time).getTime());
  const registrationClosed = $derived(deadlinePassed || eventStarted);
  const external = $derived(event.registration_mode === 'external' && event.external_registration_url);
  const venue = $derived([event.venue_name, event.address || event.location].filter(Boolean).join(' · '));
  const price = $derived(formatPrice(event.price_type, event.price_min, event.price_max, event.currency));
  const modeLabel = $derived(event.registration_mode === 'manual' ? 'Заявка с подтверждением' : 'Регистрация сразу');

  const handleSubmit: SubmitFunction = () => {
    submitting = true;
    return async ({ update }) => {
      await update();
      submitting = false;
    };
  };

  function formatPrice(type?: string, min?: number, max?: number, currency = 'RUB') {
    if (!type || type === 'free') return 'Бесплатно';
    if (type === 'donation') return 'Донат';
    if (typeof min !== 'number' && typeof max !== 'number') return 'Платно';
    if (typeof min === 'number' && typeof max === 'number' && max !== min) return `${min}-${max} ${currency}`;
    return `${min ?? max} ${currency}`;
  }
</script>

<svelte:head>
  <title>Регистрация · {event.title} · Афиша Вшаге</title>
  <meta name="description" content={event.short_description ?? event.description?.slice(0, 160) ?? 'Регистрация на событие Вшаге'} />
</svelte:head>

<main class="register-page">
  <nav class="back-nav"><a href={`/${event.id}`}>← СОБЫТИЕ</a></nav>

  <section class="hero" style={event.photo_url ? `background-image: url(${event.photo_url})` : ''}>
    <div class="overlay"></div>
    <div class="hero-content">
      <div class="kicker">РЕГИСТРАЦИЯ</div>
      <h1>{event.title}</h1>
      {#if event.short_description}
        <p>{event.short_description}</p>
      {/if}
    </div>
  </section>

  <section class="info">
    <div class="row"><div class="k">КОГДА</div><div class="v">{formatEventDateLong(event.start_time)}</div></div>
    {#if venue}
      <div class="row"><div class="k">ГДЕ</div><div class="v">{venue}</div></div>
    {:else if event.online_url}
      <div class="row"><div class="k">ФОРМАТ</div><div class="v">ОНЛАЙН</div></div>
    {/if}
    <div class="row"><div class="k">ТИП</div><div class="v">{modeLabel}</div></div>
    <div class="row"><div class="k">ЦЕНА</div><div class="v">{price}</div></div>
    {#if capacityLeft !== null}
      <div class="row"><div class="k">МЕСТА</div><div class="v">{capacityLeft > 0 ? `${capacityLeft} из ${event.max_attendees}` : 'МЕСТ НЕТ'}</div></div>
    {/if}
    {#if event.registration_deadline}
      <div class="row"><div class="k">ДЕДЛАЙН</div><div class="v">{formatEventDateLong(event.registration_deadline)}</div></div>
    {/if}
  </section>

  {#if event.description}
    <section class="desc">{event.description}</section>
  {/if}

  {#if event.attendees_note || event.age_limit}
    <section class="note">
      {#if event.age_limit}<div>{event.age_limit}</div>{/if}
      {#if event.attendees_note}<div>{event.attendees_note}</div>{/if}
    </section>
  {/if}

  <section class="form-sec">
    {#if external}
      <div class="notice">Регистрация на это событие проходит на внешней странице.</div>
      <a class="submit external" href={event.external_registration_url} target="_blank" rel="noreferrer">ПЕРЕЙТИ К РЕГИСТРАЦИИ</a>
    {:else if soldOut}
      <div class="notice warn">Свободных мест больше нет.</div>
    {:else if registrationClosed}
      <div class="notice warn">Регистрация закрыта.</div>
    {:else}
      {#if form?.success}
        <div class="notice ok">
          {#if form.already_registered}
            Вы уже зарегистрированы на это событие.
          {:else if form.status === 'waitlisted'}
            Заявка отправлена. Организатор подтвердит участие.
          {:else}
            Регистрация принята.
          {/if}
        </div>
      {:else}
        <form method="POST" use:enhance={handleSubmit} class="reg-form">
          <label>
            <span>ИМЯ</span>
            <input name="name" autocomplete="name" value={form?.values?.name ?? ''} required minlength="2" maxlength="100" />
          </label>
          <label>
            <span>КОНТАКТ</span>
            <input name="contact" autocomplete="email" value={form?.values?.contact ?? ''} required minlength="5" maxlength="120" placeholder="телефон / email / telegram" />
          </label>
          {#if form?.error}
            <div class="error">{form.error}</div>
          {/if}
          <button class="submit" disabled={submitting}>{submitting ? 'ОТПРАВКА...' : 'ЗАРЕГИСТРИРОВАТЬСЯ'}</button>
        </form>
      {/if}
    {/if}
  </section>
</main>

<style>
  .register-page {
    max-width: 860px;
    margin: 0 auto;
    padding: var(--sp-4);
  }
  .back-nav { margin-bottom: var(--sp-3); }
  .back-nav a { color: var(--fg); font-size: 11px; letter-spacing: 1px; }
  .hero {
    position: relative;
    min-height: 300px;
    display: flex;
    align-items: flex-end;
    padding: var(--sp-5);
    background: linear-gradient(135deg, var(--accent-pink) 0%, #1a0014 100%);
    background-size: cover;
    background-position: center;
    border: 1px solid var(--border);
    overflow: hidden;
  }
  .overlay { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(0,0,0,0.15) 0%, rgba(0,0,0,0.9) 100%); }
  .hero-content { position: relative; z-index: 1; display: grid; gap: var(--sp-2); }
  .kicker, .row .k {
    color: var(--accent-green);
    font-size: 10px;
    letter-spacing: 2px;
    text-transform: uppercase;
  }
  h1 {
    font-family: var(--font-display);
    font-size: clamp(34px, 8vw, 72px);
    line-height: 0.95;
    text-transform: uppercase;
    font-weight: 400;
  }
  .hero p { max-width: 620px; color: #ddd; font-size: 13px; }
  .info { padding: var(--sp-4) 0; }
  .row { display: flex; justify-content: space-between; gap: var(--sp-4); padding: var(--sp-3) 0; border-bottom: 1px solid var(--border); font-size: 12px; }
  .row .v { text-align: right; color: var(--fg); }
  .desc, .note {
    color: #ccc;
    font-size: 13px;
    line-height: 1.7;
    white-space: pre-line;
    padding-bottom: var(--sp-4);
  }
  .note {
    display: grid;
    gap: var(--sp-2);
    color: var(--accent-green);
  }
  .form-sec {
    padding: var(--sp-4) 0 var(--sp-8);
  }
  .reg-form {
    display: grid;
    gap: var(--sp-3);
  }
  label {
    display: grid;
    gap: var(--sp-1);
  }
  label span {
    color: var(--mute);
    font-size: 10px;
    letter-spacing: 2px;
  }
  input {
    width: 100%;
    min-height: 48px;
    border: 1px solid var(--border);
    background: var(--bg-elev);
    color: var(--fg);
    font-family: var(--font-mono);
    font-size: 14px;
    padding: 0 var(--sp-3);
    outline: none;
  }
  input:focus {
    border-color: var(--accent-pink);
    box-shadow: 0 0 0 1px var(--accent-pink);
  }
  .submit {
    display: block;
    width: 100%;
    border: none;
    background: var(--accent-pink);
    color: #000;
    padding: var(--sp-3);
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 1.5px;
    text-align: center;
    cursor: pointer;
  }
  .submit:hover:not(:disabled) { background: var(--accent-green); }
  .submit:disabled { opacity: 0.55; cursor: wait; }
  .submit.external { text-decoration: none; }
  .notice, .error {
    border: 1px solid var(--border);
    background: var(--bg-elev);
    padding: var(--sp-3);
    margin-bottom: var(--sp-3);
    font-size: 12px;
  }
  .notice.ok { border-color: var(--accent-green); color: var(--accent-green); }
  .notice.warn, .error { border-color: var(--warning); color: var(--warning); }
  @media (max-width: 620px) {
    .register-page { padding: var(--sp-3); }
    .hero { min-height: 240px; padding: var(--sp-4); }
    .row { flex-direction: column; gap: var(--sp-1); }
    .row .v { text-align: left; }
  }
</style>
