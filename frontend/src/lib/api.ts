import type { ListResult, PublicEvent } from './types';
import type { City } from './seo';
import { sectionQuery, type Section } from './taxonomy';

const BASE = import.meta.env.PUBLIC_API_URL ?? '/api';
// В браузере BASE — публичный адрес, вшитый Vite на сборке (см. Dockerfile).
// SSR ходит в бэкенд по внутреннему адресу докер-сети: его передаёт
// +page.server.ts третьим аргументом, потому что process.env в браузере нет.

/**
 * Города, известные фронту.
 *
 * Список ДУБЛИРУЕТ backend/internal/events/cities.go намеренно. Матчер
 * маршрута синхронный: он обязан ответить «есть такой город» до всякого
 * запроса в сеть — иначе `/hoi` открылся бы пустой лентой вместо 404, а
 * пустая лента по несуществующему адресу это страница-дубль в индексе
 * поисковика. Цена дубликата — обязанность пополнять оба списка ОДНИМ
 * заходом; цена асинхронной проверки была бы 404, которого не существует.
 */
export const CITY_SLUGS: readonly string[] = ['msk'];
export const DEFAULT_CITY_SLUG = 'msk';

/**
 * Город, которым рисуется страница, когда фасеты не ответили. Не «данные по
 * умолчанию», а падежи для заголовка: без них шапка написала бы «Афиша
 * undefined». Сам факт отказа при этом уходит в stderr — молча подставлять
 * нельзя, иначе мёртвый эндпоинт фасетов выглядит здоровьем.
 */
export const FALLBACK_CITY: City = {
	slug: DEFAULT_CITY_SLUG,
	name: 'Москва',
	in: 'в Москве',
	of: 'Москвы'
};

/**
 * Сколько карточек в одной порции — и на SSR, и в «показать ещё».
 *
 * Цифра — компромисс между весом первой отрисовки и числом нажатий. Прежняя
 * главная просила 200 разом и весила 327 КБ разметки; при 100 первый экран
 * остаётся насыщенным, вес падает вдвое (это прямо влияет на LCP, то есть на
 * ранжирование), а доска на 407 карточек разбирается четырьмя нажатиями.
 * Двигать её вниз имеет смысл, только если замер LCP на мобильном скажет, что
 * и ста много: глубже 2-3 экранов люди уходят в раздел, а не листают.
 */
export const PAGE_SIZE = 100;

/**
 * Зеркало maxWindow бэкенда (offset+limit). За ним бэкенд отвечает 400, и
 * «показать ещё» стала бы кнопкой, которая молча ничего не делает. Когда
 * доска перерастёт это число, недостающее нельзя будет открыть НИОТКУДА —
 * лечится не увеличением цифры, а тем, что человек уходит в раздел.
 */
export const MAX_WINDOW = 1000;

/**
 * Полоса доски. Разбиение полное и непересекающееся: каждое событие ровно в
 * одной полосе, и решает это сервер (`kind` в ответе), а не фронт.
 *
 * `running` — многодневная программа, которая уже открылась и ещё не
 * закрылась; `timed` — всё остальное, то есть точечные события и периоды,
 * которые ещё не начались.
 */
export type EventKind = 'running' | 'timed';

/**
 * Сколько идущих программ показывается на двухполосной странице.
 *
 * Это ПРЕВЬЮ, а не порция листания: полоса «идёт сейчас» на доске города —
 * приглашение зайти в неё целиком (`?kind=running`), а не второй список на
 * сто карточек. Двенадцать — три ряда сетки на широком экране; больше
 * отодвинуло бы вниз ту самую ленту по дате и времени, ради которой полосы и
 * разводились.
 */
export const RUNNING_PREVIEW = 12;

/**
 * Отказ API вместе с кодом.
 *
 * Код нужен вызывающему: 400 от списка означает, что словарь разделов на
 * фронте и словарь бэкенда РАЗЪЕХАЛИСЬ, и человеку надо ответить 404, а не
 * пустой лентой. Голый Error заставил бы разбирать текст сообщения строкой.
 */
export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

export interface EventsQuery {
	city: string;
	category?: string;
	when?: string;
	/** Только '1' — так у бэкенда. */
	free?: string;
	/**
	 * Полоса. Не задана — обе вместе, как было до появления полос. В запрос
	 * уезжает ТОЛЬКО когда задана: пустой `kind=` бэкенд читает как «не
	 * задан», но лишний параметр разводит ключ кэша и адрес «показать ещё».
	 */
	kind?: EventKind;
	limit?: number;
	offset?: number;
}

/** Параметры списка для города и (необязательного) раздела. */
export function queryForSection(citySlug: string, section: Section | null): EventsQuery {
	const q: EventsQuery = { city: citySlug };
	if (!section) return q;
	// Раскладка «раздел → параметр» живёт в taxonomy.ts и только там: вторая
	// копия разъедется в тот день, когда добавится раздел нового вида.
	const s = sectionQuery(section);
	if (s.category) q.category = s.category;
	if (s.when) q.when = s.when;
	if (s.free) q.free = s.free;
	return q;
}

export function eventsSearch(q: EventsQuery): URLSearchParams {
	const p = new URLSearchParams();
	// Город передаём ВСЕГДА, даже когда он один. Иначе второй город приедет
	// данными и молча смешается с первым — а заметить это будет некому.
	p.set('city', q.city);
	if (q.category) p.set('category', q.category);
	if (q.when) p.set('when', q.when);
	if (q.free) p.set('free', q.free);
	if (q.kind) p.set('kind', q.kind);
	const offset = Math.max(0, Math.trunc(q.offset ?? 0));
	const limit = Math.max(1, Math.min(Math.trunc(q.limit ?? PAGE_SIZE), MAX_WINDOW - offset));
	if (offset > 0) p.set('offset', String(offset));
	p.set('limit', String(limit));
	return p;
}

