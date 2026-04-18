import type { PageServerLoad } from './$types';
import type { ListResult } from '$lib/types';

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
  const res = await fetch(`${backend}/api/events`);
  if (!res.ok) {
    return { featured: [], all: [], total: 0 } as ListResult;
  }
  setHeaders({ 'Cache-Control': 'public, max-age=30, stale-while-revalidate=300' });
  return (await res.json()) as ListResult;
};
