import type { PageServerLoad } from './$types';
import { kindFromSearch, loadListing } from './listing';

export const load: PageServerLoad = async ({ params, fetch, url, setHeaders }) => {
	// Раздела нет — это главная доска города: закреплённое показываем, фильтр
	// в запрос не кладём. Полоса приезжает параметром, а не сегментом пути:
	// сегмент удвоил бы адреса доски и раздал бы роботу дубли.
	const data = await loadListing({
		fetch,
		origin: url.origin,
		citySlug: params.city,
		section: null,
		kind: kindFromSearch(url.searchParams)
	});
	setHeaders({ 'Cache-Control': 'public, max-age=30, stale-while-revalidate=300' });
	return data;
};
