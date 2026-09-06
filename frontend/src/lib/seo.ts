/**
 * Разметка для поисковика: канонический адрес, мета-теги и schema.org.
 *
 * Почему это отдельный модуль, а не «по месту в каждом svelte:head». Ровно
 * потому же, почему в кабинете вынесен словарь видимости: подписи, адреса и
 * структурная разметка одного события живут на ЧЕТЫРЁХ поверхностях (карточка
 * события, страница города, страница раздела, карта сайта). Разложенные по
 * компонентам, они расходятся молча — сайт при этом отвечает 200, и заметить
 * расхождение может только робот поисковика, который нам ничего не скажет.
 *
 * Чего здесь сознательно НЕТ: адреса события собираются функцией eventUrl и
 * только ею — сегодня это /<id>, и красивый слаг (задача следующего захода)
 * поменяется в одном месте вместе с редиректом.
 */

import type { PublicEvent } from './types';
import { plural } from './taxonomy';

/** Город в адресе. Форма приезжает с бэкенда — падежи не выводим правилом. */
export interface City {
  slug: string;
  name: string;
  /** Предложный: «в Москве». */
  in: string;
  /** Родительный: «афиша Москвы». */
  of: string;
  count?: number;
}

export const SITE_NAME = 'Вшаге';

/**
 * Календарная дата момента по Москве, `YYYY-MM-DD`.
 *
 * Собирается из частей `formatToParts`, а не форматом локали: локаль решает
 * порядок и разделители, и «удобный» приём с 'sv-SE' зависит от полноты ICU
 * в рантайме. Части же называются одинаково везде.
 */
export function mskDate(iso: string): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Europe/Moscow',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).formatToParts(new Date(iso));
  const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '';
  const y = get('year'), m = get('month'), d = get('day');
  // Пустая часть означает нерабочий Intl — тогда честнее вернуть исходный
  // срез, чем собрать «--» и выдать его за дату.
  return y && m && d ? `${y}-${m}-${d}` : iso.slice(0, 10);
}

/**
 * Канонический адрес события. ТРИ маршрута ведут на одну карточку
 * (/<id>, /events/<id> → 308, /e/<slug>), и без канонического поисковик
 * считает их разными страницами с одинаковым текстом.
 */
export function eventUrl(origin: string, ev: Pick<PublicEvent, 'id' | 'webreg_slug'>): string {
  // Кодирование обязательно: слаг веб-регистрации приходит из формы
  // организатора, а не из нашего генератора. Пробел или «?» в нём дают битый
  // <loc> в карте сайта, и XML-экранирование от этого не спасает — оно про
  // «&» и «<», а не про структуру адреса.
  return ev.webreg_slug
    ? `${origin}/e/${encodeURIComponent(ev.webreg_slug)}`
    : `${origin}/${encodeURIComponent(ev.id)}`;
}

export function listingUrl(origin: string, citySlug: string, sectionSlug?: string): string {
  return sectionSlug ? `${origin}/${citySlug}/${sectionSlug}` : `${origin}/${citySlug}`;
}

/** «417 событий» / «1 событие» — окончание считается, а не приклеивается. */
export function eventsCount(n: number): string {
  return `${n} ${plural(n, 'событие', 'события', 'событий')}`;
}

export interface MetaTags {
  title: string;
  description: string;
  canonical: string;
  ogImage: string;
  /** website у списков, article у карточки события. */
  ogType: 'website' | 'article';
}

/** Мета-теги страницы города или раздела. */
export function listingMeta(opts: {
  origin: string;
  city: City;
  total: number;
  heading?: string;
  blurb?: string;
  sectionSlug?: string;
}): MetaTags {
  const { origin, city, total, heading, blurb, sectionSlug } = opts;
  const what = heading ?? 'Афиша';
  const title = heading
    ? `${heading} ${city.in} — афиша событий · ${SITE_NAME}`
    : `Афиша ${city.of} — куда сходить · ${SITE_NAME}`;
  const tail = blurb ? `: ${blurb}` : '';
  const description =
    `${what} ${city.in}${tail}. ${eventsCount(total)} с датами, местами и ценами. ` +
    'Афиша обновляется каждый день.';
  return {
    title,
    description,
    canonical: listingUrl(origin, city.slug, sectionSlug),
    ogImage: `${origin}/og-default.png`,
    ogType: 'website'
  };
}

/**
 * schema.org/Event — то, ради чего вся разметка и затевалась: без него афиша
 * не попадает ни в блок мероприятий Google, ни в колдунщик Яндекса.
 *
 * Обязательные по требованиям Google поля — name, startDate, location; всё
 * остальное поднимает шанс расширенного сниппета. Врать в них нельзя:
 * неверная разметка снимает сайт с показа целиком, а не «просто не работает».
 */
