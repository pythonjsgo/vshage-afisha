import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { sectionBySlug } from '$lib/taxonomy';
import { loadListing } from '../listing';

export const load: PageServerLoad = async ({ params, fetch, url, setHeaders }) => {
	const section = sectionBySlug(params.section);
	// Матчер уже пропустил сюда только слаги из словаря — это второй забор на
	// случай, если матчер однажды разойдётся со словарём (например, словарь
	// пополнят, а матчер закешируется сборкой). Отдать в этом месте «раздел
	// без фильтра» значило бы завести страницу-дубль по несуществующему адресу.
	if (!section) error(404, 'Такого раздела нет');

	const data = await loadListing({
		fetch,
		origin: url.origin,
		citySlug: params.city,
		section
	});
	setHeaders({ 'Cache-Control': 'public, max-age=30, stale-while-revalidate=300' });
	return data;
};
