<script lang="ts">
	import type { PlaceCard } from './+page.server';
	let { data } = $props();

	let q = $state('');
	let tag = $state('');

	const kindRu: Record<string, string> = {
		cafe: 'кафе',
		restaurant: 'ресторан',
		coworking: 'коворкинг',
		library: 'библиотека',
		anticafe: 'антикафе',
		other: 'место'
	};

	const filtered = $derived.by(() => {
		const qq = q.trim().toLowerCase();
		return (data.places as PlaceCard[]).filter((p) => {
			if (tag && !(p.signal_tags || []).includes(tag)) return false;
			if (!qq) return true;
			const hay = `${p.name} ${p.district} ${p.address || ''} ${p.blurb || ''} ${(p.signal_tags || []).join(' ')}`.toLowerCase();
			return hay.includes(qq);
		});
	});

	function tagLabel(t: string) {
		const m: Record<string, string> = {
			laptop: 'ноутбук',
			outlets: 'розетки',
			quiet_talk: 'тихо / разговор',
			view: 'вид',
			date: 'свидание',
			coffee: 'кофе',
			coworking: 'коворк'
		};
		return m[t] || t;
	}
</script>

<svelte:head>
	<title>Места · ВШАГЕ</title>
	<meta
		name="description"
		content="Каталог мест для встреч Вшаге — кофе, ноутбук, розетки, свидания. Финальная подборка."
	/>
	<meta name="robots" content="noindex" />
</svelte:head>

<header class="nav">
	<a href="/" class="logo">АФИША_ВШАГЕ</a>
	<span class="badge">МЕСТА · {data.total}</span>
</header>

<main>
	<section class="hero">
		<h1>Места для встреч</h1>
		<p class="sub">
			Финальная подборка (quality-gate по отзывам). Умный векторный поиск — в API агента;
			здесь — весь опубликованный каталог.
		</p>
		<div class="controls">
			<input type="search" placeholder="фильтр: кофе, ноутбук, район…" bind:value={q} />
			<select bind:value={tag}>
				<option value="">все теги</option>
				<option value="coffee">кофе</option>
				<option value="laptop">ноутбук</option>
				<option value="outlets">розетки</option>
				<option value="view">вид</option>
				<option value="date">свидание</option>
				<option value="quiet_talk">тихо / разговор</option>
				<option value="coworking">коворкинг</option>
			</select>
		</div>
		<p class="count">показано {filtered.length} / {data.total}</p>
	</section>

	{#if filtered.length === 0}
		<p class="empty">Пока пусто — каталог ещё не выгружен.</p>
	{:else}
		<ul class="grid">
			{#each filtered as p (p.id)}
				<li class="card">
					<div class="top">
						<strong class="name">{p.name}</strong>
						<span class="kind">{kindRu[p.kind] || p.kind}</span>
					</div>
					<div class="meta">
						<span>{p.district}</span>
						{#if p.rating_avg != null && p.rating_count > 0}
							<span>★ {Number(p.rating_avg).toFixed(1)} · {p.rating_count}</span>
						{/if}
					</div>
					{#if p.address}
						<p class="addr">{p.address}</p>
					{/if}
					<div class="scores">
						<span title="talk">разговор {(p.talk_friendly * 100) | 0}%</span>
						<span title="laptop">ноут {(p.laptop_friendly * 100) | 0}%</span>
						<span title="outlets">розетки {(p.outlets_score * 100) | 0}%</span>
					</div>
					{#if p.signal_tags?.length}
						<div class="tags">
							{#each p.signal_tags as t}
								<span class="tag">{tagLabel(t)}</span>
							{/each}
						</div>
					{/if}
					{#if p.blurb}
						<p class="blurb">{p.blurb}</p>
					{/if}
					{#if p.maps_url}
						<a class="map" href={p.maps_url} target="_blank" rel="noopener">на карте →</a>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</main>

<footer>
	<p>ВШАГЕ · места · только по URL /places (не на главной)</p>
</footer>

<style>
	.nav {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--sp-3) var(--sp-4);
		border-bottom: 1px solid var(--border);
		position: sticky;
		top: 0;
		background: rgba(10, 10, 10, 0.92);
		backdrop-filter: blur(8px);
		z-index: 10;
	}
	.logo {
		font-family: var(--font-display);
		font-size: 14px;
		color: var(--accent-pink);
		letter-spacing: 1px;
	}
	.badge {
		font-size: 10px;
		letter-spacing: 1px;
		color: var(--accent-green);
		border: 1px solid var(--accent-green);
		padding: 2px 8px;
	}
	main {
		padding: var(--sp-4);
		max-width: 1200px;
		margin: 0 auto;
	}
	.hero h1 {
		font-family: var(--font-display);
		font-size: 28px;
		margin: 0 0 8px;
		color: var(--text);
	}
	.sub {
		color: var(--mute);
		font-size: 13px;
		max-width: 52ch;
		line-height: 1.45;
	}
	.controls {
		display: flex;
		gap: 8px;
		margin: 16px 0 8px;
		flex-wrap: wrap;
	}
	input,
	select {
		background: #141414;
		border: 1px solid var(--border);
		color: var(--text);
		padding: 8px 12px;
		font-size: 13px;
		min-width: 200px;
	}
	.count {
		font-size: 11px;
		color: var(--mute);
		letter-spacing: 1px;
	}
	.grid {
		list-style: none;
		padding: 0;
		margin: 20px 0 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 12px;
	}
	.card {
		border: 1px solid var(--border);
		background: #111;
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.top {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		align-items: flex-start;
	}
	.name {
		font-size: 15px;
		line-height: 1.25;
	}
	.kind {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 1px;
		color: var(--accent-pink);
		white-space: nowrap;
	}
	.meta {
		display: flex;
		gap: 10px;
		font-size: 11px;
		color: var(--mute);
	}
	.addr {
		font-size: 12px;
		color: #bbb;
		margin: 0;
	}
	.scores {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		font-size: 10px;
		color: var(--accent-green);
	}
	.tags {
		display: flex;
		flex-wrap: wrap;
		gap: 4px;
	}
	.tag {
		font-size: 10px;
		border: 1px solid #333;
		padding: 1px 6px;
		color: #ccc;
	}
	.blurb {
		font-size: 11px;
		color: #999;
		line-height: 1.35;
		margin: 4px 0 0;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.map {
		margin-top: auto;
		font-size: 11px;
		color: var(--accent-pink);
		padding-top: 6px;
	}
	.empty {
		color: var(--mute);
		margin-top: 40px;
	}
	footer {
		text-align: center;
		padding: var(--sp-8) 0;
		color: var(--mute);
		font-size: 10px;
		letter-spacing: 1px;
	}
</style>
