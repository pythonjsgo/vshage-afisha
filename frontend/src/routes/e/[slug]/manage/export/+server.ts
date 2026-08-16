import { error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3004';

/**
 * CSV proxy for the organizer's export button.
 *
 * The page lives on vshage.app, where /api/* is NOT routed to afisha-backend
 * (only afisha.vshage.app carries that route). Proxying here keeps the whole
 * organizer flow on one origin: one link to share, no CORS, and the secret key
 * never has to travel to a second hostname.
 */
export const GET: RequestHandler = async ({ params, url, fetch }) => {
	const key = url.searchParams.get('key') ?? '';
	if (!key) throw error(404, 'Ссылка не подходит');

	const sep = url.searchParams.get('sep') === ',' ? '&sep=,' : '';

	let res: Response;
	try {
		res = await fetch(
			`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/manage.csv` +
				`?key=${encodeURIComponent(key)}${sep}`
		);
	} catch (e) {
		console.error(`webreg export ${params.slug}:`, e);
		throw error(503, 'Экспорт недоступен. Попробуй ещё раз.');
	}
	if (res.status === 404) throw error(404, 'Ссылка не подходит');
	if (!res.ok) throw error(503, 'Экспорт недоступен. Попробуй ещё раз.');

	return new Response(res.body, {
		headers: {
			'Content-Type': 'text/csv; charset=utf-8',
			'Content-Disposition':
				res.headers.get('content-disposition') ??
				`attachment; filename="${params.slug}-registrations.csv"`,
			'Cache-Control': 'no-store',
			'X-Robots-Tag': 'noindex, nofollow'
		}
	});
};
