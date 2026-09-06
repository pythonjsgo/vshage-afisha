import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { kindFromSearch, loadListing } from './listing';
import { sectionBySlug } from '$lib/taxonomy';
import { PAGE_SIZE, RUNNING_PREVIEW } from '$lib/api';
import type { PublicEvent } from '$lib/types';

/**
 * Прибор на загрузку страницы города и раздела.
 *
 * Мерится здесь ровно то, что нельзя увидеть глазами на живом стенде: как
 * страница ведёт себя, когда часть бэкенда молчит. Каждая ветка отказа — это
 * выбор между «сказать неправду с кодом 200» и «сказать приходи позже», и
 * ошибка в нём немая: доска отвечает 200, карточки на месте, неправда только
 * в числах и в том, чего на странице НЕ оказалось.
 *
 * Полоса добавила к этому ещё один отказ своего рода: список идущих — вторая
 * полоса, а не продукт, и ронять из-за него доску города нельзя.
 */

const ORIGIN = 'https://afisha.vshage.app';
const BASE = 'http://localhost:3003/api';

function qEvent(over: Partial<PublicEvent> = {}): PublicEvent {
  return {
    id: 'ev_' + Math.random().toString(16).slice(2, 10),
    title: 'Событие',
    start_time: '2026-09-12T19:00:00+03:00',
    tags: [],
    attendee_count: 0,
    status: 'published',
    is_featured: false,
    ...over
  };
}

interface QReply {
  status?: number;
  body?: unknown;
  /** Отказ сети, а не код ответа: fetch отвергает промис. */
  boom?: boolean;
}

/** Подставной бэкенд: отвечает по адресу и запоминает КАЖДЫЙ запрос. */
function qBackend(reply: (u: URL) => QReply) {
  const calls: URL[] = [];
  const fetchFn = (async (input: string | URL) => {
    const u = new URL(String(input));
    calls.push(u);
    const r = reply(u);
    if (r.boom) throw new TypeError('fetch failed');
    return new Response(JSON.stringify(r.body ?? {}), {
      status: r.status ?? 200,
      headers: { 'content-type': 'application/json' }
    });
  }) as unknown as typeof globalThis.fetch;
  return { fetch: fetchFn, calls };
}

/** Какая полоса запрошена этим адресом; null — параметра нет. */
function qKindOf(u: URL): string | null {
  return u.searchParams.get('kind');
}

const qLists = (calls: URL[]) => calls.filter((u) => u.pathname.endsWith('/events'));

/** Что бросил вызов — HttpError SvelteKit'а не наследует Error. */
async function qRejection(p: Promise<unknown>): Promise<{ status?: number }> {
  return p.then(
    () => {
      throw new Error('ожидался отказ, а вызов вернул значение');
    },
    (e) => e as { status?: number }
  );
}

function qThrown(fn: () => unknown): { status?: number } | null {
  try {
    fn();
    return null;
  } catch (e) {
    return e as { status?: number };
  }
}

