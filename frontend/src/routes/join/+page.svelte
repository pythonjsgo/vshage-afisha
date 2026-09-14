<script lang="ts">
  import { enhance } from '$app/forms';
  import { page } from '$app/state';
  import type { SubmitFunction } from '@sveltejs/kit';
  import {
    ABOUT_MAX,
    COURSES,
    JOIN_CANONICAL,
    JOIN_OG_IMAGE,
    OTHER_UNIVERSITY,
    PRIVACY_URL,
    TRACKING_KEYS,
    UNIVERSITIES,
    looksLikeTelegram
  } from '$lib/join';

  let { data, form } = $props();

  const title = 'Вшаге — закрытая сеть студентов Москвы';
  const description =
    'Закрытая сеть студентов сильных вузов Москвы. Вход по анкете: заполни за минуту, ' +
    'и мы напишем в телеграм.';

  const fieldErrors = $derived((form?.fieldErrors ?? {}) as Record<string, string>);
  const values = $derived((form?.values ?? {}) as Record<string, string | boolean>);
  // «Анкета уже есть» — не ошибка ввода, и рисовать её красным неправильно:
  // человек всё сделал верно, просто дважды.
  const duplicate = $derived(form?.code === 'already_submitted');

  // ВСЕ поля — через bind:, ни одного `value={…}`. Атрибут `value` на поле
  // формы Svelte переприменяет при перерисовке, и набранное стирается на
  // соседнем действии: этим у нас однажды обнулялись имя и телеграм в форме
  // регистрации, а выглядело как «человек не заполнил».
  //
  // Начальное значение берётся ЗДЕСЬ, а не только в $effect ниже: эффекты на
  // сервере не выполняются вовсе, и у человека без JS страница с ошибкой
  // валидации возвращалась бы с пустыми полями — то есть заполненная анкета
  // на платном трафике просто пропадала бы. Чтение через функцию, чтобы не
  // ловить предупреждение «reference only captures the initial value»:
  // начальное значение нам тут и нужно, дальнейшие приносит $effect.
  const seed = () => (form?.values ?? {}) as Record<string, string | boolean>;
  let name = $state(String(seed().name ?? ''));
  let university = $state(String(seed().university ?? ''));
  let universityOther = $state(String(seed().university_other ?? ''));
  let course = $state(String(seed().course ?? ''));
  let about = $state(String(seed().about ?? ''));
  let telegram = $state(String(seed().telegram ?? ''));
  let consent = $state(Boolean(seed().consent));
  let submitting = $state(false);
  // Гидрация состоялась. Без неё галочка `bind:checked` мертва, и кнопка,
  // отрисованная сервером как disabled, НЕ разблокируется никогда: человек
  // ставит галочку, ничего не происходит, ошибки нет — ровно «мёртвая
  // кнопка». Поэтому сервер рисует кнопку живой, а гейт по согласию
  // включается только когда его есть кому снять. Согласие всё равно
  // проверяет сервер (join.go), клиентский гейт — только удобство.
  let hydrated = $state(false);
  $effect(() => {
    hydrated = true;
  });

  // Значения, вернувшиеся с сервера после ошибки, — единственный источник
  // для перерисовки. Локальное состояние синхронизируем один раз на ответ,
  // иначе набранное затрётся при каждом перерисовывании (грабли Svelte с
  // `value={...}` на поле формы уже стоили нам обнулённой формы).
  $effect(() => {
    if (!form) return;
    name = String(values.name ?? '');
    university = String(values.university ?? '');
    universityOther = String(values.university_other ?? '');
    course = String(values.course ?? '');
    about = String(values.about ?? '');
    telegram = String(values.telegram ?? '');
    consent = Boolean(values.consent);
  });

  const aboutLeft = $derived(ABOUT_MAX - about.length);
  const telegramLooksWrong = $derived(telegram.trim() !== '' && !looksLikeTelegram(telegram));

  const handleSubmit: SubmitFunction = () => {
    submitting = true;
    return async ({ result, update }) => {
      try {
        // Цель Метрики бьётся ТОЛЬКО на подтверждённой сервером отправке.
        // Повесить её на клик по кнопке значило бы считать целью и то, что
        // отвалилось валидацией, — и вся стоимость лида поехала бы.
        if (result.type === 'redirect') reachGoal();
        await update({ reset: false });
      } finally {
        // В finally: исключение внутри оставило бы человека на форме с
        // вечным «Отправляем…» и заблокированной кнопкой — уже ПОСЛЕ
        // принятой анкеты.
        submitting = false;
      }
    };
  };

  function reachGoal() {
    try {
      const id = Number(page.data?.metrikaId ?? 0);
      const ym = (globalThis as unknown as { ym?: (...a: unknown[]) => void }).ym;
      if (id > 0 && typeof ym === 'function') ym(id, 'reachGoal', 'join_form_sent');
    } catch (e) {
      // Счётчик — не причина ломать отправку. Пустой YANDEX_METRIKA_ID (штатное
      // состояние DEV) сюда даже не доходит: id = 0, ym не определён.
      console.warn('join: reachGoal:', e);
    }
  }
