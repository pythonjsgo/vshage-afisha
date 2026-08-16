import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import type { WebregManageList } from '$lib/webreg';

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3004';

export const load: PageServerLoad = async ({ params, url, fetch, setHeaders }) => {
	const key = url.searchParams.get('key') ?? '';
	// A missing key and a wrong key both read as 404 — a probe must not be
	// able to tell that this slug exists and merely needs the right secret.
	if (!key) throw error(404, 'Ссылка не подходит');

	let res: Response;
	try {
		res = await fetch(
			`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/manage?key=${encodeURIComponent(key)}`
		);
	} catch (e) {
		console.error(`webreg manage ${params.slug}:`, e);
		throw error(503, 'Список не загрузился. Обнови страницу.');
	}
	if (res.status === 404) throw error(404, 'Ссылка не подходит');
	if (!res.ok) throw error(503, 'Список не загрузился. Обнови страницу.');

	// The attendee list is personal data behind a secret link: never cached
	// by a proxy, never indexed.
	setHeaders({ 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex, nofollow' });

	return {
		list: (await res.json()) as WebregManageList,
		key
	};
};
