<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { formatShort, formatTime } from '$lib/webreg';

	let { data } = $props();

	const list = $derived(data.list);
	const when = $derived(formatShort(list.starts_at, list.timezone));
	const exportHref = $derived(
		`/e/${encodeURIComponent(list.slug)}/manage/export?key=${encodeURIComponent(data.key)}`
	);

	let refreshing = $state(false);
	let lastRefresh = $state<string | null>(null);
	let autoRefresh = $state(true);

	async function refresh() {
		refreshing = true;
		try {
			await invalidateAll();
			lastRefresh = new Date().toLocaleTimeString('ru-RU', {
				hour: '2-digit',
				minute: '2-digit',
				second: '2-digit'
			});
		} finally {
			refreshing = false;
		}
	}

	// Live list: poll while the tab is visible. Polling a hidden tab burns the
	// organizer's mobile data for a screen nobody is looking at.
	$effect(() => {
		if (!autoRefresh) return;
		const id = setInterval(() => {
			if (document.visibilityState === 'visible') refresh();
		}, 20000);
		return () => clearInterval(id);
	});
</script>

<svelte:head>
	<title>Регистрации · {list.title}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="wrap">
	<header>
		<div class="kicker">Регистрации</div>
		<h1>{list.title}</h1>
		<p class="when">{when}</p>
	</header>

	<section class="counter">
		<div class="big">{list.total}</div>
		<div class="counter-label">
			{list.capacity ? `из ${list.capacity} мест` : 'человек записались'}
		</div>
	</section>

	<div class="actions">
		<a class="btn btn-accent" href={exportHref} data-sveltekit-reload>Скачать CSV</a>
		<button class="btn btn-ghost" onclick={refresh} disabled={refreshing}>
			{refreshing ? 'Обновляем…' : 'Обновить'}
		</button>
		<label class="auto">
			<input type="checkbox" bind:checked={autoRefresh} />
			<span>каждые 20 сек</span>
		</label>
	</div>
	{#if lastRefresh}
		<p class="stamp">обновлено в {lastRefresh}</p>
	{/if}

	{#if list.total === 0}
		<p class="empty">Пока никто не записался. Список появится здесь сразу после первой заявки.</p>
	{:else}
		<ol class="rows">
			{#each list.registrations as reg, i (reg.id)}
				<li class="row">
					<span class="num">{i + 1}</span>
					<div class="who">
						<div class="name">{reg.name}</div>
						<a
							class="tg"
							href={`https://t.me/${reg.tg_username}`}
							target="_blank"
							rel="noopener">{reg.tg_display}</a
						>
						<div class="aff">{reg.affiliation}</div>
						{#each list.fields as f (f.key)}
							{#if reg.answers[f.key]}
								<div class="answer"><span>{f.label}:</span> {reg.answers[f.key]}</div>
							{/if}
						{/each}
					</div>
					<span class="time">{formatTime(reg.created_at, list.timezone)}</span>
				</li>
			{/each}
		</ol>
	{/if}

	<p class="note">
		Ссылка на эту страницу — секретная. У кого она есть, тот видит список. Не публикуй её в канале.
	</p>
</main>

<style>
	.wrap {
		max-width: 720px;
		margin: 0 auto;
		padding: var(--sp-5) var(--sp-4) var(--sp-10);
	}
	.kicker {
		font-size: 11px;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--accent-green);
	}
	h1 {
		font-family: var(--font-display);
		font-size: clamp(24px, 6vw, 32px);
		margin-top: var(--sp-2);
		line-height: 1.1;
	}
	.when {
		color: var(--mute);
		margin-top: var(--sp-2);
		font-size: 14px;
	}

	.counter {
		display: flex;
		align-items: baseline;
		gap: var(--sp-3);
		margin: var(--sp-5) 0 var(--sp-4);
		padding: var(--sp-4);
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 14px;
	}
	.big {
		font-family: var(--font-display);
		font-size: 44px;
		line-height: 1;
		color: var(--accent-green);
	}
	.counter-label {
		color: var(--mute);
		font-size: 14px;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		flex-wrap: wrap;
	}
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 46px;
		padding: 0 var(--sp-4);
		border: 1px solid var(--border);
		border-radius: 12px;
		font: inherit;
		font-size: 14px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.55;
	}
	.btn-accent {
		background: var(--accent-green);
		border-color: var(--accent-green);
		color: #000;
	}
	.btn-ghost {
		background: transparent;
		color: var(--fg);
	}
	.auto {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		font-size: 12px;
		color: var(--mute);
	}
	.auto input {
		width: 18px;
		height: 18px;
		accent-color: var(--accent-green);
	}
	.stamp {
		margin-top: var(--sp-2);
		font-size: 11px;
		color: var(--mute);
	}

	.empty {
		margin-top: var(--sp-6);
		color: var(--mute);
		line-height: 1.5;
	}

	.rows {
		list-style: none;
		margin-top: var(--sp-5);
		border-top: 1px solid var(--border);
	}
	.row {
		display: grid;
		grid-template-columns: 28px 1fr auto;
		gap: var(--sp-3);
		padding: var(--sp-4) 0;
		border-bottom: 1px solid var(--border);
	}
	.num {
		color: var(--mute);
		font-size: 12px;
		padding-top: 3px;
	}
	.name {
		font-size: 15px;
		font-weight: 600;
	}
	.tg {
		display: inline-block;
		color: var(--accent-green);
		font-size: 13px;
		margin-top: 2px;
	}
	.aff {
		color: var(--mute);
		font-size: 13px;
		margin-top: 2px;
	}
	.answer {
		font-size: 12px;
		color: var(--mute);
		margin-top: 4px;
	}
	.answer span {
		opacity: 0.7;
	}
	.time {
		color: var(--mute);
		font-size: 12px;
		white-space: nowrap;
		padding-top: 3px;
	}

	.note {
		margin-top: var(--sp-6);
		font-size: 12px;
		color: var(--mute);
		line-height: 1.5;
	}
</style>
