#!/usr/bin/env node
/**
 * Pulls a venue card out of the geo catalog so it can be pasted into an event
 * config.
 *
 *   node scripts/venue-card.mjs "точка кипения"
 *   node scripts/venue-card.mjs "точка кипения" --district Гагаринский
 *
 * The card is copied INTO the event config on purpose rather than resolved at
 * request time: the catalog is an 11MB file, and an event page that is about
 * to absorb a Telegram announcement must not depend on loading it — or on any
 * second service being up.
 *
 * Prints the top matches with an index, then the ready-to-paste "venue" block
 * for the best one (or for --pick N).
 */
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const CATALOG = join(here, '..', 'frontend', 'static', 'places-catalog.json');

const args = process.argv.slice(2);
const flag = (name) => {
	const i = args.indexOf(`--${name}`);
	return i >= 0 ? args[i + 1] : null;
};
const query = args.filter((a, i) => !a.startsWith('--') && !args[i - 1]?.startsWith('--')).join(' ');
const district = flag('district');
const pick = Number(flag('pick') ?? 0);

if (!query) {
	console.error('usage: node scripts/venue-card.mjs "<название места>" [--district X] [--pick N]');
	process.exit(1);
}

let raw;
try {
	raw = await readFile(CATALOG, 'utf-8');
} catch (e) {
	console.error(`Каталог мест не найден: ${CATALOG}`);
	console.error('Он лежит в ветке feat/places-catalog (frontend/static/places-catalog.json).');
	process.exit(1);
}

const { places = [] } = JSON.parse(raw);
const needle = query.toLowerCase().trim();

const scored = places
	.filter((p) => {
		if (district && !String(p.district ?? '').toLowerCase().includes(district.toLowerCase())) {
			return false;
		}
		return String(p.name ?? '').toLowerCase().includes(needle);
	})
	.map((p) => ({
		p,
		// Exact name first, then better-rated and better-reviewed places —
		// a search for "точка кипения" should not surface a namesake kiosk.
		score:
			(String(p.name).toLowerCase() === needle ? 1000 : 0) +
			(p.quality_score ?? 0) * 10 +
			Math.min(p.rating_count ?? 0, 500) / 500
	}))
	.sort((a, b) => b.score - a.score)
	.slice(0, 10);

if (!scored.length) {
	console.error(`Ничего не нашлось по «${query}»${district ? ` в районе ${district}` : ''}.`);
	console.error('Место можно не указывать — в конфиге хватит venue.address, страница это переживёт.');
	process.exit(2);
}

console.error(`Найдено ${scored.length} (показываю по убыванию похожести):\n`);
scored.forEach(({ p }, i) => {
	const rating = p.rating_avg ? `★${p.rating_avg.toFixed(1)} (${p.rating_count})` : 'без рейтинга';
	console.error(`  [${i}] ${p.name} — ${p.address ?? '?'} · ${p.district ?? '?'} · ${rating}`);
});

const chosen = scored[Math.min(Math.max(pick, 0), scored.length - 1)].p;
console.error(`\nБлок venue для [${pick}] «${chosen.name}» — вставь в конфиг события:\n`);

const tags = [];
if (chosen.outlets_score >= 0.6) tags.push('розетки');
if (chosen.talk_friendly >= 0.6) tags.push('можно поговорить');
if (chosen.laptop_friendly >= 0.6) tags.push('можно с ноутом');
// Catalog signal_tags are a mixed bag — some are internal English labels
// ("laptop", "wifi"). Only Cyrillic ones are fit to show a visitor.
for (const t of chosen.signal_tags ?? []) {
	if (tags.length >= 4) break;
	if (!/[а-яё]/i.test(String(t))) continue;
	if (!tags.includes(t)) tags.push(t);
}

if (!chosen.address) {
	console.error('⚠ У места в каталоге пустой адрес — впиши его в venue.address руками.\n');
}

console.log(
	JSON.stringify(
		{
			venue: {
				vid: chosen.id ?? '',
				name: chosen.name ?? '',
				address: chosen.address ?? '',
				district: chosen.district ?? '',
				lat: chosen.lat ?? 0,
				lon: chosen.lon ?? 0,
				rating_avg: chosen.rating_avg ?? null,
				rating_count: chosen.rating_count ?? 0,
				tags: tags.slice(0, 4),
				maps_url: chosen.maps_url ?? '',
				note: ''
			}
		},
		null,
		2
	)
);
