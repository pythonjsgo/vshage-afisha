import { error } from '@sveltejs/kit';
import type { PublicEvent } from '$lib/types';
import type { Section } from '$lib/taxonomy';
import { listingMeta, type City, type MetaTags } from '$lib/seo';
import {
  ApiError,
  FALLBACK_CITY,
  PAGE_SIZE,
  getEvents,
  getFacets,
  queryForSection,
  type Facets
} from '$lib/api';

/**
 * Общая загрузка страницы города и страницы раздела.
 *
 * Один файл на два маршрута намеренно: они отличаются РОВНО одним аргументом
 * (раздел), а разъезжаются такие пары мгновенно — стоит поправить обработку
 * отказа в одном месте и забыть про второе, и одна из двух страниц начнёт
 * молча врать. Файл лежит в каталоге маршрута и роутером не подхватывается:
 * SvelteKit считает страницами только файлы с «+».
 */
export interface ListingData {
  origin: string;
  city: City;
  facets: Facets | null;
  /**
   * false — часть счётчиков занижена: один из сторов молчал при подсчёте.
   * Тогда цифры не рисуются вовсе. Ноль в разделе читается человеком как
   * «сегодня ничего нет», и такое утверждение мы делать не вправе.
   */
  countsTrusted: boolean;
  section: Section | null;
  featured: PublicEvent[];
  events: PublicEvent[];
  total: number;
  meta: MetaTags;
  /** Путь, а не абсолютный адрес: ссылка в разметке обязана остаться своей. */
  crumbs: { name: string; path: string }[];
}

export async function loadListing(opts: {
  fetch: typeof globalThis.fetch;
  origin: string;
  citySlug: string;
  section: Section | null;
}): Promise<ListingData> {
  const { fetch, origin, citySlug, section } = opts;
  // Тот же адрес по умолчанию, что у остальных загрузчиков этого фронта:
  // в бою его всегда задаёт compose, менять умолчание в одном файле из
  // четырёх — верный способ получить расхождение стендов.
  const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
  // База для api.ts включает /api — в браузере она вшита Vite ровно такой
  // (https://afisha.vshage.app/api), а BACKEND_INTERNAL_URL указывает на
  // корень сервиса. Забыть здесь /api значит получить 404 на каждый запрос
  // при полностью живом бэкенде, причём страница при этом отвечает 200 с
  // пустой лентой — отказ немой.
  const base = `${backend}/api`;
  const query = queryForSection(citySlug, section);

  // Список и фасеты просим параллельно и разбираем отказы ПО ОТДЕЛЬНОСТИ:
  // фасеты — это счётчики, список — это сам продукт. Мёртвые счётчики не
  // повод не показать ленту.
  const [listRes, facetsRes] = await Promise.allSettled([
    getEvents(fetch, { ...query, limit: PAGE_SIZE, offset: 0 }, base),
    getFacets(fetch, citySlug, base)
  ]);

  if (listRes.status === 'rejected') {
    const reason = listRes.reason;
    // 400 на списке означает, что словарь разделов фронта и словарь бэкенда
    // разъехались: матчер пропустил слаг, которого бэкенд не знает. Человеку
    // честнее 404, чем пустая лента, а причину читает оператор в stdout.
    if (reason instanceof ApiError && reason.status === 400) {
      console.error(
        'afisha: бэкенд отверг параметры раздела — словари разъехались:',
        JSON.stringify(query),
        reason.message
      );
      error(404, 'Такого раздела нет');
    }
    // Любой другой отказ — это «источники молчат», а не «событий нет».
    // Подставить пустую ленту значило бы отдать 200 с честной на вид доской:
    // подпись «ВСЕ СОБЫТИЯ · 0», описание «0 событий с датами, местами и
    // ценами» в мете и пустой ItemList в разметке — то есть мы бы СКАЗАЛИ
    // роботу, что в Москве сегодня ничего нет, и заголовок кэша разрешил бы
    // раздавать это полчаса. 503 честен ровно в ту сторону, в какую нужно:
    // и человек, и робот понимают «приходи позже», страница ошибки не несёт
    // ни разметки, ни успешного кэша. Логи прода в Loki не доезжают, поэтому
    // строка выше — единственный след, и она обязана остаться.
    console.error('afisha: список событий не ответил:', reason);
    error(503, 'Лента временно недоступна');
  }
  if (facetsRes.status === 'rejected') {
    // Молча подставить нули нельзя: страница выглядела бы здоровой, а плитка
    // и «быстрый выбор» исчезли бы без объяснения. Строка в stdout — то, на
    // что можно повесить смок.
    console.error('afisha: фасеты не ответили, страница без счётчиков:', facetsRes.reason);
  }

  const list = listRes.status === 'fulfilled' ? listRes.value : { featured: [], all: [], total: 0 };
  let facets = facetsRes.status === 'fulfilled' ? facetsRes.value : null;
  const city: City = facets?.city ?? { ...FALLBACK_CITY, slug: citySlug };

  // Активная рубрика обязана быть в плитке, даже когда бэкенд её не прислал:
  // фасеты несут только count > 0, а раздел с нулём событий — адрес живой,
  // матчер его пропускает. Без этой вставки на пустом разделе подсвечивать
  // нечего, и человек не понимает, где он находится.
  if (
    facets &&
    section?.kind === 'category' &&
    !facets.categories.some((c) => c.code === section.code)
  ) {
    facets = {
      ...facets,
      categories: [...facets.categories, { code: section.code, count: list.total }]
    };
  }

  const meta = listingMeta({
    origin,
    city,
    total: list.total,
    heading: section?.heading,
    blurb: section?.blurb,
    sectionSlug: section?.slug
  });

  const crumbs = [{ name: `Афиша ${city.of}`, path: `/${city.slug}` }];
  if (section) crumbs.push({ name: section.tile, path: `/${city.slug}/${section.slug}` });

  // Неполные счётчики не показываем вовсе. Правило уже сформулировано в
  // FilterRow: «неверное число хуже отсутствующего» — ноль в разделе значит
  // «сегодня ничего нет», и это может быть ложью. Плитка при этом остаётся:
  // раздел без цифры ведёт туда же, куда вёл.
  const countsTrusted = !facets?.degraded?.length;

  return {
    origin,
    city,
    facets,
    countsTrusted,
    section,
    // Закрепление относится к доске города, а не к разделу: закреплённое
    // событие другой рубрики в шапке раздела — ложь. Бэкенд отдаёт здесь
    // пустой массив, но проверку держим и на клиенте: правило важнее того,
    // кто именно его сегодня соблюдает.
    featured: section ? [] : list.featured,
    events: list.all,
    total: list.total,
    meta,
    crumbs
  };
}