beforeEach(() => {
  // Отказы пишутся в stdout намеренно — это единственный след в бою (логи
  // прода в Loki не доезжают). В прогоне их глушим, но факт записи проверяем.
  vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterEach(() => {
  vi.restoreAllMocks();
});

describe('kindFromSearch — неизвестная полоса это 404, а не сброс', () => {
  it('параметра нет — обе полосы', () => {
    expect(kindFromSearch(new URLSearchParams(''))).toBeNull();
  });

  // Пустое значение бэкенд читает как «не задан», и ссылка, собранная с
  // пустым параметром, обязана вести на обе полосы, а не в 404.
  it('пустое значение — тоже обе полосы', () => {
    expect(kindFromSearch(new URLSearchParams('kind='))).toBeNull();
  });

  it('известные значения проходят', () => {
    expect(kindFromSearch(new URLSearchParams('kind=running'))).toBe('running');
    expect(kindFromSearch(new URLSearchParams('kind=timed'))).toBe('timed');
  });

  // Молчаливый сброс выглядит успехом: страница отвечает 200 полной доской,
  // человек читает её как выдачу своего фильтра, а робот получает адрес-дубль.
  it('мусор отвергается кодом 404', () => {
    expect(qThrown(() => kindFromSearch(new URLSearchParams('kind=junk')))?.status).toBe(404);
    expect(qThrown(() => kindFromSearch(new URLSearchParams('kind=RUNNING')))?.status).toBe(404);
    expect(qThrown(() => kindFromSearch(new URLSearchParams('kind=1')))?.status).toBe(404);
  });
});

describe('loadListing — доска города, обе полосы', () => {
  const pin = qEvent({ id: 'ev_pinned', is_featured: true });

  function qBoard() {
    return qBackend((u) => {
      if (u.pathname.endsWith('/facets')) {
        return { body: { total: 260, kind: { running: 88, timed: 172 }, categories: [] } };
      }
      // Закреплённое приезжает с ОСНОВНОЙ полосой: полоса не входит в
      // Filter.IsEmpty бэкенда, поэтому `kind=timed` featured не обнуляет.
      if (qKindOf(u) === 'running')
        return { body: { featured: [], all: [qEvent({ kind: 'running' })], total: 88 } };
      return { body: { featured: [pin], all: [qEvent({ kind: 'timed' })], total: 172 } };
    });
  }

  it('основная полоса просится как «по дате и времени», вторая — превью', async () => {
    const b = qBoard();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });

    const lists = qLists(b.calls);
    const timed = lists.find((u) => qKindOf(u) === 'timed');
    const running = lists.find((u) => qKindOf(u) === 'running');
    expect(timed?.searchParams.get('limit')).toBe(String(PAGE_SIZE));
    expect(running?.searchParams.get('limit')).toBe(String(RUNNING_PREVIEW));

    expect(data.kind).toBeNull();
    expect(data.total).toBe(172);
    expect(data.running?.total).toBe(88);
    // Заголовок и мета подписываются числом ОБЕИХ полос: «Афиша Москвы ·
    // 88 событий» на доске, где их 260, было бы неправдой о городе.
    expect(data.grandTotal).toBe(260);
    expect(data.meta.description).toContain('260 событий');
  });

  // Закрепление — свойство доски города, и приезжает оно вместе с основной
  // полосой: `Kind` не входит в `Filter.IsEmpty` бэкенда (§3.1), иначе запрос
  // `kind=timed` обнулил бы featured и герой исчез бы с главной молча.
  // Отдельного запроса ради одного поля тут нет — и появиться он не должен.
  it('закреплённое приезжает вместе с основной полосой, тремя запросами', async () => {
    const b = qBoard();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(b.calls.length).toBe(3);
    expect(qLists(b.calls).map(qKindOf).sort()).toEqual(['running', 'timed']);
    expect(data.featured.map((e) => e.id)).toEqual(['ev_pinned']);
  });

  it('canonical не несёт полосы ни при каком запросе', async () => {
    const b = qBoard();
    for (const kind of [null, 'running', 'timed'] as const) {
      const data = await loadListing({
        fetch: b.fetch,
        origin: ORIGIN,
        citySlug: 'msk',
        section: null,
        kind
      });
      expect(data.meta.canonical).toBe(`${ORIGIN}/msk`);
    }
  });
});