export async function getEvents(
	fetch: typeof globalThis.fetch,
	q: EventsQuery,
	base = BASE
): Promise<ListResult> {
	const res = await fetch(`${base}/events?${eventsSearch(q)}`);
	if (!res.ok) throw new ApiError(res.status, `events list: ${res.status}`);
	const data = (await res.json()) as ListResult;
	// Отказ одного из источников приходит в ответе 200 с непустым degraded —
	// снаружи это неотличимо от «в источнике просто ничего нет». Логи PROD в
	// Loki не доезжают, поэтому строка идёт в stdout контейнера: на неё можно
	// повесить смок.
	if (data.degraded?.length) {
		console.error('afisha: лента деградировала, молчат источники:', data.degraded.join(', '));
	}
	return data;
}

export interface CategoryFacet {
	code: string;
	count: number;
}

export interface WhenFacets {
	today: number;
	tomorrow: number;
	weekend: number;
}

/**
 * Сколько событий в каждой полосе. Считается со всеми прочими условиями
 * фильтра, но БЕЗ своего собственного — иначе на `?kind=running` вторая
 * пилюля показала бы ноль и выглядела бы пустым разделом.
 */
export interface KindFacets {
	running: number;
	timed: number;
}

export interface Facets {
	city: City;
	cities: City[];
	total: number;
	/** Только count > 0, по убыванию count — пустые рубрики бэкенд не шлёт. */
	categories: CategoryFacet[];
	when: WhenFacets;
	/**
	 * Разрез по полосам. Необязателен НАМЕРЕННО, и подставлять вместо него нули
	 * нельзя: его отсутствие — единственный признак того, что бэкенд полос ещё
	 * не знает. Нули означают ровно противоположное — бэкенд полосы посчитал, и
	 * в них сейчас пусто. Разбор, стирающий это различие, гасит переключатель на
	 * совершенно исправном стенде с пустой полосой, то есть отбирает дорогу
	 * обратно ровно там, где она нужна. Тот же приём, что уже взят у `degraded`.
	 */
	kind?: KindFacets;
	free: number;
	/**
	 * Источники ленты, которые не ответили при подсчёте. Непустой массив
	 * означает, что ВСЕ числа ниже занижены на вклад молчащего стора — а
	 * заниженное число выглядит достоверным. Бэкенд его для этого и завёл
	 * (internal/events/models.go), и потерять его здесь значит превратить
	 * частичный ответ в уверенное враньё на плитке.
	 */
	degraded?: string[];
}

/**
 * Разбор фасетов терпимый к неполному ответу: каждое поле необязательно.
 *
 * Это не перестраховка. Плитка и ряд фильтров рисуются ОДНИМ ответом, и
 * жёсткий разбор означал бы, что стенд со старым бэкендом (или бэкенд, где
 * `when` ещё не посчитан) роняет всю страницу города вместо того, чтобы
 * показать ленту без счётчиков.
 */
function normalizeFacets(raw: unknown, citySlug: string): Facets {
	const o = (raw ?? {}) as Partial<Facets>;
	const city = o.city ?? { ...FALLBACK_CITY, slug: citySlug };
	const cities = Array.isArray(o.cities) && o.cities.length > 0 ? o.cities : [city];
	const categories = Array.isArray(o.categories)
		? o.categories.filter((c) => c && typeof c.code === 'string' && c.count > 0)
		: [];
	const when = o.when ?? { today: 0, tomorrow: 0, weekend: 0 };
	// Разрез по полосам приезжает с бэкенда этой же волны, а фронт умеет
	// оказаться старше или новее его на один деплой. Отсутствие ключа
	// СОХРАНЯЕТСЯ: по нему страница отличает «бэкенд полос не знает» от
	// «полосы посчитаны, и в одной из них пусто». Разбор внутри ключа остаётся
	// терпимым — половина разреза это половина разреза.
	const kind =
		o.kind && typeof o.kind === 'object'
			? { running: o.kind.running ?? 0, timed: o.kind.timed ?? 0 }
			: undefined;
	return {
		city,
		cities,
		degraded: Array.isArray(o.degraded) && o.degraded.length > 0 ? o.degraded : undefined,
		total: typeof o.total === 'number' ? o.total : 0,
		categories,
		when: {
			today: when.today ?? 0,
			tomorrow: when.tomorrow ?? 0,
			weekend: when.weekend ?? 0
		},
		kind,
		free: typeof o.free === 'number' ? o.free : 0
	};
}

export async function getFacets(
	fetch: typeof globalThis.fetch,
	citySlug: string,
	base = BASE
): Promise<Facets> {
	const res = await fetch(`${base}/events/facets?city=${encodeURIComponent(citySlug)}`);
	if (!res.ok) throw new ApiError(res.status, `events facets: ${res.status}`);
	const facets = normalizeFacets(await res.json(), citySlug);
	// Логируем так же, как getEvents логирует деградацию списка. Асимметрия
	// была бы худшего сорта: у списка неполнота видна по числу карточек, а у
	// фасетов — это ровно те цифры, которые мы печатаем обещанием возле
	// каждой плитки.
	if (facets.degraded?.length) {
		console.error(
			'afisha: счётчики неполные, молчат источники:',
			facets.degraded.join(', ')
		);
	}
	return facets;
}

export async function getEvent(
	id: string,
	fetch: typeof globalThis.fetch,
	base = BASE
): Promise<PublicEvent | null> {
	const res = await fetch(`${base}/events/${encodeURIComponent(id)}`);
	if (res.status === 404) return null;
	if (!res.ok) throw new ApiError(res.status, `event get: ${res.status}`);
	return res.json();
}
