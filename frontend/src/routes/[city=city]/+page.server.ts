import type { PageServerLoad } from './$types';
import { loadListing } from './listing';

export const load: PageServerLoad = async ({ params, fetch, url, setHeaders }) => {
	// Раздела нет — это главная доска города: закреплённое показываем, фильтр
	// в запрос не кладём.
	const data = await loadListing({
		fetch,
		origin: url.origin,
		citySlug: params.city,
		section: null
	});
	setHeaders({ 'Cache-Control': 'public, max-age=30, stale-while-revalidate=300' });
	return data;
};
