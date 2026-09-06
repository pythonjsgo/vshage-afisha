/**
 * Разделы афиши: словарь категорий ленты 0.7 плюс три временны́х/ценовых
 * раздела. Один список на весь фронт — по нему строятся и плитка, и адрес,
 * и заголовок страницы, и карта сайта.
 *
 * Слаг РАВЕН коду словаря намеренно. Отдельный «красивый» слаг (theatre
 * вместо theatre_cinema) был бы ЧЕТВЁРТОЙ копией таксономии — три уже есть
 * (core-api internal/feed/taxonomy.go, кабинет lib/event-category.ts,
 * бэкенд афиши internal/tgevents/models.go), и каждая новая копия однажды
 * разъезжается. Русские ключевые слова живут в <h1> и <title>, а не в пути:
 * поисковику важнее заголовок, чем транслит в адресе.
 *
 * Пополняется ВМЕСТЕ со словарём ленты, одним заходом — как и остальные три.
 */

export type SectionKind = 'category' | 'when' | 'price';

export interface Section {
  /** Последний сегмент адреса: /msk/<slug>. */
  slug: string;
  /** Что уходит в API: category=<code> либо when=<code> либо free=1. */
  kind: SectionKind;
  code: string;
  /** Подпись на плитке и в хлебных крошках. */
  tile: string;
  /** Именительный во множественном для <h1>: «Концерты в Москве». */
  heading: string;
  /** Хвост описания для <meta name="description">. */
  blurb: string;
}

/** Порядок канонический — совпадает со словарём ленты, плитка рисует как есть. */
export const CATEGORY_SECTIONS: Section[] = [
  { slug: 'concert',        kind: 'category', code: 'concert',        tile: 'Концерты',      heading: 'Концерты',            blurb: 'живые выступления, клубы и залы' },
  { slug: 'party',          kind: 'category', code: 'party',          tile: 'Вечеринки',     heading: 'Вечеринки',           blurb: 'танцы, диджеи и открытые сеты' },
  { slug: 'lecture',        kind: 'category', code: 'lecture',        tile: 'Лекции',        heading: 'Лекции',              blurb: 'наука, дискуссии и книжные клубы' },
  { slug: 'workshop',       kind: 'category', code: 'workshop',       tile: 'Мастер-классы', heading: 'Мастер-классы',       blurb: 'практикумы, интенсивы и лаборатории' },
  { slug: 'exhibition',     kind: 'category', code: 'exhibition',     tile: 'Выставки',      heading: 'Выставки',            blurb: 'музеи, галереи и арт-пространства' },
  { slug: 'market',         kind: 'category', code: 'market',         tile: 'Маркеты',       heading: 'Маркеты и ярмарки',   blurb: 'ярмарки, барахолки и гастрофесты' },
  { slug: 'sport',          kind: 'category', code: 'sport',          tile: 'Спорт',         heading: 'Спорт',               blurb: 'турниры, забеги и тренировки' },
  { slug: 'theatre_cinema', kind: 'category', code: 'theatre_cinema', tile: 'Театр и кино',  heading: 'Театр и кино',        blurb: 'спектакли, кинопоказы и премьеры' },
  { slug: 'networking',     kind: 'category', code: 'networking',     tile: 'Нетворкинг',    heading: 'Нетворкинг',          blurb: 'конференции, питчи и деловые встречи' },
  { slug: 'excursion',      kind: 'category', code: 'excursion',      tile: 'Экскурсии',     heading: 'Экскурсии',           blurb: 'прогулки по городу и маршруты' },
  { slug: 'campus',         kind: 'category', code: 'campus',         tile: 'Кампус',        heading: 'Студенческие события', blurb: 'вузы, студсоветы и посвящения' },
  { slug: 'family',         kind: 'category', code: 'family',         tile: 'Для семьи',     heading: 'События для семьи',   blurb: 'детские и семейные программы' },
  { slug: 'dating',         kind: 'category', code: 'dating',         tile: 'Знакомства',    heading: 'Знакомства',          blurb: 'вечера знакомств и свидания' },
  { slug: 'nightlife',      kind: 'category', code: 'nightlife',      tile: 'Ночная жизнь',  heading: 'Ночная жизнь',        blurb: 'ночные программы и бар-хопы' },
  { slug: 'spiritual',      kind: 'category', code: 'spiritual',      tile: 'Духовное',      heading: 'Духовные события',    blurb: 'храмы, службы и паломничества' },
  { slug: 'health',         kind: 'category', code: 'health',         tile: 'Здоровье',      heading: 'Здоровье',            blurb: 'йога, психология и ретриты' },
  { slug: 'other',          kind: 'category', code: 'other',          tile: 'Другое',        heading: 'Другие события',      blurb: 'всё, что не попало в остальные разделы' }
];

