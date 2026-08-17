<script lang="ts">
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { page } from '$app/state';
	import {
		OTHER_OPTION,
		afishaEventURL,
		formatEventWhen,
		has2GIS,
		routeURL,
		type WebregField
	} from '$lib/webreg';

	let { data, form } = $props();

	const event = $derived(data.event);
	const venue = $derived(event.venue ?? {});
	const bridge = $derived(event.bridge ?? {});
	const cfg = $derived(event.form);
	const when = $derived(formatEventWhen(event.starts_at, event.timezone));
	const started = $derived(new Date(event.starts_at).getTime() < Date.now());
	const closed = $derived(!event.registration_open);

	const fieldErrors = $derived((form?.fieldErrors ?? {}) as Record<string, string>);
	const values = $derived(form?.values);

	// Venue block degrades: full card → address only → online → hidden.
	const hasVenueCard = $derived(Boolean(venue.name));
	const hasAddress = $derived(Boolean(venue.address));
	const isOnline = $derived(Boolean(venue.online_url) && !venue.name && !venue.address);
	const showVenue = $derived(hasVenueCard || hasAddress || isOnline);
	const mapsHref = $derived(routeURL(venue));
	const gisHref = $derived(has2GIS(venue));

	const affiliationOptions = $derived([...(event.affiliations ?? []), OTHER_OPTION]);

	// Ссылки на два других представления события. Регистрационная страница
	// намеренно короткая: обложка и полное описание живут в афише, сюда
	// человек приходит по ссылке из лички, чтобы записаться (директива 17.08).
	const afishaHref = $derived(
		event.publish_afisha ? afishaEventURL(data.afishaOrigin, event.slug) : null
	);
	const installURL = $derived(bridge.install_url || 'https://vshage.app/#beta');
	// «Открыть во Вшаге» без приложения означает «поставить Вшаге» — иначе
	// это кнопка в никуда. Явный vshage_url перебивает, когда он появится.
	const vshageHref = $derived(
		event.publish_vshage
			? bridge.vshage_url || bridge.testflight_url || bridge.app_store_url || installURL
			: null
	);

	// Описание сжимаем: первый абзац до 240 символов. Полный текст — в афише.
	const DESC_LIMIT = 240;
	const descFull = $derived((event.description ?? '').trim());
	const descShort = $derived.by(() => {
		const firstPara = descFull.split(/\n\s*\n/)[0] ?? '';
		if (firstPara.length <= DESC_LIMIT) return firstPara;
		const cut = firstPara.slice(0, DESC_LIMIT);
		const lastSpace = cut.lastIndexOf(' ');
		return (lastSpace > 120 ? cut.slice(0, lastSpace) : cut) + '…';
	});
	const descTruncated = $derived(descShort.length < descFull.length);

	// Every field is bound, never rendered as value={...}.
	//
	// This is not a style preference. A plain value={x} attribute is
	// re-applied on every re-render, so touching ANY other control (picking a
	// вуз from the dropdown) silently wiped the name and Telegram the visitor
	// had already typed — the submit then failed validation on fields that
	// looked filled in on screen. Caught by tests/e2e/webreg.spec.ts.
	let name = $state(form?.values?.name ?? '');
	let fullName = $state(form?.values?.full_name ?? '');
	let email = $state(form?.values?.email ?? '');
	let phone = $state(form?.values?.phone ?? '');
	let tgUsername = $state(form?.values?.tg_username ?? '');
	let affiliation = $state(form?.values?.affiliation ?? '');
	let affiliationOther = $state(form?.values?.affiliation_other ?? '');
	let answers = $state<Record<string, string>>({ ...(form?.values?.answers ?? {}) });
	let answersOther = $state<Record<string, string>>({});
	let consent = $state(form?.values?.consent ?? false);
	let submitting = $state(false);
	let waitlistSubmitting = $state(false);

	// Липкая кнопка «Иду» существует только пока форма НЕ на экране.
	// Иначе внизу страницы их видно две подряд — поймано фаундером на
	// скриншоте 17.08. Плюс она рисуется только при живом JS: без него
	// прятать её нечем, а дубль хуже, чем её отсутствие.
	let mounted = $state(false);
	let submitEl = $state<HTMLElement | null>(null);
	let submitOnScreen = $state(false);

	$effect(() => {
		mounted = true;
	});

	$effect(() => {
		if (!submitEl) return;
		const io = new IntersectionObserver(([entry]) => (submitOnScreen = entry.isIntersecting), {
			rootMargin: '0px 0px -80px 0px'
		});
		io.observe(submitEl);
		return () => io.disconnect();
	});

	// Restore what the visitor typed after a failed submit.
	$effect(() => {
		if (!values) return;
		name = values.name ?? '';
		fullName = values.full_name ?? '';
		email = values.email ?? '';
		phone = values.phone ?? '';
		tgUsername = values.tg_username ?? '';
		affiliation = values.affiliation ?? '';
		affiliationOther = values.affiliation_other ?? '';
		answers = { ...(values.answers ?? {}) };
		consent = values.consent ?? false;
	});

	// Обложка со страницы убрана, но в превью Телеграма она остаётся — там
	// картинка и решает, откроют ссылку или пролистают.
	const ogImage = $derived(
		event.cover_url && /^https?:\/\//.test(event.cover_url) ? event.cover_url : null
	);
	const shareDescription = $derived(
		event.tagline || `${when}${venue.name ? ` · ${venue.name}` : ''}`
	);

	const handleRegister: SubmitFunction = () => {
		submitting = true;
		return async ({ update }) => {
			await update({ reset: false });
			submitting = false;
		};
	};

	const handleWaitlist: SubmitFunction = () => {
		waitlistSubmitting = true;
		return async ({ update }) => {
			await update({ reset: false });
			waitlistSubmitting = false;
		};
	};

	function scrollToForm(e: MouseEvent) {
		e.preventDefault();
		const el = document.getElementById('reg');
		el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
		// Focus after the scroll settles; focusing immediately fights the
		// smooth scroll on iOS and jumps the viewport.
		setTimeout(() => document.querySelector<HTMLElement>('#reg .input')?.focus(), 400);
	}

	function optionsFor(f: WebregField): string[] {
		const base = f.options ?? [];
		return f.allow_other ? [...base, OTHER_OPTION] : base;
	}

	/** «Почта» / «Почта · необязательно» — метка несёт статус поля. */
	function labelFor(text: string, required: boolean): string {
		return required ? text : `${text} · необязательно`;
	}
