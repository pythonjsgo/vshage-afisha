import type { RequestHandler } from './$types';
import { ALL_SECTIONS } from '$lib/taxonomy';
import { eventUrl, listingUrl } from '$lib/seo';
import { CITY_SLUGS, eventsSearch } from '$lib/api';
import type { ListResult, PublicEvent } from '$lib/types';

/**
 * Карта сайта: главная, город, 21 раздел и КАЖДОЕ событие доски.
 *
 * Зачем она нужна при живом SSR. Робот доходит до карточки только по ссылке, а
 * с главной их видно ровно столько, сколько влезло в одну порцию выдачи, — то
 * есть с появлением «показать ещё» часть доски перестаёт быть достижимой
 * обходом вовсе. Карта — единственный список, который не зависит от того,
 * сколько карточек нарисовала страница.
 *
 * Собирается на лету, а не при сборке образа: доска меняется каждый день, а
 * образ живёт неделями. Отсюда и кэш ниже.
 */

/** Размер страницы выдачи: потолок maxPageSize у бэкенда. */
const PAGE_SIZE = 200;

/**
 * Потолок числа страниц НА ГОРОД. Существует не ради лимита спецификации
 * (50 000 URL — до него далеко), а против бэкенда, который перестал уважать
 * `offset`: без потолка такой ответ крутил бы цикл, держа запрос открытым.
 * Настоящий потолок НИЖЕ этого числа и задаётся не им: `eventsSearch`
 * подрезает limit под общее окно ленты (MAX_WINDOW = 1000), поэтому на
 * странице 5 запрос ушёл бы с offset=1000 и получил 400. То есть карта
 * знает не больше 1000 событий на город, и когда доска подойдёт к этому
 * числу, понадобится не большее MAX_PAGES, а окно поглубже на бэкенде —
 * либо sitemap index из нескольких файлов. Само MAX_PAGES остаётся
 * предохранителем от бэкенда, переставшего уважать offset.
 */
const MAX_PAGES = 25;

/** Слаг города из чужого ответа — в адрес пускаем только опознаваемое. */
const CITY_SLUG_RE = /^[a-z][a-z0-9-]{1,20}$/;

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';

/** Из фасетов нужен ТОЛЬКО список городов — остальное рисует страница. */
type FacetsResponse = { cities?: { slug?: string }[] };

/**
 * Экранирование обязательно, и обязательно целиком: невалидный символ делает
 * невалидным ВЕСЬ документ, а не одну строку — робот тогда не читает ни одной
 * ссылки. Амперсанд первым, иначе экранируем уже собственные сущности.
 */
function xml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

/**
 * Города для карты.
 *
 * Спрашиваем бэкенд (словарь городов принадлежит ему), но пересекаем с
 * CITY_SLUGS — списком, по которому матчер маршрута решает, существует ли
 * `/<город>`. Город, известный бэкенду и НЕ известный фронту, дал бы в карте
 * ссылку на 404: страница есть в списке, а маршрута под неё нет. Пересечение
 * снимает это по построению — и остаётся честным, потому что список фронта и
 * `internal/events/cities.go` пополняются одним заходом (см. api.ts).
 *
 * Эндпоинта может не быть вовсе (он приезжает этой же волной) — тогда берём
 * то, что фронт умеет рисовать.
 */
async function cities(fetchFn: typeof globalThis.fetch): Promise<string[]> {
  const routable = new Set(CITY_SLUGS);
  try {
    const res = await fetchFn(`${backendURL()}/api/events/facets`);
    if (!res.ok) return [...routable];
    const data = (await res.json()) as FacetsResponse;
    const list = (data.cities ?? [])
      .map((c) => (c?.slug ?? '').toLowerCase())
      .filter((s) => CITY_SLUG_RE.test(s) && routable.has(s));
    return list.length ? [...new Set(list)] : [...routable];
  } catch (e) {
    console.error('sitemap: фасеты недоступны, беру города фронта:', e);
    return [...routable];
  }
}

/**
 * События одного города страницами.
 *
 * Останавливаемся на первой короткой странице ИЛИ на первом отказе. Отказ тут
 * штатный: у бэкенда есть потолок окна (`offset+limit`) — сегодня на проде
 * 300, после этой волны 1000, — и запрос за его край честно отвечает 400.
 * Падать на этом нельзя: пятисотка на карте снимает с обхода ВСЕ ссылки
 * разом, включая те, что уже набраны. Неполная карта лучше отсутствующей.
 */