describe('loadListing — выбрана одна полоса', () => {
  function qOneLane() {
    return qBackend((u) => {
      if (u.pathname.endsWith('/facets')) {
        return { body: { total: 260, kind: { running: 88, timed: 172 }, categories: [] } };
      }
      return {
        body: {
          featured: [qEvent({ id: 'ev_pinned' })],
          all: [qEvent({ kind: 'running' })],
          total: 88
        }
      };
    });
  }

  it('два запроса, полоса в query, второй полосы нет', async () => {
    const b = qOneLane();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(b.calls.length).toBe(2);
    expect(qLists(b.calls).map(qKindOf)).toEqual(['running']);
    expect(data.kind).toBe('running');
    expect(data.total).toBe(88);
    expect(data.running).toBeNull();
  });

  // На выбранной полосе закрепление не показывается по той же причине, по
  // которой его нет в разделе: закреплённое обещало бы, что закрепили именно
  // идущее. Правило держим и на клиенте — бэкенд его тоже соблюдает, но
  // правило важнее того, кто именно его сегодня выполняет.
  it('закреплённое не показывается', async () => {
    const b = qOneLane();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.featured).toEqual([]);
  });

  it('число обеих полос берётся из фасетов', async () => {
    const b = qOneLane();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.grandTotal).toBe(260);
  });

  // Фасеты просятся БЕЗ фильтра раздела (иначе плитка категорий схлопнулась
  // бы в одну), поэтому их kind — число по всему городу. Подставить его в
  // «Выставки в Москве · N событий» значило бы соврать ровно в ту сторону,
  // ради которой grandTotal и заведён.
  it('в разделе берётся число раздела, а не города', async () => {
    const b = qBackend((u) => {
      if (u.pathname.endsWith('/facets')) {
        return {
          body: {
            total: 260,
            kind: { running: 88, timed: 172 },
            categories: [{ code: 'exhibition', count: 40 }]
          }
        };
      }
      return { body: { featured: [], all: [qEvent({ kind: 'running' })], total: 31 } };
    });
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: sectionBySlug('exhibition') ?? null,
      kind: 'running'
    });
    expect(data.total).toBe(31);
    expect(data.grandTotal).toBe(40);
    expect(data.meta.canonical).toBe(`${ORIGIN}/msk/exhibition`);
  });
});

describe('loadListing — отказы разбираются по отдельности', () => {
  const qOk = (u: URL): QReply =>
    u.pathname.endsWith('/facets')
      ? { body: { total: 260, kind: { running: 88, timed: 172 }, categories: [] } }
      : { body: { featured: [], all: [qEvent({ kind: 'timed' })], total: 172 } };

  // Уронить доску города из-за превью двенадцати карточек было бы хуже того
  // отказа, ради которого 503 и заведён: основная лента уже приехала, и
  // человек пришёл именно за ней.
  it('отказ полосы идущих полосу не рисует, но страницу не роняет', async () => {
    const b = qBackend((u) => (qKindOf(u) === 'running' ? { status: 500 } : qOk(u)));
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(data.running).toBeNull();
    expect(data.events.length).toBe(1);
    // Молчание превратило бы «запрос упал» в «сегодня ничего не идёт» — а
    // такого утверждения мы делать не вправе.
    expect(console.error).toHaveBeenCalled();
  });

  it('сеть оборвалась на полосе идущих — тот же исход', async () => {
    const b = qBackend((u) => (qKindOf(u) === 'running' ? { boom: true } : qOk(u)));
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(data.running).toBeNull();
    expect(data.grandTotal).toBe(260);
  });

  // Правила основного списка не меняются полосой: 400 — это разъехавшиеся
  // словари, и человеку честнее 404, чем пустая лента.
  it('400 на основной полосе — 404', async () => {
    const b = qBackend((u) => (u.pathname.endsWith('/facets') ? qOk(u) : { status: 400 }));
    const e = await qRejection(
      loadListing({ fetch: b.fetch, origin: ORIGIN, citySlug: 'msk', section: null, kind: 'timed' })
    );
    expect(e.status).toBe(404);
  });

  // Пустая лента с кодом 200 — это утверждение «здесь пусто», сказанное и
  // человеку, и роботу, и закреплённое заголовком кэша на полчаса.
  it('любой другой отказ основной полосы — 503', async () => {
    const b = qBackend((u) => (u.pathname.endsWith('/facets') ? qOk(u) : { status: 500 }));
    const e = await qRejection(
      loadListing({ fetch: b.fetch, origin: ORIGIN, citySlug: 'msk', section: null, kind: 'running' })
    );
    expect(e.status).toBe(503);
  });

  it('мёртвые фасеты оставляют страницу без чисел, но с лентой', async () => {
    const b = qBackend((u) => (u.pathname.endsWith('/facets') ? { status: 500 } : qOk(u)));
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'timed'
    });
    expect(data.facets).toBeNull();
    // Число обеих полос взять негде — остаётся total своей полосы. Заниженное
    // честнее выдуманного.
    expect(data.grandTotal).toBe(172);
  });
});