/**
 * Разделы по времени и цене. Слаги здесь транслитом, а коды категорий —
 * латиницей из словаря, поэтому пространство имён одного сегмента общее и
 * столкнуться они не могут по построению.
 */
export const EXTRA_SECTIONS: Section[] = [
  { slug: 'segodnya',  kind: 'when',  code: 'today',    tile: 'Сегодня',    heading: 'Куда сходить сегодня', blurb: 'события, которые идут прямо сегодня' },
  { slug: 'zavtra',    kind: 'when',  code: 'tomorrow', tile: 'Завтра',     heading: 'Куда сходить завтра',  blurb: 'события завтрашнего дня' },
  { slug: 'vyhodnye',  kind: 'when',  code: 'weekend',  tile: 'Выходные',   heading: 'Куда сходить на выходных', blurb: 'программа ближайших субботы и воскресенья' },
  { slug: 'besplatno', kind: 'price', code: 'free',     tile: 'Бесплатно',  heading: 'Бесплатные события',   blurb: 'вход свободный, без билета' }
];

export const ALL_SECTIONS: Section[] = [...EXTRA_SECTIONS, ...CATEGORY_SECTIONS];

/**
 * Индекс по слагу — и заодно проверка того, что список действительно
 * непротиворечив.
 *
 * `new Map` дубль проглатывает молча: второй раздел с тем же слагом просто
 * вытеснил бы первый, и страница показывала бы события чужой категории под
 * верным заголовком. Отказ был бы немым — сайт отвечает 200, разметка
 * валидна, неправ только состав. Поэтому падаем громко и на сборке: дубль в
 * этом списке может появиться только правкой кода, а такую правку ловит
 * ближайший рендер на DEV, а не человек, читающий выдачу.
 */
const BY_SLUG = new Map<string, Section>();
for (const s of ALL_SECTIONS) {
  if (BY_SLUG.has(s.slug)) {
    throw new Error(`taxonomy: слаг раздела «${s.slug}» объявлен дважды`);
  }
  BY_SLUG.set(s.slug, s);
}

/** Раздел по слагу адреса; undefined — повод отдать 404, а не пустую ленту. */
export function sectionBySlug(slug: string): Section | undefined {
  return BY_SLUG.get(slug);
}

/** Подпись категории для карточки и хлебных крошек. */
export function categoryLabel(code: string | null | undefined): string | undefined {
  if (!code) return undefined;
  return CATEGORY_SECTIONS.find((s) => s.code === code)?.tile;
}

/** Параметры запроса к /api/events для раздела. */
export function sectionQuery(s: Section): Record<string, string> {
  if (s.kind === 'category') return { category: s.code };
  if (s.kind === 'when') return { when: s.code };
  return { free: '1' };
}

/**
 * Русское склонение числительного: plural(3, 'событие','события','событий').
 * Нужно в заголовках и описаниях («417 событий», «1 событие») — строка с
 * неверным окончанием в <title> читается как машинная и снижает кликабельность.
 */
export function plural(n: number, one: string, few: string, many: string): string {
  const mod100 = n % 100;
  if (mod100 >= 11 && mod100 <= 14) return many;
  const mod10 = n % 10;
  if (mod10 === 1) return one;
  if (mod10 >= 2 && mod10 <= 4) return few;
  return many;
}