</script>

<svelte:head>
  <title>{title}</title>
  <meta name="description" content={description} />
  <!-- Канонический адрес — apex: он стоит в объявлении Директа, и это же
       снимает дубль между vshage.app/join и afisha.vshage.app/join. -->
  <link rel="canonical" href={JOIN_CANONICAL} />
  <meta property="og:type" content="website" />
  <meta property="og:title" content={title} />
  <meta property="og:description" content={description} />
  <meta property="og:url" content={JOIN_CANONICAL} />
  <meta property="og:image" content={JOIN_OG_IMAGE} />
  <meta name="twitter:card" content="summary_large_image" />
</svelte:head>

<main>
  <header>
    <!-- Вордмарк латиницей и только латиницей: в Bowlby One кириллицы нет
         вовсе, и «ВШАГЕ» этим шрифтом дало бы пустоту. -->
    <div class="wordmark">VSHAGE</div>
    <h1>Закрытая сеть студентов Москвы</h1>
    <p class="lead">
      Вшаге — закрытая сеть студентов сильных вузов Москвы: МГУ, ВШЭ, МГИМО, РАНХиГС, МФТИ и
      других. Вход по анкете, каждую читаем руками. Если подходишь — напишем в телеграм.
    </p>
  </header>

  <!-- ym-hide-content: Вебвизор Метрики включён в раскладке сайта
       (+layout.svelte), и без этого класса он записал бы содержимое полей —
       имя, вуз, юзернейм. Настройка «записывать поля» живёт в кабинете
       счётчика и из кода не видна, поэтому маскируем в разметке. -->
  <form method="POST" use:enhance={handleSubmit} novalidate class="ym-hide-content">
    <!-- Ловушка для ботов: человек до неё не доберётся ни глазами, ни табом.
         Имя поля намеренно бессмысленное, а НЕ `website`/`url`/`company` —
         по таким именам его заполнит автозаполнение Safari из карточки
         контакта и менеджеры паролей, `autocomplete="off"` они для этого
         случая не соблюдают. Ложное срабатывание тут стоит оплаченного лида,
         поэтому сервер ещё и пишет сработку в лог целиком. -->
    <div class="hp" aria-hidden="true">
      <label for="hp_note">Не заполняйте это поле</label>
      <input id="hp_note" name="hp_note" type="text" tabindex="-1" autocomplete="off" />
    </div>

    {#each TRACKING_KEYS as key (key)}
      <input type="hidden" name={key} value={data.tracking?.[key] ?? ''} />
    {/each}
    <input type="hidden" name="referrer" value={String(seed().referrer ?? '') || (data.referrer ?? '')} />

    {#if form?.error && !duplicate}
      <p class="banner err">{form.error}</p>
    {/if}
    {#if duplicate}
      <p class="banner ok">{form?.error}</p>
    {/if}

    <label class="row">
      <span class="lbl">Имя</span>
      <input
        class="input"
        class:invalid={fieldErrors.name}
        name="name"
        autocomplete="name"
        maxlength="80"
        bind:value={name}
        placeholder="Иван Петров"
      />
      {#if fieldErrors.name}<span class="err">{fieldErrors.name}</span>{/if}
    </label>

    <label class="row">
      <span class="lbl">Вуз</span>
      <select class="input" class:invalid={fieldErrors.university} name="university" bind:value={university}>
        <option value="" disabled>Выбери из списка</option>
        {#each UNIVERSITIES as u (u.code)}
          <option value={u.code}>{u.label}</option>
        {/each}
      </select>
      {#if fieldErrors.university}<span class="err">{fieldErrors.university}</span>{/if}
    </label>

    {#if university === OTHER_UNIVERSITY}
      <label class="row">
        <span class="lbl">Какой вуз</span>
        <input
          class="input"
          class:invalid={fieldErrors.university_other}
          name="university_other"
          maxlength="80"
          bind:value={universityOther}
          placeholder="Например, МИСиС"
        />
        {#if fieldErrors.university_other}<span class="err">{fieldErrors.university_other}</span>{/if}
      </label>
    {/if}

    <label class="row">
      <span class="lbl">Курс</span>
      <select class="input" class:invalid={fieldErrors.course} name="course" bind:value={course}>
        <option value="" disabled>Выбери курс</option>
        {#each COURSES as c (c.code)}
          <option value={c.code}>{c.label}</option>
        {/each}
      </select>
      {#if fieldErrors.course}<span class="err">{fieldErrors.course}</span>{/if}
    </label>

    <label class="row">
      <span class="lbl">Чем занимаешься</span>
      <input
        class="input"
        class:invalid={fieldErrors.about}
        name="about"
        maxlength={ABOUT_MAX}
        bind:value={about}
        placeholder="Учусь на 3 курсе, делаю подкаст про урбанистику"
      />
      <span class="hint" class:warn={aboutLeft < 0}>Осталось {aboutLeft} знаков</span>
      {#if fieldErrors.about}<span class="err">{fieldErrors.about}</span>{/if}
    </label>

    <label class="row">
      <span class="lbl">Телеграм</span>
      <input
        class="input"
        class:invalid={fieldErrors.telegram || telegramLooksWrong}
        name="telegram"
        inputmode="text"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        bind:value={telegram}
        placeholder="@ivanov"
      />
      {#if fieldErrors.telegram}
        <span class="err">{fieldErrors.telegram}</span>
      {:else if telegramLooksWrong}
        <span class="err">Нужен юзернейм, например @ivanov — а не имя или ссылка на канал</span>
      {/if}
    </label>

    <label class="consent">
      <input type="checkbox" name="consent" bind:checked={consent} />
      <span>
        Согласен на обработку персональных данных в соответствии с
        <a href={PRIVACY_URL} target="_blank" rel="noopener">политикой конфиденциальности</a>.
      </span>
    </label>
    {#if fieldErrors.consent}<span class="err">{fieldErrors.consent}</span>{/if}

    <button class="btn" type="submit" disabled={(hydrated && !consent) || submitting}>
      {submitting ? 'Отправляем…' : 'Отправить'}
    </button>
    <!-- Подпись под неактивной кнопкой обязательна: кнопка, которая не
         нажимается и молчит, читается как сломанная страница. -->
    {#if hydrated && !consent}
      <p class="hint under">Поставь галочку согласия — тогда кнопка станет активной.</p>
    {/if}
  </form>

  <footer>
    <a href="https://afisha.vshage.app/">Афиша событий Москвы</a>
  </footer>
</main>

<style>
  main {
    max-width: 560px;
    margin: 0 auto;
    padding: var(--sp-6) var(--sp-4) var(--sp-10);
  }

  .wordmark {
    font-family: var(--font-display);
    font-size: 28px;
    letter-spacing: 1px;
    line-height: 1;
    margin-bottom: var(--sp-5);
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

  form {
    margin-top: var(--sp-8);
    display: flex;
    flex-direction: column;
    gap: var(--sp-5);
  }

  .row {
    display: block;
  }

  .lbl {
    display: block;
    font-size: 11px;
    letter-spacing: 1px;
    text-transform: uppercase;
    color: var(--mute);
    margin-bottom: var(--sp-2);
  }

  .input {
    width: 100%;
    /* 16px держит iOS Safari от зума вьюпорта на фокусе. */
    font-size: 16px;
    font-family: inherit;
    color: var(--fg);
    background: var(--bg-elev);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 14px var(--sp-3);
    min-height: 50px;
    appearance: none;
  }
  .input:focus {
    outline: none;
    border-color: var(--accent-green);
  }
  .input.invalid {
    border-color: var(--warning);
  }

  select.input {
    /* Стрелка своя: нативная у select в тёмной теме на iOS почти не видна. */
    background-image: linear-gradient(45deg, transparent 50%, var(--mute) 50%),
      linear-gradient(135deg, var(--mute) 50%, transparent 50%);
    background-position:
      calc(100% - 18px) 22px,
      calc(100% - 13px) 22px;
    background-size:
      5px 5px,
      5px 5px;
    background-repeat: no-repeat;
    padding-right: var(--sp-8);
  }

  .hint {
    display: block;
    font-size: 12px;
    margin-top: 6px;
    color: var(--mute);
  }
  .hint.warn {
    color: var(--warning);
  }
  .hint.under {
    margin-top: 0;
    text-align: center;
  }

  .err {
    display: block;
    font-size: 12px;
    margin-top: 6px;
    color: var(--warning);
  }

  .banner {
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: var(--sp-3);
    font-size: 14px;
    margin: 0;
  }
  .banner.err {
    border-color: var(--warning);
    color: var(--warning);
  }
  .banner.ok {
    color: var(--fg);
  }

  .consent {
    display: flex;
    gap: var(--sp-3);
    align-items: flex-start;
    font-size: 13px;
    line-height: 1.5;
    color: var(--mute);
  }
  .consent input {
    /* Крупная зона нажатия: галочка размером с системную на телефоне
       промахивается пальцем чаще, чем кажется. */
    width: 22px;
    height: 22px;
    flex: 0 0 22px;
    margin-top: 1px;
    accent-color: var(--fg);
  }
  .consent a {
    color: var(--fg);
    text-decoration: underline;
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    min-height: 52px;
    padding: 0 var(--sp-5);
    border: 1px solid var(--fg);
    border-radius: 12px;
    background: var(--fg);
    color: #000;
    font: inherit;
    font-size: 15px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 1px;
    cursor: pointer;
    transition: opacity var(--dur-fast) var(--ease-out);
  }
  .btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  /* Honeypot: невидим и не занимает места, но существует в DOM. display:none
     часть ботов пропускает — уводим за пределы экрана. */
  .hp {
    position: absolute;
    left: -9999px;
    width: 1px;
    height: 1px;
    overflow: hidden;
  }

  footer {
    margin-top: var(--sp-10);
    padding-top: var(--sp-5);
    border-top: 1px solid var(--border);
    font-size: 12px;
    color: var(--mute);
  }
  footer a {
    text-decoration: underline;
  }
</style>