describe('loadListing — бэкенд ещё не знает полос (фронт выкатился первым)', () => {
  // Фронт и бэкенд — два образа, порядок выката не гарантирован. Старый
  // бэкенд `kind` игнорирует и на ОБА запроса отдаёт одну и ту же доску:
  // полоса «идёт сейчас» стала бы копией верхушки основной. Отказ немой —
  // код 200, карточки настоящие, в логах ни строки.
  const qOldBackend = () =>
    qBackend((u) =>
      u.pathname.endsWith('/facets')
        ? // Разреза по полосам в ответе нет вовсе — он разберётся в нули.
          { body: { total: 260, categories: [{ code: 'exhibition', count: 40 }] } }
        : // Карточки без поля kind — по ним и опознаётся старый бэкенд.
          { body: { featured: [qEvent({ id: 'ev_pin' })], all: [qEvent(), qEvent()], total: 260 } }
    );

  it('вторая полоса не рисуется — доска выглядит как вчера', async () => {
    const b = qOldBackend();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(data.laneSplit).toBe(false);
    expect(data.running).toBeNull();
    // Основная лента при этом на месте: доска не деградирует, она просто
    // однополосная — ровно как до этой волны.
    expect(data.events.length).toBe(2);
    expect(data.featured.map((e) => e.id)).toEqual(['ev_pin']);
  });

  // Ноль из отсутствующего разреза — это «числа нет», а не «событий нет».
  // Подпись «Афиша Москвы · 0 событий» над сотней карточек была бы ложью
  // того же рода, ради которой заведён countsTrusted.
  it('число доски берётся из ленты, а не из пустого разреза фасетов', async () => {
    const b = qOldBackend();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(data.grandTotal).toBe(260);
    expect(data.meta.description).toContain('260 событий');
  });

  it('в разделе остальные разрезы фасетов по-прежнему верны', async () => {
    const b = qOldBackend();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: sectionBySlug('exhibition') ?? null,
      kind: 'timed'
    });
    expect(data.laneSplit).toBe(false);
    expect(data.grandTotal).toBe(40);
  });
});

describe('loadListing — раскладка есть, а полоса пуста', () => {
  // У НОВОГО бэкенда полоса идущих бывает пуста законно: в разделе может не
  // идти ничего. Опознавать «бэкенд старый» по пустому списку нельзя —
  // переключатель исчез бы у совершенно исправного стенда.
  it('пустая полоса не выдаётся за отсутствие полос', async () => {
    const b = qBackend((u) => {
      if (u.pathname.endsWith('/facets')) {
        return { body: { total: 172, kind: { running: 0, timed: 172 }, categories: [] } };
      }
      return qKindOf(u) === 'running'
        ? { body: { featured: [], all: [], total: 0 } }
        : { body: { featured: [], all: [qEvent({ kind: 'timed' })], total: 172 } };
    });
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: null
    });
    expect(data.laneSplit).toBe(true);
    expect(data.running).toEqual({ events: [], total: 0 });
    expect(data.grandTotal).toBe(172);
  });
});

describe('loadListing — числа пилюль считают ЭТУ страницу, а не город', () => {
  // Фасеты просятся БЕЗ фильтра раздела (иначе плитка категорий схлопнулась
  // бы в одну строку), поэтому их разрез по полосам — число по всему городу.
  // Пилюля же ведёт В РАЗДЕЛ: на `/msk/concert` она обещала бы «ИДЁТ СЕЙЧАС ·
  // 88» и открывала пустую сетку. Числа полос загрузчик знает точно и
  // бесплатно — это тоталы двух запросов, которые он и так делает.
  const qCityFacets = {
    total: 260,
    kind: { running: 88, timed: 172 },
    categories: [{ code: 'concert', count: 34 }]
  };

  function qSection(over: (u: URL) => QReply | null = () => null) {
    return qBackend((u) => {
      const custom = over(u);
      if (custom) return custom;
      if (u.pathname.endsWith('/facets')) return { body: qCityFacets };
      return qKindOf(u) === 'running'
        ? { body: { featured: [], all: [], total: 0 } }
        : { body: { featured: [], all: [qEvent({ kind: 'timed' })], total: 34 } };
    });
  }

  const qConcert = () => sectionBySlug('concert') ?? null;

  it('числа берутся из тоталов полос раздела', async () => {
    const b = qSection();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: qConcert(),
      kind: null
    });
    expect(data.laneCounts).toEqual({ timed: 34, running: 0 });
  });

  // Выбрана одна полоса — второй запрос не делался, значит второго числа НЕТ.
  // Подставить сюда городской разрез фасетов значило бы вернуть ровно тот
  // дефект, ради которого числа и переехали.
  it('выбрана одна полоса — чисел нет вовсе', async () => {
    const b = qSection();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: qConcert(),
      kind: 'timed'
    });
    expect(data.laneCounts).toBeNull();
  });

  it('полоса идущих не ответила — чисел нет, а не число города', async () => {
    const b = qSection((u) => (qKindOf(u) === 'running' ? { status: 500 } : null));
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: qConcert(),
      kind: null
    });
    expect(data.laneCounts).toBeNull();
  });

  // Источник, молчавший при выдаче списка, занижает его total — а заниженное
  // число выглядит достоверным. То же правило, что у countsTrusted, только
  // теперь оно про тоталы полос: числа пилюль приходят из списков.
  it('молчал источник списка — чисел нет', async () => {
    const b = qSection((u) =>
      qKindOf(u) === 'timed'
        ? {
            body: {
              featured: [],
              all: [qEvent({ kind: 'timed' })],
              total: 34,
              degraded: ['tgevents']
            }
          }
        : null
    );
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: qConcert(),
      kind: null
    });
    expect(data.laneCounts).toBeNull();
  });
});

