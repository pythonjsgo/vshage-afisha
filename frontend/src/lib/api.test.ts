import { describe, it, expect, vi, afterEach } from 'vitest';
import { eventsSearch, getFacets, RUNNING_PREVIEW, PAGE_SIZE, MAX_WINDOW } from './api';

/**
 * Прибор на слой запроса.
 *
 * Полоса доски (`kind`) едет ТОЛЬКО отсюда: страница её не собирает руками.
 * Значит и ошибиться можно ровно в двух местах — не положить параметр в
 * запрос (и молча получить обе полосы там, где человек выбрал одну) или
 * положить пустым (и развести ключ кэша бэкенда на ровном месте). Оба отказа
 * немые: страница отвечает 200 и выглядит рабочей.
 *
 * Разрез фасетов по полосам проверяется через getFacets, а не через
 * внутренний normalizeFacets: разбор обязан пережить ответ бэкенда, который
 * про полосы ещё не знает, — фронт и бэкенд выкатываются по отдельности, и
 * между двумя выкатками живёт ровно этот случай.
 */

/** Ответ фасетов из подставного бэкенда. */
function qFacetsFetch(body: unknown): typeof globalThis.fetch {
	return (async () =>
		new Response(JSON.stringify(body), {
			status: 200,
			headers: { 'content-type': 'application/json' }
		})) as unknown as typeof globalThis.fetch;
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('eventsSearch — полоса уезжает в запрос только когда выбрана', () => {
	it('без полосы параметра нет вовсе', () => {
		const p = eventsSearch({ city: 'msk' });
		expect(p.has('kind')).toBe(false);
	});

	it('выбранная полоса едет как есть', () => {
		expect(eventsSearch({ city: 'msk', kind: 'running' }).get('kind')).toBe('running');
		expect(eventsSearch({ city: 'msk', kind: 'timed' }).get('kind')).toBe('timed');
	});

	it('полоса не мешает остальным параметрам раздела', () => {
		const p = eventsSearch({ city: 'msk', category: 'exhibition', kind: 'running', limit: 12 });
		expect(p.get('city')).toBe('msk');
		expect(p.get('category')).toBe('exhibition');
		expect(p.get('kind')).toBe('running');
		expect(p.get('limit')).toBe('12');
	});

	// Потолок окна листания полосой не меняется: за ним бэкенд отвечает 400, и
	// «показать ещё» стала бы кнопкой, которая молча ничего не делает.
	it('limit по-прежнему подрезается общим окном', () => {
		const p = eventsSearch({ city: 'msk', kind: 'timed', offset: MAX_WINDOW - 10, limit: 100 });
		expect(Number(p.get('limit'))).toBe(10);
	});
});

describe('getFacets — разрез по полосам', () => {
	it('читается, когда бэкенд его прислал', async () => {
		const f = await getFacets(qFacetsFetch({ kind: { running: 88, timed: 172 } }), 'msk');
		expect(f.kind).toEqual({ running: 88, timed: 172 });
	});

	// РАЗНЫЕ ОТВЕТЫ, и разница между ними единственная, по которой фронт
	// отличает бэкенд, который полос ещё не знает, от пустой доски у бэкенда,
	// который их знает. Прежний разбор подставлял нули в ОБОИХ случаях и этим
	// стирал различие: на исправном стенде с пустой полосой переключатель исчезал
	// вместе с дорогой обратно. Поэтому отсутствие остаётся отсутствием — тот же
	// приём, что уже взят для `degraded`. Страница при этом не падает, как и раньше.
	it('разреза нет вовсе — это НЕ нули', async () => {
		const f = await getFacets(qFacetsFetch({ total: 260 }), 'msk');
		expect(f.kind).toBeUndefined();
		expect(f.total).toBe(260);
	});

	it('нули приехали разрезом — это не отсутствие', async () => {
		const f = await getFacets(qFacetsFetch({ kind: { running: 0, timed: 0 } }), 'msk');
		expect(f.kind).toEqual({ running: 0, timed: 0 });
	});

	// Разбор ВНУТРИ ключа остаётся терпимым: половина разреза — это
	// половина разреза, а не его отсутствие.
	it('половина разреза — вторая половина ноль', async () => {
		const f = await getFacets(qFacetsFetch({ kind: { running: 5 } }), 'msk');
		expect(f.kind).toEqual({ running: 5, timed: 0 });
	});
});

describe('RUNNING_PREVIEW — превью, а не порция листания', () => {
	// Полоса «идёт сейчас» на двухполосной странице приглашает зайти в неё
	// целиком, а не выкладывает вторую сотню карточек: иначе лента по дате и
	// времени, ради которой полосы и разводились, уезжает вниз экрана.
	it('заметно меньше страницы листания', () => {
		expect(RUNNING_PREVIEW).toBeGreaterThan(0);
		expect(RUNNING_PREVIEW).toBeLessThan(PAGE_SIZE);
	});
});
