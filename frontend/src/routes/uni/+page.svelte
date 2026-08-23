<script lang="ts">
	import type { TgEvent } from './+page.server';
	import MetaPill from '$lib/components/MetaPill.svelte';

	let { data } = $props();

	const MONTHS = ['ЯНВ','ФЕВ','МАР','АПР','МАЯ','ИЮН','ИЮЛ','АВГ','СЕН','ОКТ','НОЯ','ДЕК'];

	// Даты приходят как YYYY-MM-DD без часового пояса — парсим руками,
	// new Date('2026-08-23') дал бы UTC-полночь и съехал бы на день в минусовых поясах.
	function parts(d: string): { day: number; mon: string } {
		const [, m, day] = d.split('-').map(Number);
		return { day, mon: MONTHS[(m ?? 1) - 1] ?? '' };
	}

	function dateLabel(ev: TgEvent): string {
		const a = parts(ev.date);
		if (ev.date_end && ev.date_end !== ev.date) {
			const b = parts(ev.date_end);
			return a.mon === b.mon
				? `${a.day}–${b.day} ${a.mon}`
				: `${a.day} ${a.mon} – ${b.day} ${b.mon}`;
		}
		return `${a.day} ${a.mon}`;
	}

	const ACCESS: Record<string, { text: string; variant?: 'warning' } | null> = {
		open: { text: 'Открытый вход' },
		university: { text: 'Для студентов' },
		invite: { text: 'По приглашению', variant: 'warning' },
		unknown: null
	};

	function placeLine(ev: TgEvent): string {
		if (ev.online) return 'Онлайн';
		return [ev.place_name, ev.address].filter(Boolean).join(' · ');
	}

	function sourceHost(url: string): string {
		try {
			return new URL(url).host.replace(/^www\./, '');
		} catch {
			return 'источник';
		}
	}

</script>

<svelte:head>
	<title>СТУДСОБЫТИЯ · АФИША ВШАГЕ</title>
	<meta
		name="description"
		content="Студенческие события Москвы: лекции, экскурсии, фестивали — собрано из открытых анонсов"
	/>
</svelte:head>

<header class="nav">
	<a href="/" class="logo">АФИША_ВШАГЕ</a>
	<span class="section-tag">/UNI</span>
</header>

