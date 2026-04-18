import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import type { PublicEvent } from '$lib/types';

export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
  const res = await fetch(`${backend}/api/events/${encodeURIComponent(params.id)}`);
  if (res.status === 404) throw error(404, 'Событие не найдено');
  if (!res.ok) throw error(500, 'Сервер недоступен');
  setHeaders({ 'Cache-Control': 'public, max-age=60, stale-while-revalidate=300' });
  const event = (await res.json()) as PublicEvent;
  return { event };
};
