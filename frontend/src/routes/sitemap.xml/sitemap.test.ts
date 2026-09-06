import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { GET } from './+server';
import { ALL_SECTIONS } from '$lib/taxonomy';
import type { PublicEvent } from '$lib/types';

/**
 * Прибор на карту сайта.
 *
 * Полоса доски (`?kind=`) — это ПАРАМЕТР, а не сегмент пути, и в карту она не
 * попадает. Причина не косметическая: сегмент удвоил бы 21 адрес раздела до
 * 42 почти одинаковых страниц, и карта раздала бы роботу дубли собственными
 * руками. Отказ здесь немой в обе стороны — карта отвечает 200, XML валиден,
 * неправ только состав, и узнать об этом можно месяцами позже по выпавшим из
 * выдачи страницам.
 *
 * Файл лежит рядом с маршрутом и роутером не подхватывается: SvelteKit
 * считает эндпоинтом только `+server`.
 */

function qEvent(id: string, over: Partial<PublicEvent> = {}): PublicEvent {
  return {
    id,
    title: 'Событие',
    start_time: '2026-09-12T19:00:00+03:00',
    tags: [],
    attendee_count: 0,
    status: 'published',
    is_featured: false,
    ...over
  };
}

/** Подставной бэкенд карты: одна короткая страница выдачи и город из фасетов. */
function qBackend() {
  const calls: URL[] = [];
  const fetchFn = (async (input: string | URL) => {
    const u = new URL(String(input));
    calls.push(u);
    const body = u.pathname.endsWith('/facets')
      ? { cities: [{ slug: 'msk' }] }
      : { featured: [], all: [qEvent('ev_a'), qEvent('ev_b')], total: 2 };
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'content-type': 'application/json' }
    });
  }) as unknown as typeof globalThis.fetch;
  return { fetch: fetchFn, calls };
}

async function qSitemap() {
  const b = qBackend();
  const res = await GET({
    url: new URL('https://afisha.vshage.app/sitemap.xml'),
    fetch: b.fetch
    // Обработчику нужны только адрес и fetch; остальное поле RequestEvent он
    // не читает, и подставлять их значило бы описывать не тот прибор.
  } as unknown as Parameters<typeof GET>[0]);
  return { res, body: await res.text(), calls: b.calls };
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterEach(() => {
  vi.restoreAllMocks();
});

describe('sitemap.xml — полосы в карте нет', () => {
  it('ни один адрес не несёт ?kind=', async () => {
    const { body } = await qSitemap();
    expect(body).not.toContain('kind=');
    // Контроль: карта в этом же прогоне заведомо НЕ пуста и содержит те самые
    // адреса разделов, которые полоса и удвоила бы. Без него «нет kind=»
    // одинаково верно и для пустого документа, то есть прибор мерил бы
    // собственную поломку.
    expect(body).toContain('<loc>https://afisha.vshage.app/msk</loc>');
    for (const s of ALL_SECTIONS) {
      expect(body).toContain(`<loc>https://afisha.vshage.app/msk/${s.slug}</loc>`);
    }
    expect(body).toContain('<loc>https://afisha.vshage.app/ev_a</loc>');
  });

  it('бэкенд тоже не спрашивается по полосам — карта знает всю доску', async () => {
    const { calls } = await qSitemap();
    for (const u of calls) expect(u.searchParams.has('kind')).toBe(false);
  });

  it('полная карта отдаётся двухсоткой и кэшируется', async () => {
    const { res } = await qSitemap();
    expect(res.status).toBe(200);
    expect(res.headers.get('cache-control')).toContain('max-age=1800');
  });
});
