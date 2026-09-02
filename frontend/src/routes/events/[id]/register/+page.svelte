<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { enhance } from '$app/forms';
  import type { SubmitFunction } from '@sveltejs/kit';
  import { formatEventDateLong } from '$lib/dateFormat';
  import type { RegField, RegFieldToggle } from '$lib/types';

  let { data, form } = $props();
  let submitting = $state(false);

  const event = $derived(data.event);
  const origin = $derived(page.url.origin);
  // Описание для превью: короткая строка организатора, иначе начало описания,
  // иначе дата и место — пустое og:description мессенджер рисует пустотой.
  const ogDescription = $derived(
    event.short_description?.trim() ||
      event.description?.trim().replace(/\s+/g, ' ').slice(0, 180) ||
      [formatEventDateLong(event.start_time), event.venue_name ?? event.address ?? event.city]
        .filter(Boolean)
        .join(' · ')
  );
  const ogImage = $derived(`${origin}/api/og/${event.id}?kind=register`);
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
  const registrationClosedReason = $derived(
    eventStarted
      ? 'Событие уже началось. Регистрация закрыта.'
      : event.registration_deadline && deadlinePassed
        ? `Дедлайн регистрации прошёл: ${formatEventDateLong(event.registration_deadline)}.`
        : 'Регистрация закрыта.'
  );
  const external = $derived(event.registration_mode === 'external' && event.external_registration_url);
  const venue = $derived([event.venue_name, event.address || event.location].filter(Boolean).join(' · '));
  const price = $derived(formatPrice(event.price_type, event.price_min, event.price_max, event.currency));
  const modeLabel = $derived(event.registration_mode === 'manual' ? 'Заявка с подтверждением' : 'Регистрация сразу');

  /** Свободный вариант в выпадающем списке с allow_other. */
  const OTHER_OPTION = 'Другое';
  const OFF: RegFieldToggle = { enabled: false, required: false };

  /**
   * Настраиваемая раскладка — или null, и тогда рисуется старая форма из двух
   * полей. Событие, заведённое до этой возможности, не должно поменяться под
   * человеком, который прямо сейчас его заполняет.
   *
   * Тумблеры нормализуются: v>=1 обещает все шесть, но неполный ответ должен
   * стоить нам ненарисованного поля, а не белого экрана.
   */
  const cfg = $derived.by(() => {
    const f = event.reg_form;
    if (!f || (f.v ?? 0) < 1) return null;
    return {
      name: f.name ?? OFF,
      full_name: f.full_name ?? OFF,
      email: f.email ?? OFF,
      phone: f.phone ?? OFF,
      tg: f.tg ?? OFF,
      contact: f.contact ?? OFF,
      pass_note: (f.pass_note ?? '').trim()
    };
  });
  const extraFields = $derived<RegField[]>(cfg ? (event.reg_fields ?? []) : []);

  // Стартовый снимок, а не подписка: при отрисовке на сервере после неудачной
  // отправки JS ещё не работал, и набранное обязано приехать прямо в разметке.
  // Дальше значения ведёт $effect ниже.
  const seeded = untrack(() => form?.values);
  let name = $state(seeded?.name ?? '');
  let fullName = $state(seeded?.full_name ?? '');
  let email = $state(seeded?.email ?? '');
  let phone = $state(seeded?.phone ?? '');
  let tgUsername = $state(seeded?.tg_username ?? '');
  let contact = $state(seeded?.contact ?? '');
  let answers = $state<Record<string, string>>({ ...(seeded?.answers ?? {}) });
  let answersOther = $state<Record<string, string>>({ ...(seeded?.answers_other ?? {}) });
  /** Ошибки нашей проверки. Серверные приходят в form.fields. */
  let clientErrors = $state<Record<string, string>>({});

  // Возвращаем набранное после неудачной отправки. Значение поля живёт в
  // состоянии, а не в атрибуте value: атрибут переприменяется на каждой
  // перерисовке и стирает уже введённое (ловушка веб-регистрации 17.08).
  $effect(() => {
    const v = form?.values;
    if (!v) return;
    name = v.name ?? '';
    fullName = v.full_name ?? '';
    email = v.email ?? '';
    phone = v.phone ?? '';
    tgUsername = v.tg_username ?? '';
    contact = v.contact ?? '';
    answers = { ...(v.answers ?? {}) };
    answersOther = { ...(v.answers_other ?? {}) };
  });

  const fieldErrors = $derived<Record<string, string>>({
    ...((form?.fields ?? {}) as Record<string, string>),
    ...clientErrors
  });

  /** Подписи по имени поля — нужны, чтобы назвать поля в сводке у кнопки. */
  const fieldLabels = $derived.by(() => {
    const m: Record<string, string> = {};
    if (!cfg) {
      m.name = 'ИМЯ';
      m.contact = 'КОНТАКТ';
      return m;
    }
    if (cfg.name.enabled) m.name = 'ИМЯ';
    if (cfg.full_name.enabled) m.full_name = 'ФИО КАК В ДОКУМЕНТЕ';
    if (cfg.email.enabled) m.email = 'ПОЧТА';
    if (cfg.phone.enabled) m.phone = 'ТЕЛЕФОН';
    if (cfg.tg.enabled) m.tg_username = 'ТЕЛЕГРАМ';
    if (cfg.contact.enabled) m.contact = 'КОНТАКТ';
    for (const f of extraFields) {
      m[`answer:${f.key}`] = f.label;
      m[`answer_other:${f.key}`] = f.label;
    }
    return m;
  });

  /** Сообщение у кнопки: без него отказ виден только вверху, за экраном. */
  const clientSummary = $derived.by(() => {
    const keys = Object.keys(clientErrors);
    if (!keys.length) return '';
    const named = keys.map((k) => fieldLabels[k] ?? k).join(', ');
    return `Проверьте поля: ${named}`;
  });

  const handleSubmit: SubmitFunction = ({ cancel, formElement }) => {
    const errs = validate();
    clientErrors = errs;
    const first = Object.keys(errs)[0];
    if (first) {
      // Форма с novalidate: браузер молчит, значит объяснить отказ обязаны мы,
      // и сразу показать, какое поле виновато.
      cancel();
      const el = formElement.querySelector<HTMLElement>(`[name="${first}"]`);
      el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      setTimeout(() => el?.focus(), 300);
      return;
    }
    submitting = true;
    return async ({ update }) => {
      // reset:false — сброс формы вернул бы поля к атрибутам, а состояние
      // связок об этом не узнало бы.
      await update({ reset: false });
      submitting = false;
    };
  };

  function plural(n: number, one: string, few: string, many: string): string {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return one;
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
    return many;
  }

  function validate(): Record<string, string> {
    const errs: Record<string, string> = {};
    const check = (key: string, value: string, required: boolean, min = 0) => {
      const v = value.trim();
      if (!v) {
        if (required) errs[key] = 'Заполните это поле';
        return;
      }
      if (min && v.length < min) {
        errs[key] = `Минимум ${min} ${plural(min, 'символ', 'символа', 'символов')}`;
      }
    };

    if (!cfg) {
      check('name', name, true, 2);
      check('contact', contact, true, 5);
      return errs;
    }

    if (cfg.name.enabled) check('name', name, cfg.name.required, 2);
    if (cfg.full_name.enabled) check('full_name', fullName, cfg.full_name.required);
    if (cfg.email.enabled) check('email', email, cfg.email.required);
    if (cfg.phone.enabled) check('phone', phone, cfg.phone.required);
    if (cfg.tg.enabled) check('tg_username', tgUsername, cfg.tg.required);
    if (cfg.contact.enabled) check('contact', contact, cfg.contact.required, 5);

    for (const f of extraFields) {
      const key = `answer:${f.key}`;
      const value = answers[f.key] ?? '';
      if (f.type === 'checkbox') {
        if (f.required && value !== 'Да') errs[key] = 'Отметьте это поле';
        continue;
      }
      if (f.required && !value.trim()) {
        errs[key] = 'Заполните это поле';
        continue;
      }
      if (f.type === 'select' && value === OTHER_OPTION && !(answersOther[f.key] ?? '').trim()) {
        errs[`answer_other:${f.key}`] = 'Впишите свой вариант';
      }
    }
    return errs;
  }

  function optionsFor(f: RegField): string[] {
    const base = f.options ?? [];
    return f.allow_other ? [...base, OTHER_OPTION] : base;
  }

  /**
   * «ПОЧТА» / «ПОЧТА · НЕОБЯЗАТЕЛЬНО» — метка несёт статус поля.
   * Регистр пометки берём у самой метки: свои подписи на этой странице капсом,
   * а подписи организатора приходят как есть и капс им не идёт.
   */
  function labelFor(text: string, required: boolean): string {
    if (required) return text;
    const upper = text === text.toUpperCase();
    return `${text} · ${upper ? 'НЕОБЯЗАТЕЛЬНО' : 'необязательно'}`;
  }

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
  <meta name="description" content={ogDescription} />
  <!--
    Именно эту ссылку организатор рассылает людям, и до 03.09 у неё не было
    ни одного og-тега: в телеграме она разворачивалась голой строкой адреса.
    Карточка берётся с kind=register — у неё свой кикер «ЗАПИСЬ ОТКРЫТА».
  -->
  <meta property="og:site_name" content="Вшаге" />
  <meta property="og:locale" content="ru_RU" />
  <meta property="og:type" content="website" />
  <meta property="og:title" content={`${event.title} — запись открыта`} />
  <meta property="og:description" content={ogDescription} />
  <meta property="og:url" content={`${origin}/events/${event.id}/register`} />
  <meta property="og:image" content={ogImage} />
  <meta property="og:image:width" content="1200" />
  <meta property="og:image:height" content="630" />
  <meta property="og:image:alt" content={event.title} />
  <meta name="twitter:card" content="summary_large_image" />
  <meta name="twitter:title" content={`${event.title} — запись открыта`} />
  <meta name="twitter:description" content={ogDescription} />
  <meta name="twitter:image" content={ogImage} />
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
      <div class="notice warn">{registrationClosedReason}</div>
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
        <!-- novalidate: нативная валидация глушит submit до JS, и кнопка
             выглядит мёртвой — свои сообщения показываем сами. -->
        <form method="POST" use:enhance={handleSubmit} class="reg-form" novalidate>
          {#if !cfg}
            <label>
              <span>ИМЯ</span>
              <input name="name" autocomplete="name" bind:value={name} required minlength="2" maxlength="100" class:invalid={fieldErrors.name} />
              {#if fieldErrors.name}<span class="field-err">{fieldErrors.name}</span>{/if}
            </label>
            <label>
              <span>КОНТАКТ</span>
              <input name="contact" autocomplete="email" bind:value={contact} required minlength="5" maxlength="120" placeholder="телефон / email / telegram" class:invalid={fieldErrors.contact} />
              {#if fieldErrors.contact}<span class="field-err">{fieldErrors.contact}</span>{/if}
            </label>
          {:else}
            {#if cfg.name.enabled}
              <label>
                <span>{labelFor('ИМЯ', cfg.name.required)}</span>
                <input name="name" autocomplete="name" bind:value={name} required={cfg.name.required} minlength="2" maxlength="100" class:invalid={fieldErrors.name} />
                {#if fieldErrors.name}<span class="field-err">{fieldErrors.name}</span>{/if}
              </label>
            {/if}

            {#if cfg.full_name.enabled}
              <label>
                <span>{labelFor('ФИО КАК В ДОКУМЕНТЕ', cfg.full_name.required)}</span>
                <input name="full_name" autocomplete="name" bind:value={fullName} required={cfg.full_name.required} maxlength="150" placeholder="Иванов Иван Иванович" class:invalid={fieldErrors.full_name} />
                {#if fieldErrors.full_name}
                  <span class="field-err">{fieldErrors.full_name}</span>
                {:else if cfg.pass_note}
                  <span class="field-hint">{cfg.pass_note}</span>
                {/if}
              </label>
            {/if}

            {#if cfg.email.enabled}
              <label>
                <span>{labelFor('ПОЧТА', cfg.email.required)}</span>
                <input name="email" type="email" inputmode="email" autocomplete="email" autocapitalize="none" autocorrect="off" spellcheck="false" bind:value={email} required={cfg.email.required} maxlength="120" placeholder="you@example.com" class:invalid={fieldErrors.email} />
                {#if fieldErrors.email}<span class="field-err">{fieldErrors.email}</span>{/if}
              </label>
            {/if}

            {#if cfg.phone.enabled}
              <label>
                <span>{labelFor('ТЕЛЕФОН', cfg.phone.required)}</span>
                <input name="phone" type="tel" inputmode="tel" autocomplete="tel" bind:value={phone} required={cfg.phone.required} maxlength="30" placeholder="+7 903 123-45-67" class:invalid={fieldErrors.phone} />
                {#if fieldErrors.phone}<span class="field-err">{fieldErrors.phone}</span>{/if}
              </label>
            {/if}

            {#if cfg.tg.enabled}
              <label>
                <span>{labelFor('ТЕЛЕГРАМ', cfg.tg.required)}</span>
                <input name="tg_username" autocapitalize="none" autocorrect="off" spellcheck="false" bind:value={tgUsername} required={cfg.tg.required} maxlength="64" placeholder="@username" class:invalid={fieldErrors.tg_username} />
                {#if fieldErrors.tg_username}<span class="field-err">{fieldErrors.tg_username}</span>{/if}
              </label>
            {/if}

            {#if cfg.contact.enabled}
              <label>
                <span>{labelFor('КОНТАКТ', cfg.contact.required)}</span>
                <input name="contact" autocomplete="email" bind:value={contact} required={cfg.contact.required} minlength="5" maxlength="120" placeholder="телефон / email / telegram" class:invalid={fieldErrors.contact} />
                {#if fieldErrors.contact}<span class="field-err">{fieldErrors.contact}</span>{/if}
              </label>
            {/if}

            {#each extraFields as f (f.key)}
              {#if f.type === 'checkbox'}
                <div class="field">
                  <label class="check-row" class:invalid={fieldErrors[`answer:${f.key}`]}>
                    <input type="checkbox" name={`answer:${f.key}`} value="Да" checked={answers[f.key] === 'Да'} onchange={(e) => (answers[f.key] = e.currentTarget.checked ? 'Да' : '')} />
                    <span>{labelFor(f.label, Boolean(f.required))}</span>
                  </label>
                  {#if fieldErrors[`answer:${f.key}`]}
                    <span class="field-err">{fieldErrors[`answer:${f.key}`]}</span>
                  {:else if f.hint}
                    <span class="field-hint">{f.hint}</span>
                  {/if}
                </div>
              {:else}
                <div class="field">
                  <span class="field-label">{labelFor(f.label, Boolean(f.required))}</span>
                  {#if f.type === 'select'}
                    <select name={`answer:${f.key}`} bind:value={answers[f.key]} required={f.required} class:invalid={fieldErrors[`answer:${f.key}`]}>
                      <option value="" disabled selected={!answers[f.key]}>Выберите из списка</option>
                      {#each optionsFor(f) as opt}
                        <option value={opt}>{opt}</option>
                      {/each}
                    </select>
                    {#if answers[f.key] === OTHER_OPTION}
                      <input name={`answer_other:${f.key}`} bind:value={answersOther[f.key]} maxlength={f.max_len || 500} placeholder="Свой вариант" class:invalid={fieldErrors[`answer_other:${f.key}`]} />
                      {#if fieldErrors[`answer_other:${f.key}`]}
                        <span class="field-err">{fieldErrors[`answer_other:${f.key}`]}</span>
                      {/if}
                    {/if}
                  {:else if f.type === 'textarea'}
                    <textarea name={`answer:${f.key}`} rows="4" maxlength={f.max_len || 500} bind:value={answers[f.key]} required={f.required} class:invalid={fieldErrors[`answer:${f.key}`]}></textarea>
                  {:else}
                    <input name={`answer:${f.key}`} maxlength={f.max_len || 500} bind:value={answers[f.key]} required={f.required} class:invalid={fieldErrors[`answer:${f.key}`]} />
                  {/if}
                  {#if fieldErrors[`answer:${f.key}`]}
                    <span class="field-err">{fieldErrors[`answer:${f.key}`]}</span>
                  {:else if f.hint}
                    <span class="field-hint">{f.hint}</span>
                  {/if}
                </div>
              {/if}
            {/each}
          {/if}

          {#if form?.error}
            <div class="error">{form.error}</div>
          {/if}
          {#if clientSummary}
            <div class="error">{clientSummary}</div>
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
  label, .field {
    display: grid;
    gap: var(--sp-1);
  }
  label span, .field-label {
    color: var(--mute);
    font-size: 10px;
    letter-spacing: 2px;
  }
  input:not([type='checkbox']), select, textarea {
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
  input:not([type='checkbox']):focus, select:focus, textarea:focus {
    border-color: var(--accent-pink);
    box-shadow: 0 0 0 1px var(--accent-pink);
  }
  textarea {
    padding: var(--sp-3);
    line-height: 1.6;
    resize: vertical;
  }
  select option { background: var(--bg-elev); color: var(--fg); }
  .invalid { border-color: var(--warning); }
  .check-row {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-2);
    min-height: 44px;
    cursor: pointer;
  }
  .check-row input[type='checkbox'] {
    width: 18px;
    height: 18px;
    margin-top: 2px;
    accent-color: var(--accent-pink);
    flex: none;
  }
  /* Подпись галочки — читаемый текст, а не мелкий разрежённый ключ. */
  .check-row span {
    color: var(--fg);
    font-size: 13px;
    letter-spacing: 0;
    line-height: 1.5;
  }
  .field-err, .field-hint {
    font-size: 11px;
    letter-spacing: 0;
    line-height: 1.5;
  }
  .field-err { color: var(--warning); }
  .field-hint { color: var(--mute); }
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
