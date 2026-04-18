import type { ListResult, PublicEvent } from './types';

const BASE = import.meta.env.PUBLIC_API_URL ?? '/api';
// server-side uses internal URL via env on +page.server.ts, not this

export async function getEvents(fetch: typeof globalThis.fetch, base = BASE): Promise<ListResult> {
  const res = await fetch(`${base}/events`);
  if (!res.ok) throw new Error(`events list: ${res.status}`);
  return res.json();
}

export async function getEvent(id: string, fetch: typeof globalThis.fetch, base = BASE): Promise<PublicEvent | null> {
  const res = await fetch(`${base}/events/${encodeURIComponent(id)}`);
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`event get: ${res.status}`);
  return res.json();
}
