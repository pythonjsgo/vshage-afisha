import type { PageServerLoad } from './$types';

/**
 * Витрина событий из телеграм-каналов (конвейер vshage-geo).
 * Юр. рамка: карточка несёт только НАШ текст (annonce) и ссылку на
 * первоисточник — чужих текстов и медиа здесь нет, поэтому и полей под
 * них нет.
 */
export type TgEvent = {
	id: string;
	title: string;
	annonce: string;
	date: string; // YYYY-MM-DD
	date_end?: string;
	time_start?: string;
	city?: string;
	place_name?: string;
	address?: string;
	online: boolean;
	price_raw?: string;
	is_free?: boolean;
	registration_url?: string;
	access_level: 'open' | 'university' | 'invite' | 'unknown';
	segment?: string;
	org_name?: string;
	source_url?: string;
	// Обложка с нашего origin (/api/tg-events/<id>/cover) — байты превью
	// поста, скачанные импортёром; прямые ссылки на CDN телеги протухают.
	cover_url?: string;
};

export type TgEventList = { events: TgEvent[]; count: number; degraded?: boolean };

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
	const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3004';
	const res = await fetch(`${backend}/api/tg-events`);
	if (!res.ok) {
		// «Пока пусто» — легитимное состояние витрины, поэтому отказ бэкенда
		// обязан выглядеть иначе: флаг degraded + строка в логе. Полный даун
		// (fetch reject) сюда не попадает — load бросит и страница отдаст 500.
		console.error(`uni: backend ответил ${res.status}`);
		return { events: [], count: 0, degraded: true } as TgEventList;
	}
	setHeaders({ 'Cache-Control': 'public, max-age=60, stale-while-revalidate=300' });
	return (await res.json()) as TgEventList;
};
