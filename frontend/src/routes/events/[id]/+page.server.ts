import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// Страница события живёт по короткому адресу /<id>, а ссылка на запись — по
// /events/<id>/register. Человек, которому прислали ссылку на запись, легко
// обрезает её до /events/<id> — и до 03.09 получал 404. Постоянный редирект,
// чтобы и поисковик, и мессенджер знали канонический адрес.
export const load: PageServerLoad = async ({ params }) => {
	throw redirect(308, `/${params.id}`);
};