async function cityEvents(
  fetchFn: typeof globalThis.fetch,
  citySlug: string,
  seenIds: Set<string>
): Promise<{ events: PublicEvent[]; partial: boolean }> {
  const out: PublicEvent[] = [];

  // Неполнота — это состояние ответа, а не строчка в логе: логи прода в Loki
  // не доезжают, и обрезанная карта, отданная двухсоткой, неотличима от
  // «доска пуста». Роботу это стоит выпавших из индекса событий.
  let partial = false;
  let page = 0;
  for (; page < MAX_PAGES; page++) {
    const offset = page * PAGE_SIZE;
    // Параметры собирает eventsSearch: он же ставит city (город передаётся
    // всегда, даже когда он один) и подрезает limit по общему потолку окна.
    const qs = eventsSearch({ city: citySlug, limit: PAGE_SIZE, offset });
    let data: ListResult;
    try {
      const res = await fetchFn(`${backendURL()}/api/events?${qs}`);
      if (!res.ok) {
        // Строка обязана называть город и смещение: иначе «карта короче
        // доски» не с чем сопоставить.
        console.error(
          `sitemap: ${citySlug} offset=${offset} отвечает ${res.status}, останавливаюсь`
        );
        partial = true;
        break;
      }
      data = (await res.json()) as ListResult;
    } catch (e) {
      console.error(`sitemap: ${citySlug} offset=${offset} не получен:`, e);
      partial = true;
      break;
    }

    // featured склеиваем с all: закреплённое событие может не попасть в
    // общий список, а карта обязана знать про каждое.
    for (const ev of [...(data.featured ?? []), ...(data.all ?? [])]) {
      if (!ev?.id || seenIds.has(ev.id)) continue;
      seenIds.add(ev.id);
      out.push(ev);
    }
    // Короткая страница — конец ленты. Считаем по `all`: featured приезжает
    // в каждой странице заново и о длине выдачи ничего не говорит.
    if ((data.all?.length ?? 0) < PAGE_SIZE) {
      // Признак полноты приезжает в том же ответе и до сих пор не
      // использовался: набрали меньше, чем доска обещает, — значит карта
      // короче доски, даже если ни один запрос не упал (упёрлись в окно).
      if (typeof data.total === 'number' && seenIds.size < data.total) partial = true;
      break;
    }
  }
  if (page >= MAX_PAGES) partial = true;
  return { events: out, partial };
}

/** Only real content timestamps belong in lastmod; unknown is omitted. */
function lastmodOf(ev: PublicEvent): string | undefined {
  const raw = ev.updated_at;
  if (!raw || !/^\d{4}-\d{2}-\d{2}T/.test(raw)) return undefined;
  const date = new Date(raw);
  return Number.isFinite(date.getTime()) && date.getTime() <= Date.now() + 300000
    ? date.toISOString() : undefined;
}

export const GET: RequestHandler = async ({ url, fetch }) => {
  const origin = url.origin;
  const entries: { loc: string; lastmod?: string; image?: string }[] = [];
  const seenLoc = new Set<string>();
  const add = (loc: string, lastmod?: string, image?: string) => {
    if (seenLoc.has(loc)) return;
    seenLoc.add(loc);
    entries.push({ loc, lastmod, image });
  };

  // Главная. Она отдаёт 308 на город — робот редирект проходит и склеивает
  // адреса сам, а ссылки извне ведут именно на корень домена.
  // The root redirects; submit only canonical pages.

  const citySlugs = await cities(fetch);
  for (const city of citySlugs) {
    add(listingUrl(origin, city));
    for (const s of ALL_SECTIONS) add(listingUrl(origin, city, s.slug));
  }

  // Адрес карточки собирает eventUrl и только он: у события веб-регистрации
  // канонический адрес /e/<slug>, и карта обязана называть тот же, что
  // <link rel="canonical"> на самой странице.
  const seenIds = new Set<string>();
  let partial = false;
  for (const city of citySlugs) {
    const got = await cityEvents(fetch, city, seenIds);
    partial = partial || got.partial;
    for (const ev of got.events) {
      if (ev.indexable === false) continue;
      const image = ev.photo_url ? new URL(ev.photo_url, origin).href : undefined;
      add(eventUrl(origin, ev), lastmodOf(ev), image?.startsWith('http') ? image : undefined);
    }
  }

  const body =
    '<?xml version="1.0" encoding="UTF-8"?>\n' +
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">\n' +
    entries
      .map((e) => `  <url><loc>${xml(e.loc)}</loc>${e.lastmod ? `<lastmod>${e.lastmod}</lastmod>` : ''}${e.image ? `<image:image><image:loc>${xml(e.image)}</image:loc></image:image>` : ''}</url>`)
      .join('\n') +
    '\n</urlset>\n';

  return new Response(body, {
    // Неполная карта отдаётся с кодом 503 и без кэша. Двухсотка на обрезке —
    // это утверждение «вот вся доска», сказанное роботу: он выкинет из
    // индекса всё, чего в карте не оказалось, а разрешение кэшировать сутки
    // закрепило бы обрезок на сутки. 503 он читает как «приходи позже» и
    // сохраняет уже поданные адреса. Тело при этом остаётся полезным: то,
    // что набрали, — правда, просто не вся.
    status: partial ? 503 : 200,
    headers: {
      'Content-Type': 'application/xml; charset=utf-8',
      // Keep new automatic publications discoverable within minutes.
      'Cache-Control': partial
        ? 'no-store'
        : 'public, max-age=300, stale-while-revalidate=300'
    }
  });
};