describe('loadListing — выбранная полоса пуста, а бэкенд исправен', () => {
  // Самый тяжёлый из немых отказов: `/msk?kind=running` в день, когда идущих
  // нет. Второго списка не спрашивали, первый пуст — и признак «бэкенд умеет
  // полосы», выведенный из карточек, отвечает «не умеет». Дальше по цепочке
  // доска подписывается нулём: «Афиша Москвы · 0 событий», то же в мете и
  // пустой ItemList в разметке, и заголовок кэша разрешает раздавать это
  // полчаса. То есть страница УТВЕРЖДАЕТ человеку и роботу, что в Москве
  // событий нет.
  const qEmptyLane = () =>
    qBackend((u) =>
      u.pathname.endsWith('/facets')
        ? { body: { total: 408, kind: { running: 0, timed: 408 }, categories: [] } }
        : { body: { featured: [], all: [], total: 0 } }
    );

  it('город не объявляется пустым', async () => {
    const b = qEmptyLane();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.total).toBe(0);
    expect(data.grandTotal).toBe(408);
    expect(data.meta.description).toContain('408 событий');
  });

  // Без переключателя из пустой полосы не выйти ничем, кроме кнопки «назад»,
  // а метка сетки откатывается на дововолновую «СОБЫТИЯ · 0» — страница
  // начинает противоречить собственному заголовку.
  it('переключатель остаётся: полосы опознаются по разрезу фасетов', async () => {
    const b = qEmptyLane();
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.laneSplit).toBe(true);
  });

  // Обратная сторона того же признака: у бэкенда, который полос не знает,
  // разреза в ответе НЕТ вовсе. Если разбор фасетов подставит нули, признак
  // станет «фасеты ответили» и переключатель нарисуется там, где полос не
  // существует, — пилюля повела бы на выдачу, не отличимую от доски.
  it('старый бэкенд разреза не шлёт — полос по-прежнему нет', async () => {
    const b = qBackend((u) =>
      u.pathname.endsWith('/facets')
        ? { body: { total: 408, categories: [] } }
        : { body: { featured: [], all: [], total: 0 } }
    );
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.laneSplit).toBe(false);
  });

  // СТОРОЖ (был зелёным и до правки): фасеты могут не ответить вовсе — тогда
  // полосы опознаются по карточкам, как и раньше. Признак из фасетов
  // ДОБАВЛЕН к признаку из карточек, а не заменил его.
  it('фасеты мертвы — полосы опознаются по карточкам', async () => {
    const b = qBackend((u) =>
      u.pathname.endsWith('/facets')
        ? { status: 500 }
        : { body: { featured: [], all: [qEvent({ kind: 'running' })], total: 3 } }
    );
    const data = await loadListing({
      fetch: b.fetch,
      origin: ORIGIN,
      citySlug: 'msk',
      section: null,
      kind: 'running'
    });
    expect(data.laneSplit).toBe(true);
  });
});
