import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import type { PublicEvent } from '$lib/types';
import type { WebregEvent } from '$lib/webreg';

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
// Импортированное студсобытие: ev_ + хеш карточки (vshage-geo/collect/tg_event_cards.py).
const TG_ID_RE = /^ev_[0-9a-f]{6,}$/i;

/**
 * Maps a web-registration event onto the board's event shape so one detail
 * page renders both sources.
 *
 * registration_mode 'external' is what splits the two links the founder asked
 * for (17.08): this page is the showcase — cover, full description, share
 * preview — and its button hands the visitor over to /e/<slug>, the short
 * page built for filling a form on a phone.
 */
function fromWebreg(ev: WebregEvent): PublicEvent {
	return {
		id: ev.slug,
		webreg_slug: ev.slug,
		title: ev.title,
		short_description: ev.tagline,
		description: ev.description,
		start_time: ev.starts_at,
		end_time: ev.ends_at,
		tags: [],
		max_attendees: ev.capacity,
		attendee_count: ev.registered_count,
		photo_url: ev.cover_url,
		status: 'published',
		registration_mode: 'external',
		external_registration_url: `/e/${ev.slug}`,
		price_type: 'free',
		currency: 'RUB',
		venue_name: ev.venue?.name,
		address: ev.venue?.address,
		online_url: ev.venue?.online_url,
		is_featured: false,
		organizer_name: ev.organizer_title,
		photos: []
	};
}

export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
	const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';

	// Импортированное студсобытие: id вида ev_<хеш>, своя таблица, свой
	// эндпоинт. Проверка идёт ПЕРЕД веб-регистрацией — иначе ev_* уходил бы
	// в /api/e/<slug> как слаг и отвечал 404 (это и было третьим
	// препятствием, из-за которого 23.08 завели отдельную страницу /uni).
	// Бэкенд отдаёт здесь уже готовый PublicEvent, поэтому маппер не нужен.
	if (TG_ID_RE.test(params.id)) {
		const res = await fetch(`${backend}/api/tg-events/${encodeURIComponent(params.id)}`);
		if (res.status === 404) throw error(404, 'Событие не найдено');
		if (!res.ok) throw error(500, 'Сервер недоступен');
		const ev = (await res.json()) as PublicEvent;
		setHeaders({ 'Cache-Control': 'public, max-age=60, stale-while-revalidate=300' });
		return { event: ev };
	}

	// A slug that is not a UUID can only be a web-registration event; the
	// shared events table is keyed by UUID and would answer 404 for it.
	if (!UUID_RE.test(params.id)) {
		const res = await fetch(`${backend}/api/e/${encodeURIComponent(params.id)}`);
		if (res.status === 404) throw error(404, 'Событие не найдено');
		if (!res.ok) throw error(500, 'Сервер недоступен');
		const ev = (await res.json()) as WebregEvent;
		if (!ev.publish_afisha) throw error(404, 'Событие не найдено');
		setHeaders({ 'Cache-Control': 'public, max-age=60, stale-while-revalidate=300' });
		return { event: fromWebreg(ev) };
	}

	const res = await fetch(`${backend}/api/events/${encodeURIComponent(params.id)}`);
	if (res.status === 404) throw error(404, 'Событие не найдено');
	if (!res.ok) throw error(500, 'Сервер недоступен');
	setHeaders({ 'Cache-Control': 'public, max-age=60, stale-while-revalidate=300' });
	const event = (await res.json()) as PublicEvent;
	return { event };
};