</script>

<svelte:head>
	<title>{event.title} · Вшаге</title>
	<meta name="description" content={shareDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:title" content={event.title} />
	<meta property="og:description" content={shareDescription} />
	<meta property="og:url" content={page.url.href} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
		<meta name="twitter:card" content="summary_large_image" />
	{/if}
</svelte:head>

{#if data.done}
	<!-- ─── Готово ─────────────────────────────────────────────── -->
	<main class="wrap done">
		<div class="check">✓</div>
		<h1 class="done-title">
			{data.alreadyRegistered ? 'Ты уже в списке' : 'Ты в списке'}
		</h1>
		<p class="done-sub">
			{event.title}<br />
			{when}{venue.name ? ` · ${venue.name}` : ''}
		</p>
		{#if data.position > 0}
			<p class="position">{data.position}-й участник</p>
		{/if}

		{#if data.ticketCode}
			<section class="ticket-box">
				<div class="ticket-k">Билет на вход</div>
				<div class="ticket-code">{data.ticketCode}</div>
				<a class="btn btn-primary" href={`/e/${event.slug}/t/${data.ticketCode}`}>
					{event.ticket_mode === 'qr' ? 'Открыть билет с QR' : 'Открыть билет'}
				</a>
				<p class="ticket-hint">Покажи его на входе. Ссылку можно сохранить или переслать себе.</p>
			</section>
		{/if}

		{#if bridge.tg_chat_url || bridge.tg_channel_url}
			<a
				class="btn btn-ghost"
				href={bridge.tg_chat_url || bridge.tg_channel_url}
				target="_blank"
				rel="noopener"
			>
				{bridge.tg_chat_url ? 'Зайти в чат события' : 'Подписаться на канал'}
			</a>
		{/if}

		<!-- ─── Мост в сеть ─────────────────────────────────────── -->
		{#if data.platform === 'ios' && bridge.ios_mode !== 'off'}
			{@const iosHref =
				bridge.ios_mode === 'app_store' ? bridge.app_store_url : bridge.testflight_url}
			{#if iosHref}
				<section class="bridge">
					<h2>Вшаге — сеть тех, кто рядом</h2>
					<p>
						На событии приложение покажет, кто из участников сейчас в зале.
						{#if bridge.ios_mode === 'testflight'}
							Пока раздаём через TestFlight — это бета, ставится в два тапа.
						{/if}
					</p>
					<a class="btn btn-accent" href={iosHref} target="_blank" rel="noopener">
						{bridge.ios_mode === 'app_store' ? 'Установить приложение' : 'Поставить бету'}
					</a>
					{#if bridge.invite_code}
						<p class="invite">Код приглашения: <b>{bridge.invite_code}</b></p>
					{/if}
				</section>
			{/if}
		{:else if data.platform === 'android' && bridge.android_waitlist}
			<section class="bridge">
				<h2>Android-версия на подходе</h2>
				{#if form?.waitlisted}
					<p class="ok">Записали. Напишем в Телеграм, когда откроем.</p>
				{:else}
					<p>Оставь юзернейм — напишем первым, когда откроем доступ.</p>
					<form method="POST" action="?/waitlist" use:enhance={handleWaitlist}>
						<input type="hidden" name="platform" value="android" />
						<input
							class="input"
							type="text"
							name="tg_username"
							inputmode="text"
							autocapitalize="none"
							autocorrect="off"
							spellcheck="false"
							placeholder="@username"
							required
						/>
						<button class="btn btn-accent" type="submit" disabled={waitlistSubmitting}>
							{waitlistSubmitting ? 'Записываем…' : 'В лист ожидания'}
						</button>
					</form>
					{#if form?.waitlistError}
						<p class="err">{form.waitlistError}</p>
					{/if}
				{/if}
			</section>
		{/if}

		{#if afishaHref}
			<a class="back-link" href={afishaHref}>Событие в афише →</a>
		{/if}
		<a class="back" href={`/e/${event.slug}`}>← к событию</a>
	</main>
{:else}
	<!-- ─── Событие ────────────────────────────────────────────── -->
	<main class="wrap">
		<header class="hero">
			{#if event.organizer_title}
				<div class="kicker">{event.organizer_title}</div>
			{/if}
			<h1>{event.title}</h1>
			{#if event.tagline}
				<p class="tagline">{event.tagline}</p>
			{/if}
		</header>

		<section class="facts">
			<div class="fact">
				<span class="fact-k">Когда</span>
				<span class="fact-v">{when}</span>
			</div>
			{#if event.registered_count > 0}
				<div class="fact">
					<span class="fact-k">Идут</span>
					<span class="fact-v">{event.registered_count}</span>
				</div>
			{/if}
			{#if event.seats_left !== undefined && event.seats_left !== null}
				<div class="fact">
					<span class="fact-k">Мест</span>
					<span class="fact-v">{event.seats_left > 0 ? event.seats_left : 'нет'}</span>
				</div>
			{/if}
		</section>

		{#if showVenue}
			<section class="venue">
				{#if isOnline}
					<div class="venue-name">Онлайн</div>
					<a class="btn btn-ghost" href={venue.online_url} target="_blank" rel="noopener">
						Ссылка на трансляцию
					</a>
				{:else}
					{#if hasVenueCard}
						<div class="venue-name">{venue.name}</div>
					{/if}
					{#if hasAddress}
						<div class="venue-addr">{venue.address}</div>
					{/if}
					{#if venue.district || (venue.rating_avg && venue.rating_count)}
						<div class="venue-meta">
							{#if venue.district}<span>{venue.district}</span>{/if}
							{#if venue.rating_avg && venue.rating_count}
								<span>★ {venue.rating_avg.toFixed(1)} · {venue.rating_count}</span>
							{/if}
						</div>
					{/if}
					{#if venue.note}
						<div class="venue-note">{venue.note}</div>
					{/if}
					<div class="venue-actions">
						{#if mapsHref}
							<a class="btn btn-ghost" href={mapsHref} target="_blank" rel="noopener">
								Яндекс.Карты
							</a>
						{/if}
						{#if gisHref}
							<a class="btn btn-ghost" href={gisHref} target="_blank" rel="noopener">2ГИС</a>
						{/if}
					</div>
				{/if}
			</section>
		{/if}

		{#if descShort}
			<section class="desc">
				<p>{descShort}</p>
				{#if descTruncated}
					{#if afishaHref}
						<a class="more" href={afishaHref}>Полное описание в афише →</a>
					{:else}
						<details>
							<summary>Читать полностью</summary>
							<p class="desc-rest">{descFull}</p>
						</details>
					{/if}
				{/if}
			</section>
		{/if}

		<!-- ─── Форма ──────────────────────────────────────────── -->
		<section class="reg" id="reg">
			{#if closed || started}
				<div class="closed">
					{closed ? 'Регистрация закрыта.' : 'Событие уже началось.'}
				</div>
			{:else}
				<h2>Иду</h2>

				{#if form?.error}
					<p class="err banner">{form.error}</p>
				{/if}

				<form method="POST" action="?/register" use:enhance={handleRegister} novalidate>
					{#if cfg.name.enabled}
						<label class="field">
							<span class="label">{labelFor('Имя', cfg.name.required)}</span>
							<input
								class="input"
								class:invalid={fieldErrors.name}
								name="name"
								type="text"
								autocomplete="name"
								enterkeyhint="next"
								placeholder="Как тебя зовут"
								bind:value={name}
								required={cfg.name.required}
							/>
							{#if fieldErrors.name}<span class="err">{fieldErrors.name}</span>{/if}
						</label>
					{/if}

					{#if cfg.full_name.enabled}
						<label class="field">
							<span class="label">{labelFor('ФИО как в документе', cfg.full_name.required)}</span>
							<input
								class="input"
								class:invalid={fieldErrors.full_name}
								name="full_name"
								type="text"
								autocomplete="name"
								enterkeyhint="next"
								placeholder="Иванов Иван Иванович"
								bind:value={fullName}
								required={cfg.full_name.required}
							/>
							{#if fieldErrors.full_name}
								<span class="err">{fieldErrors.full_name}</span>
							{:else}
								<span class="hint">
									{cfg.pass_note || 'Нужно, чтобы выписать пропуск на входе'}
								</span>
							{/if}
						</label>
					{/if}

					{#if cfg.email.enabled}
						<label class="field">
							<span class="label">{labelFor('Почта', cfg.email.required)}</span>
							<input
								class="input"
								class:invalid={fieldErrors.email}
								name="email"
								type="email"
								inputmode="email"
								autocomplete="email"
								autocapitalize="none"
								autocorrect="off"
								spellcheck="false"
								enterkeyhint="next"
								placeholder="you@example.com"
								bind:value={email}
								required={cfg.email.required}
							/>
							{#if fieldErrors.email}
								<span class="err">{fieldErrors.email}</span>
							{:else}
								<span class="hint">На неё пришлём билет и напоминание перед событием</span>
							{/if}
						</label>
					{/if}

					{#if cfg.phone.enabled}
						<label class="field">
							<span class="label">{labelFor('Телефон', cfg.phone.required)}</span>
							<input
								class="input"
								class:invalid={fieldErrors.phone}
								name="phone"
								type="tel"
								inputmode="tel"
								autocomplete="tel"
								enterkeyhint="next"
								placeholder="+7 903 123-45-67"
								bind:value={phone}
								required={cfg.phone.required}
							/>
							{#if fieldErrors.phone}<span class="err">{fieldErrors.phone}</span>{/if}
						</label>
					{/if}

					{#if cfg.tg.enabled}
						<label class="field">
							<span class="label">{labelFor('Телеграм', cfg.tg.required)}</span>
							<input
								class="input"
								class:invalid={fieldErrors.tg_username}
								name="tg_username"
								type="text"
								inputmode="text"
								autocapitalize="none"
								autocorrect="off"
								spellcheck="false"
								enterkeyhint="next"
								placeholder="@username"
								bind:value={tgUsername}
								required={cfg.tg.required}
							/>
							{#if fieldErrors.tg_username}
								<span class="err">{fieldErrors.tg_username}</span>
							{:else}
								<span class="hint">По нему организатор добавит тебя в чат</span>
							{/if}
						</label>
					{/if}

					{#if cfg.affiliation.enabled}
						<label class="field">
							<span class="label">{labelFor('Вуз или статус', cfg.affiliation.required)}</span>
							<select
								class="input"
								class:invalid={fieldErrors.affiliation}
								name="affiliation"
								bind:value={affiliation}
								required={cfg.affiliation.required}
							>
								<option value="" disabled selected={!affiliation}>Выбери из списка</option>
								{#each affiliationOptions as opt}
									<option value={opt}>{opt}</option>
								{/each}
							</select>
							{#if fieldErrors.affiliation}<span class="err">{fieldErrors.affiliation}</span>{/if}
						</label>

						{#if affiliation === OTHER_OPTION}
							<label class="field">
								<span class="label">Что именно</span>
								<input
									class="input"
									name="affiliation_other"
									type="text"
									placeholder="Вуз, компания или статус"
									bind:value={affiliationOther}
									required
								/>
							</label>
						{/if}
					{/if}

					{#each event.fields ?? [] as f (f.key)}
						<div class="field">
							<span class="label">{labelFor(f.label, Boolean(f.required))}</span>
							{#if f.type === 'select'}
								<select
									class="input"
									class:invalid={fieldErrors[f.key]}
									name={`answer:${f.key}`}
									bind:value={answers[f.key]}
									required={f.required}
								>
									<option value="" disabled selected={!answers[f.key]}>Выбери из списка</option>
									{#each optionsFor(f) as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select>
								{#if answers[f.key] === OTHER_OPTION}
									<input
										class="input other"
										name={`answer_other:${f.key}`}
										type="text"
										placeholder="Свой вариант"
										bind:value={answersOther[f.key]}
										required
									/>
								{/if}
							{:else if f.type === 'textarea'}
								<textarea
									class="input"
									class:invalid={fieldErrors[f.key]}
									name={`answer:${f.key}`}
									rows="3"
									maxlength={f.max_len || 500}
									bind:value={answers[f.key]}
									required={f.required}
								></textarea>
							{:else if f.type === 'checkbox'}
								<label class="check-row">
									<input type="checkbox" name={`answer:${f.key}`} value="Да" />
									<span>{f.hint || 'Да'}</span>
								</label>
							{:else}
								<input
									class="input"
									class:invalid={fieldErrors[f.key]}
									name={`answer:${f.key}`}
									type="text"
									maxlength={f.max_len || 500}
									bind:value={answers[f.key]}
									required={f.required}
								/>
							{/if}
							{#if fieldErrors[f.key]}
								<span class="err">{fieldErrors[f.key]}</span>
							{:else if f.hint && f.type !== 'checkbox'}
								<span class="hint">{f.hint}</span>
							{/if}
						</div>
					{/each}

					<label class="check-row consent" class:invalid={fieldErrors.consent}>
						<input type="checkbox" name="consent" required bind:checked={consent} />
						<span>
							Согласен(на) на обработку персональных данных
							<a
								href={bridge.privacy_url || 'https://vshage.app/privacy/'}
								target="_blank"
								rel="noopener">Политика</a
							>
						</span>
					</label>
					{#if fieldErrors.consent}<span class="err">{fieldErrors.consent}</span>{/if}

					<input type="hidden" name="source" value={page.url.searchParams.get('from') ?? ''} />

					<button
						class="btn btn-accent submit"
						type="submit"
						disabled={submitting}
						bind:this={submitEl}
					>
						{submitting ? 'Записываем…' : 'Иду'}
					</button>
				</form>
			{/if}
		</section>

		<!-- ─── Где ещё есть это событие ───────────────────────── -->
		<section class="app-cta">
			<h2>Вшаге — сеть тех, кто рядом</h2>
			<p>На событии приложение покажет, кто из участников сейчас в зале.</p>
			<div class="cta-row">
				<a class="btn btn-ghost" href={installURL} target="_blank" rel="noopener">
					Скачать приложение
				</a>
				{#if vshageHref}
					<a class="btn btn-ghost" href={vshageHref} target="_blank" rel="noopener">
						Открыть во Вшаге
					</a>
				{/if}
				{#if afishaHref}
					<a class="btn btn-ghost" href={afishaHref}>Открыть в афише</a>
				{/if}
			</div>
		</section>

		{#if mounted && !closed && !started && !submitOnScreen}
			<div class="sticky">
				<a class="btn btn-accent" href="#reg" onclick={scrollToForm}>Иду</a>
			</div>
		{/if}
	</main>
{/if}

<style>
	.wrap {
		max-width: 560px;
		margin: 0 auto;
		padding: 0 var(--sp-4) calc(88px + env(safe-area-inset-bottom));
	}

	/* ─── Hero ─────────────────────────────────────────────── */
	.hero {
		padding-top: var(--sp-5);
	}
	.kicker {
		font-size: 11px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--accent-green);
		margin-bottom: var(--sp-2);
	}
	h1 {
		font-family: var(--font-display);
		font-size: clamp(28px, 8vw, 40px);
		line-height: 1.05;
		letter-spacing: -0.01em;
	}
	.tagline {
		color: var(--mute);
		margin-top: var(--sp-3);
		font-size: 15px;
		line-height: 1.45;
	}

	/* ─── Facts ────────────────────────────────────────────── */
	.facts {
		display: flex;
		gap: var(--sp-5);
		flex-wrap: wrap;
		margin: var(--sp-5) 0;
		padding: var(--sp-4) 0;
		border-top: 1px solid var(--border);
		border-bottom: 1px solid var(--border);
	}
	.fact {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.fact-k {
		font-size: 10px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--mute);
	}
	.fact-v {
		font-size: 15px;
		font-weight: 600;
	}

	/* ─── Venue ────────────────────────────────────────────── */
	.venue {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 14px;
		padding: var(--sp-4);
		margin-bottom: var(--sp-5);
	}
	.venue-name {
		font-size: 17px;
		font-weight: 700;
		margin-bottom: 2px;
	}
	.venue-addr {
		color: var(--mute);
		font-size: 14px;
	}
	.venue-meta {
		display: flex;
		gap: var(--sp-3);
		color: var(--mute);
		font-size: 12px;
		margin-top: var(--sp-2);
	}
	.venue-note {
		margin-top: var(--sp-3);
		font-size: 13px;
		color: var(--fg);
	}
	.venue-actions {
		display: flex;
		gap: var(--sp-2);
		margin-top: var(--sp-4);
	}

	.desc {
		line-height: 1.6;
		font-size: 15px;
		margin-bottom: var(--sp-6);
	}
	.desc p {
		white-space: pre-wrap;
	}
	.desc .more,
	.desc summary {
		display: inline-block;
		margin-top: var(--sp-3);
		font-size: 14px;
		color: var(--accent-green);
		cursor: pointer;
	}
	.desc-rest {
		margin-top: var(--sp-3);
		color: var(--mute);
	}

	/* ─── Form ─────────────────────────────────────────────── */
	.reg h2 {
		font-family: var(--font-display);
		font-size: 24px;
		margin-bottom: var(--sp-4);
	}
	.field {
		display: block;
		margin-bottom: var(--sp-4);
	}
	.label {
		display: block;
		font-size: 11px;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--mute);
		margin-bottom: var(--sp-2);
	}
	.input {
		width: 100%;
		/* 16px keeps iOS Safari from zooming the viewport on focus — at 15px
		   it zooms, and the visitor has to pinch back out mid-form. */
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
	.input.other {
		margin-top: var(--sp-2);
	}
	select.input {
		background-image: linear-gradient(45deg, transparent 50%, var(--mute) 50%),
			linear-gradient(135deg, var(--mute) 50%, transparent 50%);
		background-position:
			calc(100% - 18px) calc(50% + 2px),
			calc(100% - 13px) calc(50% + 2px);
		background-size: 5px 5px;
		background-repeat: no-repeat;
		padding-right: 36px;
	}
	.hint,
	.err {
		display: block;
		font-size: 12px;
		margin-top: 6px;
		line-height: 1.35;
	}
	.hint {
		color: var(--mute);
	}
	.err {
		color: var(--warning);
	}
	.err.banner {
		background: rgba(255, 45, 0, 0.1);
		border: 1px solid var(--warning);
		border-radius: 10px;
		padding: var(--sp-3);
		margin-bottom: var(--sp-4);
		font-size: 14px;
	}
	.check-row {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		font-size: 13px;
		line-height: 1.4;
		color: var(--mute);
	}
	.check-row input[type='checkbox'] {
		width: 22px;
		height: 22px;
		flex: 0 0 22px;
		accent-color: var(--accent-green);
		margin-top: 1px;
	}
	.check-row a {
		text-decoration: underline;
		color: var(--fg);
	}
	.consent {
		margin: var(--sp-5) 0 var(--sp-4);
	}
	.closed {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: var(--sp-4);
		text-align: center;
		color: var(--mute);
	}

	/* ─── Buttons ──────────────────────────────────────────── */
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 50px;
		padding: 0 var(--sp-5);
		border: 1px solid var(--border);
		border-radius: 12px;
		font: inherit;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
		text-align: center;
		transition: opacity var(--dur-fast) var(--ease-out);
	}
	.btn:disabled {
		opacity: 0.55;
	}
	.btn-accent {
		background: var(--accent-green);
		border-color: var(--accent-green);
		color: #000;
	}
	.btn-primary {
		background: var(--fg);
		border-color: var(--fg);
		color: #000;
	}
	.btn-ghost {
		background: transparent;
		color: var(--fg);
	}
	.submit {
		width: 100%;
		min-height: 56px;
		font-size: 17px;
	}

	/* ─── Приложение + другие витрины события ───────────────── */
	.app-cta {
		margin-top: var(--sp-8);
		padding-top: var(--sp-5);
		border-top: 1px solid var(--border);
	}
	.app-cta h2 {
		font-family: var(--font-display);
		font-size: 18px;
		margin-bottom: var(--sp-2);
	}
	.app-cta p {
		color: var(--mute);
		font-size: 14px;
		line-height: 1.5;
		margin-bottom: var(--sp-4);
	}
	.cta-row {
		display: flex;
		flex-direction: column;
		gap: var(--sp-2);
	}
	.cta-row .btn {
		width: 100%;
	}

	/* ─── Sticky CTA ───────────────────────────────────────── */
	.sticky {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		padding: var(--sp-3) var(--sp-4) calc(var(--sp-3) + env(safe-area-inset-bottom));
		background: linear-gradient(to top, var(--bg) 60%, transparent);
		display: flex;
		justify-content: center;
	}
	.sticky .btn {
		width: 100%;
		max-width: 528px;
	}

	/* ─── Done ─────────────────────────────────────────────── */
	.done {
		text-align: center;
		padding-top: var(--sp-10);
	}
	.check {
		width: 64px;
		height: 64px;
		margin: 0 auto var(--sp-5);
		border-radius: 50%;
		background: var(--accent-green);
		color: #000;
		font-size: 34px;
		line-height: 64px;
		font-weight: 700;
	}
	.done-title {
		font-family: var(--font-display);
		font-size: clamp(26px, 7vw, 34px);
	}
	.done-sub {
		color: var(--mute);
		margin-top: var(--sp-3);
		line-height: 1.5;
	}
	.position {
		margin-top: var(--sp-3);
		color: var(--accent-green);
		font-weight: 600;
	}
	.done .btn {
		margin-top: var(--sp-5);
		width: 100%;
	}
	.ticket-box {
		margin-top: var(--sp-6);
		padding: var(--sp-4);
		border: 1px solid var(--accent-green);
		border-radius: 14px;
		background: var(--bg-elev);
	}
	.ticket-k {
		font-size: 10px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--mute);
	}
	.ticket-code {
		font-family: var(--font-mono, monospace);
		font-size: 30px;
		letter-spacing: 0.16em;
		margin: var(--sp-2) 0 0;
	}
	.ticket-hint {
		margin-top: var(--sp-3);
		font-size: 12px;
		color: var(--mute);
		line-height: 1.4;
	}
	.bridge {
		margin-top: var(--sp-8);
		padding-top: var(--sp-5);
		border-top: 1px solid var(--border);
		text-align: left;
	}
	.bridge h2 {
		font-family: var(--font-display);
		font-size: 20px;
		margin-bottom: var(--sp-3);
	}
	.bridge p {
		color: var(--mute);
		font-size: 14px;
		line-height: 1.5;
	}
	.bridge form {
		display: flex;
		flex-direction: column;
		gap: var(--sp-3);
		margin-top: var(--sp-4);
	}
	.bridge .ok {
		color: var(--accent-green);
	}
	.invite {
		margin-top: var(--sp-3);
		font-size: 13px;
	}
	.back-link {
		display: block;
		margin-top: var(--sp-6);
		color: var(--accent-green);
		font-size: 14px;
	}
	.back {
		display: inline-block;
		margin-top: var(--sp-4);
		color: var(--mute);
		font-size: 13px;
	}
</style>