export function eventJsonLd(ev: PublicEvent, origin: string, city?: City): Record<string, unknown> {
  const url = eventUrl(origin, ev);
  const abs = (u: string) => (u.startsWith('http') ? u : `${origin}${u}`);

  // Время известно не у всех карточек (у 40% доски его нет вовсе). Отдавать
  // тогда «00:00» — значит подписать событие временем, которого мы не знаем,
  // и робот покажет это человеку как факт. schema.org допускает голую дату.
  //
  // Дата считается В МОСКОВСКОЙ ЗОНЕ, а не срезается первыми десятью знаками
  // строки. Срез верен ровно до тех пор, пока источник сериализует смещение
  // +03:00 (так делает tgevents), и молча ошибается на сутки в тот день,
  // когда признак «времени нет» начнёт ставить источник, отдающий UTC:
  // полночь 12-го по Москве — это 21:00 11-го по Гринвичу.
  //
  // И ещё: `start_time` у идущей многодневной программы СДВИНУТ на сегодня
  // ради сортировки. Человеку это читается верно, роботу — как «выставка
  // начинается сегодня», причём заново каждый день. Настоящий первый день
  // приезжает отдельным полем ровно для этого случая.
  const startDate =
    ev.actual_start_date ??
    (ev.start_time_known === false ? mskDate(ev.start_time) : ev.start_time);

  const node: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'Event',
    name: ev.title,
    startDate,
    eventStatus: 'https://schema.org/EventScheduled',
    url
  };

  // У импортированной карточки конец программы — это ДАТА, а не момент:
  // бэкенд ставит последний день плюс 23:59 как сторож «до конца дня»
  // (tgevents/afisha.go). Отдать его как есть — сообщить роботу, что
  // выставка закрывается в 23:59. Времени закрытия мы не знаем ни у одной
  // такой карточки.
  if (ev.end_time) {
    node.endDate = ev.source === 'tg' ? mskDate(ev.end_time) : ev.end_time;
  }
  const desc = (ev.short_description || ev.description || '').trim().replace(/\s+/g, ' ');
  if (desc) node.description = desc.slice(0, 500);
  if (ev.photo_url) node.image = [abs(ev.photo_url)];

  // location обязателен, и у онлайна он ДРУГОГО типа. Пометить событие
  // онлайновым, оставив физический Place, — это не «неточность», а
  // невалидная разметка: Google отбрасывает такое событие целиком, то есть
  // отказ немой и выглядит как «нас просто не показывают».
  const placeName = ev.venue_name || ev.location || city?.name;
  const hasPhysical = Boolean(placeName || ev.address);
  const virtual = ev.online_url
    ? { '@type': 'VirtualLocation', url: abs(ev.online_url) }
    : undefined;

  node.eventAttendanceMode = virtual
    ? hasPhysical
      ? 'https://schema.org/MixedEventAttendanceMode'
      : 'https://schema.org/OnlineEventAttendanceMode'
    : 'https://schema.org/OfflineEventAttendanceMode';

  if (hasPhysical) {
    const address: Record<string, unknown> = {
      '@type': 'PostalAddress',
      addressCountry: 'RU'
    };
    if (city?.name) address.addressLocality = city.name;
    if (ev.address) address.streetAddress = ev.address;
    const place: Record<string, unknown> = {
      '@type': 'Place',
      name: placeName ?? ev.address,
      address
    };
    if (typeof ev.venue_lat === 'number' && typeof ev.venue_lon === 'number') {
      place.geo = { '@type': 'GeoCoordinates', latitude: ev.venue_lat, longitude: ev.venue_lon };
    }
    node.location = virtual ? [place, virtual] : place;
  } else if (virtual) {
    node.location = virtual;
  }

  // Мест может не остаться, и данные для честного ответа у нас есть прямо
  // здесь. Безусловный InStock — это приглашение прийти туда, куда уже не
  // пускают; для робота это такой же факт, как цена.
  const soldOut =
    typeof ev.max_attendees === 'number' &&
    ev.max_attendees > 0 &&
    ev.attendee_count >= ev.max_attendees;
  const availability = soldOut
    ? 'https://schema.org/SoldOut'
    : 'https://schema.org/InStock';

  // offers: цену объявляем только когда знаем её. «0» по умолчанию — это
  // обещание бесплатного входа от нашего имени на чужое событие.
  if (ev.price_type === 'free') {
    node.offers = {
      '@type': 'Offer',
      price: 0,
      priceCurrency: ev.currency || 'RUB',
      availability,
      url: abs(ev.external_registration_url || url)
    };
  } else if (typeof ev.price_min === 'number') {
    node.offers = {
      '@type': 'Offer',
      price: ev.price_min,
      priceCurrency: ev.currency || 'RUB',
      availability,
      url: abs(ev.external_registration_url || url)
    };
  }

  if (ev.organizer_name) {
    node.organizer = { '@type': 'Organization', name: ev.organizer_name };
  }
  return node;
}

/** Список событий раздела — им поисковик строит карусель мероприятий. */
export function itemListJsonLd(events: PublicEvent[], origin: string): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'ItemList',
    itemListElement: events.slice(0, 50).map((ev, i) => ({
      '@type': 'ListItem',
      position: i + 1,
      url: eventUrl(origin, ev),
      name: ev.title
    }))
  };
}

export function breadcrumbJsonLd(
  origin: string,
  crumbs: { name: string; url: string }[]
): Record<string, unknown> {
  return {
    '@context': 'https://schema.org',
    '@type': 'BreadcrumbList',
    itemListElement: crumbs.map((c, i) => ({
      '@type': 'ListItem',
      position: i + 1,
      name: c.name,
      item: c.url
    }))
  };
}

/**
 * Готовое содержимое <script type="application/ld+json">.
 *
 * Экранирование обязательно: заголовок события — чужой текст, и строка
 * «</script>» внутри него закрыла бы тег и превратила остаток разметки в
 * исполняемый HTML. Это не гипотетика — заголовки едут из телеграма.
 */
export function jsonLdScript(node: Record<string, unknown>): string {
  return JSON.stringify(node)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026');
}
