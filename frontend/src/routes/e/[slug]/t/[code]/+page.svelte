<script lang="ts">
	import { enhance } from '$app/forms';
	import { formatEventWhen, formatTime } from '$lib/webreg';

	let { data, form } = $props();

	const t = $derived(data.ticket);
	const when = $derived(formatEventWhen(t.starts_at, t.timezone));
	// The server value is the truth on load; the action's reply wins after a
	// successful scan so the door sees the change without a reload.
	const checkedInAt = $derived(form?.checkedIn ? new Date().toISOString() : t.checked_in_at);
	const isOrganizer = $derived(Boolean(data.manageKey));
</script>

<svelte:head>
	<title>Билет · {t.event_title}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<main class="wrap">
	<div class="kicker">Билет на вход</div>
	<h1>{t.event_title}</h1>
	<p class="when">
		{when}{t.venue_name ? ` · ${t.venue_name}` : ''}
	</p>

	<section class="card" class:used={checkedInAt}>
		<div class="qr">{@html data.qr}</div>
		<div class="code">{t.code}</div>
		<div class="holder">{t.full_name || t.name}</div>

		{#if checkedInAt}
			<div class="badge">Отмечен · {formatTime(checkedInAt, t.timezone)}</div>
		{/if}
	</section>

	{#if t.venue_address}
		<p class="addr">{t.venue_address}</p>
	{/if}

	{#if isOrganizer}
		<!-- Организаторский режим: открыт по ссылке с ключом, у гостя его нет. -->
		<section class="door">
			{#if checkedInAt}
				<p class="ok">Гость отмечен. Повторное сканирование время не перезапишет.</p>
			{:else}
				<form method="POST" action="?/checkin" use:enhance>
					<input type="hidden" name="key" value={data.manageKey} />
					<button class="btn btn-accent" type="submit">Отметить пришедшим</button>
				</form>
			{/if}
			{#if form?.checkinError}
				<p class="err">{form.checkinError}</p>
			{/if}
		</section>
	{:else}
		<p class="hint">Покажи этот экран на входе. Страницу можно сохранить или переслать себе.</p>
	{/if}

	<a class="back" href={`/e/${t.event_slug}`}>← к событию</a>
</main>

<style>
	.wrap {
		max-width: 460px;
		margin: 0 auto;
		padding: var(--sp-6) var(--sp-4) var(--sp-8);
		text-align: center;
	}
	.kicker {
		font-size: 11px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--accent-green);
	}
	h1 {
		font-family: var(--font-display);
		font-size: clamp(22px, 6vw, 28px);
		line-height: 1.1;
		margin-top: var(--sp-2);
	}
	.when {
		color: var(--mute);
		margin-top: var(--sp-2);
		font-size: 14px;
	}
	.card {
		margin-top: var(--sp-5);
		padding: var(--sp-5) var(--sp-4);
		border: 1px solid var(--border);
		border-radius: 16px;
		background: var(--bg-elev);
	}
	.card.used {
		opacity: 0.6;
	}
	/* The QR must stay dark-on-light regardless of theme: a phone camera at a
	   door reads an inverted code far less reliably, and some readers not at
	   all. So the plate is always white, whatever the page around it does. */
	.qr {
		width: min(260px, 70vw);
		margin: 0 auto;
		padding: var(--sp-3);
		background: #fff;
		border-radius: 12px;
	}
	.qr :global(svg) {
		display: block;
		width: 100%;
		height: auto;
	}
	.code {
		font-family: var(--font-mono, monospace);
		font-size: 28px;
		letter-spacing: 0.18em;
		margin-top: var(--sp-4);
	}
	.holder {
		margin-top: var(--sp-2);
		font-size: 15px;
		font-weight: 600;
	}
	.badge {
		display: inline-block;
		margin-top: var(--sp-4);
		padding: 6px 12px;
		border-radius: 999px;
		background: var(--accent-green);
		color: #000;
		font-size: 12px;
		font-weight: 700;
	}
	.addr {
		margin-top: var(--sp-4);
		color: var(--mute);
		font-size: 14px;
	}
	.door {
		margin-top: var(--sp-5);
	}
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 100%;
		min-height: 54px;
		padding: 0 var(--sp-5);
		border: 1px solid var(--border);
		border-radius: 12px;
		font: inherit;
		font-size: 16px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn-accent {
		background: var(--accent-green);
		border-color: var(--accent-green);
		color: #000;
	}
	.ok {
		color: var(--accent-green);
		font-size: 14px;
	}
	.err {
		color: var(--warning);
		font-size: 13px;
		margin-top: var(--sp-3);
	}
	.hint {
		margin-top: var(--sp-5);
		color: var(--mute);
		font-size: 13px;
		line-height: 1.5;
	}
	.back {
		display: inline-block;
		margin-top: var(--sp-6);
		color: var(--mute);
		font-size: 13px;
	}
</style>