<main>
	<section class="intro">
		<h1>СТУД<span>СОБЫТИЯ</span></h1>
		<p>
			Что происходит в вузах и вокруг них. Описания — наши, по открытым
			анонсам; у каждой карточки есть ссылка на первоисточник.
		</p>
	</section>

	{#if data.degraded}
		<p class="empty">Витрина временно недоступна — загляни чуть позже.</p>
	{:else if data.events.length === 0}
		<p class="empty">Пока пусто — новые события появляются после очередного сбора.</p>
	{:else}
		<div class="list">
			{#each data.events as ev (ev.id)}
				<article class="event" class:with-cover={!!ev.cover_url}>
					{#if ev.cover_url && ev.source_url}
						<a
							class="cover"
							href={ev.source_url}
							target="_blank"
							rel="noopener noreferrer"
							aria-label="Анонс-первоисточник"
						>
							<img src={ev.cover_url} alt="" loading="lazy" />
						</a>
					{:else if ev.cover_url}
						<div class="cover">
							<img src={ev.cover_url} alt="" loading="lazy" />
						</div>
					{/if}
					<div class="when">
						<span class="date">{dateLabel(ev)}</span>
						{#if ev.time_start}<span class="time">{ev.time_start}</span>{/if}
					</div>
					<div class="body">
						<div class="pills">
							{#if ev.is_free}
								<MetaPill text="Бесплатно" />
							{:else if ev.price_raw}
								<MetaPill text={ev.price_raw} />
							{/if}
							{#if ACCESS[ev.access_level]}
								<MetaPill
									text={ACCESS[ev.access_level]!.text}
									variant={ACCESS[ev.access_level]!.variant}
								/>
							{/if}
						</div>
						<h2>{ev.title}</h2>
						<p class="annonce">{ev.annonce}</p>
						{#if placeLine(ev)}
							<div class="place">{placeLine(ev)}</div>
						{/if}
						{#if ev.org_name}
							<div class="org">by {ev.org_name}</div>
						{/if}
						<div class="actions">
							{#if ev.registration_url}
								<a
									class="register"
									href={ev.registration_url}
									target="_blank"
									rel="noopener noreferrer">РЕГИСТРАЦИЯ ↗</a
								>
							{/if}
							{#if ev.source_url}
								<a
									class="source"
									href={ev.source_url}
									target="_blank"
									rel="noopener noreferrer">анонс: {sourceHost(ev.source_url)} ↗</a
								>
							{/if}
						</div>
					</div>
				</article>
			{/each}
		</div>
	{/if}
</main>

<footer>
	<p>ВШАГЕ · {new Date().getFullYear()}</p>
</footer>

<style>
	.nav {
		display: flex; justify-content: space-between; align-items: center;
		padding: var(--sp-3) var(--sp-4);
		border-bottom: 1px solid var(--border);
		position: sticky; top: 0; background: rgba(10,10,10,0.9);
		backdrop-filter: blur(8px); z-index: 10;
	}
	.logo { font-family: var(--font-display); font-size: 14px; color: var(--accent-pink); letter-spacing: 1px; }
	.section-tag { font-family: var(--font-mono); font-size: 11px; color: var(--accent-green); letter-spacing: 2px; }

	main {
		padding: var(--sp-4); display: flex; flex-direction: column;
		gap: var(--sp-5); max-width: 860px; margin: 0 auto;
	}

	.intro h1 {
		font-family: var(--font-display); font-weight: 400;
		font-size: clamp(40px, 9vw, 72px); line-height: 0.95;
		letter-spacing: -1px; margin: var(--sp-5) 0 var(--sp-3);
	}
	.intro h1 span { color: var(--accent-green); }
	.intro p { color: var(--mute); font-size: 13px; max-width: 52ch; line-height: 1.55; }

	.empty { color: var(--mute); padding: var(--sp-8) 0; text-align: center; }

	.list { display: flex; flex-direction: column; gap: var(--sp-4); }

	.event {
		display: grid;
		grid-template-areas: 'when body';
		grid-template-columns: 110px 1fr;
		gap: var(--sp-4);
		background: var(--bg-elev); border: 1px solid var(--border);
		padding: var(--sp-4);
		transition: border-color var(--dur-fast) var(--ease-out),
		            box-shadow var(--dur-fast) var(--ease-out);
	}
	.event.with-cover {
		grid-template-areas: 'when body cover';
		grid-template-columns: 110px 1fr 220px;
	}
	.event:hover {
		border-color: var(--accent-pink);
		box-shadow: 0 0 0 1px var(--accent-pink), 0 0 24px rgba(255, 0, 204, 0.18);
	}

	.cover {
		grid-area: cover;
		display: block;
		min-height: 180px;
		max-height: 300px;
		border: 1px solid var(--border);
		overflow: hidden;
	}
	.cover img {
		display: block;
		width: 100%; height: 100%;
		object-fit: cover;
	}

	.when { grid-area: when; display: flex; flex-direction: column; gap: var(--sp-1); }
	.date {
		font-family: var(--font-display); font-size: 20px; line-height: 1.05;
		color: var(--accent-green);
	}
	.time { font-family: var(--font-mono); font-size: 11px; color: var(--mute); }

	.body { grid-area: body; display: flex; flex-direction: column; gap: var(--sp-2); min-width: 0; }
	.pills { display: flex; gap: var(--sp-2); flex-wrap: wrap; }
	h2 {
		font-family: var(--font-display); font-weight: 400; font-size: 22px;
		line-height: 1.05; text-transform: uppercase; letter-spacing: -0.5px;
		text-wrap: balance;
	}
	.annonce { font-size: 13px; line-height: 1.55; color: var(--fg); max-width: 62ch; }
	.place { font-size: 11px; color: var(--mute); }
	.org { font-size: 10px; color: var(--mute); opacity: 0.6; letter-spacing: 0.5px; }

	.actions {
		display: flex; align-items: center; gap: var(--sp-4);
		margin-top: var(--sp-2); flex-wrap: wrap;
	}
	.register {
		background: var(--accent-green); color: #000; padding: 6px 12px;
		font-size: 11px; font-weight: 700; letter-spacing: 1px;
	}
	.register:hover { filter: brightness(1.15); }
	.source { font-size: 11px; color: var(--mute); text-decoration: underline; }
	.source:hover { color: var(--fg); }

	footer { text-align: center; padding: var(--sp-8) 0; color: var(--mute); font-size: 10px; letter-spacing: 2px; }

	@media (max-width: 720px) {
		.event {
			grid-template-areas: 'when' 'body';
			grid-template-columns: 1fr;
			gap: var(--sp-2);
		}
		.event.with-cover { grid-template-areas: 'cover' 'when' 'body'; }
		.cover { min-height: 0; height: 200px; }
		.when { flex-direction: row; align-items: baseline; gap: var(--sp-2); }
	}
</style>
