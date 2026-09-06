import { describe, it, expect } from 'vitest';
import { eventJsonLd, jsonLdScript, eventUrl, eventsCount } from './seo';
import { plural } from './taxonomy';
import type { PublicEvent } from './types';

/**
 * Прибор на разметку для поисковика.
 *
 * Почему на неё вообще нужен тест. Разметка schema.org — единственная часть
 * страницы, которую НЕ видит человек: она уезжает роботу, и ошибка в ней не
 * ломает ни вёрстку, ни ответ 200. Обратная связь приходит месяцами и в форме
 * «нас нет в блоке мероприятий», а неверная разметка снимает сайт с показа
 * целиком — то есть отказ немой в обе стороны. Значит утверждать её должен
 * прибор, а не чтение кода.
 *
 * Каждый случай ниже — это то, чем мы можем СОВРАТЬ роботу от своего имени
 * про чужое событие: выдумать время, выдумать бесплатный вход, увести на
 * другой адрес. Именно поэтому они здесь, а не «для покрытия».
 *
 * ВАЖНО про запуск: `vitest.config.ts` собирает только `tests/unit/**`, а этот
 * файл лежит рядом с модулем (так его задал контракт зоны). Чтобы он попал в
 * `npm run test:unit`, в `include` нужна вторая строка
 * `'src/**\/*.{test,spec}.{js,ts}'` — иначе тест не выполняется вовсе, а
 * невыполненный тест хуже отсутствующего: он выглядит защитой.
 */

const ORIGIN = 'https://afisha.vshage.app';

/** Минимальное событие доски; каждый тест дописывает только то, что мерит. */
function ev(over: Partial<PublicEvent> = {}): PublicEvent {
  return {
    id: '11111111-2222-3333-4444-555555555555',
    title: 'Открытая лекция про город',
    start_time: '2026-09-12T19:00:00+03:00',
    tags: [],
    attendee_count: 0,
    status: 'published',
    is_featured: false,
    ...over
  };
}

describe('eventJsonLd — startDate', () => {
  // У 40% доски времени начала нет вовсе (импорт из телеграма даёт только
  // дату). Полночь в разметке робот покажет человеку как факт «начало в
  // 00:00» — это выдуманное время на чужом событии.
  it('без известного времени отдаёт голую дату', () => {
    const node = eventJsonLd(ev({ start_time_known: false }), ORIGIN);
    expect(node.startDate).toBe('2026-09-12');
    expect(String(node.startDate)).not.toContain('T');
  });

  it('с известным временем отдаёт полную отметку со смещением', () => {
    const node = eventJsonLd(ev({ start_time_known: true }), ORIGIN);
    expect(node.startDate).toBe('2026-09-12T19:00:00+03:00');
  });

  // Признак необязательный: события, заведённые до его появления, приезжают
  // без него. Отсутствие означает «время известно» — так их и печатает сайт.
  it('без признака ведёт себя как «время известно»', () => {
    const node = eventJsonLd(ev(), ORIGIN);
    expect(node.startDate).toBe('2026-09-12T19:00:00+03:00');
  });
});

describe('jsonLdScript — экранирование', () => {
  const title = 'Лекция </script><img src=x onerror=alert(1)> про город';

  it('строка «</script>» в заголовке не закрывает тег', () => {
    const out = jsonLdScript(eventJsonLd(ev({ title }), ORIGIN));
    expect(out).not.toContain('</script>');
    // Ни одной угловой скобки: закрыть тег нечем в принципе, а не «нет
    // конкретно этой последовательности».
    expect(out).not.toContain('<');
    expect(out).not.toContain('>');
  });

  it('экранирование обратимо — робот читает исходный заголовок', () => {
    const out = jsonLdScript(eventJsonLd(ev({ title }), ORIGIN));
    expect(JSON.parse(out).name).toBe(title);
  });
});

describe('eventJsonLd — offers', () => {
  // Цена неизвестна у большинства импортированных карточек. Подставить 0 —
  // значит от нашего имени пообещать бесплатный вход на чужое событие.
  it('не появляются, когда цена неизвестна', () => {
    expect(eventJsonLd(ev(), ORIGIN).offers).toBeUndefined();
  });

  it('не появляются у платного события без нижней цены', () => {
    expect(eventJsonLd(ev({ price_type: 'paid' }), ORIGIN).offers).toBeUndefined();
  });

  it('нулевая цена — только когда вход объявлен свободным', () => {
    const offers = eventJsonLd(ev({ price_type: 'free' }), ORIGIN).offers as Record<string, unknown>;
    expect(offers.price).toBe(0);
    expect(offers.priceCurrency).toBe('RUB');
  });

  it('цена берётся из price_min, когда она есть', () => {
    const offers = eventJsonLd(ev({ price_type: 'paid', price_min: 800 }), ORIGIN).offers as Record<
      string,
      unknown
    >;
    expect(offers.price).toBe(800);
  });
});

describe('eventUrl — канонический адрес', () => {
  // Три маршрута ведут на одну карточку (/<id>, /events/<id> → 308,
  // /e/<slug>). Без единого источника адреса поисковик считает их разными
  // страницами с одинаковым текстом и делит вес между ними.
  it('веб-регистрация ведёт на /e/<slug>', () => {
    expect(eventUrl(ORIGIN, { id: 'mavantura', webreg_slug: 'mavantura' })).toBe(
      `${ORIGIN}/e/mavantura`
    );
  });

  it('остальные события — на /<id>', () => {
    expect(eventUrl(ORIGIN, { id: 'ev_09333abc' })).toBe(`${ORIGIN}/ev_09333abc`);
  });

  it('пустой слаг не превращается в /e/', () => {
    expect(eventUrl(ORIGIN, { id: 'ev_09333abc', webreg_slug: '' })).toBe(`${ORIGIN}/ev_09333abc`);
  });
});

describe('plural — окончание считается, а не приклеивается', () => {
  it.each([
    [1, '1 событие'],
    [2, '2 события'],
    [5, '5 событий'],
    [11, '11 событий'],
    [21, '21 событие'],
    [114, '114 событий'],
    [402, '402 события']
  ])('%i → %s', (n, expected) => {
    expect(eventsCount(n)).toBe(expected);
  });

  it('работает и на других словах', () => {
    expect(plural(1, 'раздел', 'раздела', 'разделов')).toBe('раздел');
    expect(plural(3, 'раздел', 'раздела', 'разделов')).toBe('раздела');
    expect(plural(21, 'раздел', 'раздела', 'разделов')).toBe('раздел');
  });
});

/* ─────────────────────────────────────────────────────────────────────────
   Разбор ревью зоны Z3 по этому же модулю: четыре дефекта, найденные в нём
   при сборке разметки. Тесты написаны ПОСЛЕ правок и обязаны падать на
   старом коде — проверено подменой каждой правки обратно.
   ───────────────────────────────────────────────────────────────────────── */

import { mskDate } from './seo';
import { ALL_SECTIONS, sectionBySlug, categoryLabel } from './taxonomy';

describe('eventJsonLd — онлайн размечается своим типом места', () => {
  // Пометить событие онлайновым, оставив физический Place, — невалидная
  // разметка: Google отбрасывает такое событие целиком. Отказ немой и
  // выглядит как «нас просто не показывают».
  it('только онлайн — VirtualLocation и Online-режим', () => {
    const n = eventJsonLd(ev({ online_url: 'https://meet.vshage.app/x' }), ORIGIN) as any;
    expect(n.location['@type']).toBe('VirtualLocation');
    expect(n.eventAttendanceMode).toBe('https://schema.org/OnlineEventAttendanceMode');
  });

  it('онлайн и площадка вместе — оба места и Mixed-режим', () => {
    const n = eventJsonLd(
      ev({ online_url: 'https://meet.vshage.app/x', venue_name: 'Депо' }),
      ORIGIN
    ) as any;
    expect(n.location.map((l: any) => l['@type'])).toEqual(['Place', 'VirtualLocation']);
    expect(n.eventAttendanceMode).toBe('https://schema.org/MixedEventAttendanceMode');
  });

  it('обычное событие остаётся офлайновым Place', () => {
    const n = eventJsonLd(ev({ venue_name: 'Депо' }), ORIGIN) as any;
    expect(n.location['@type']).toBe('Place');
    expect(n.eventAttendanceMode).toBe('https://schema.org/OfflineEventAttendanceMode');
  });
});

describe('eventJsonLd — адреса в разметке абсолютные', () => {
  // Относительный адрес в offers робот прочитает от своего корня. Ссылка на
  // запись уехала бы в никуда, а мы бы об этом не узнали.
  it('offers.url абсолютен, даже когда источник дал относительный', () => {
    const n = eventJsonLd(
      ev({ price_type: 'free', external_registration_url: '/e/mavantura' }),
      ORIGIN
    ) as any;
    expect(n.offers.url).toBe(`${ORIGIN}/e/mavantura`);
  });

  it('уже абсолютный адрес не удваивается', () => {
    const n = eventJsonLd(
      ev({ price_min: 500, external_registration_url: 'https://timepad.ru/e/1' }),
      ORIGIN
    ) as any;
    expect(n.offers.url).toBe('https://timepad.ru/e/1');
  });
});

describe('eventUrl — слаг из чужой формы кодируется', () => {
  it('пробел в слаге не рвёт адрес', () => {
    expect(eventUrl(ORIGIN, { id: 'x', webreg_slug: 'моё меро' })).toBe(
      `${ORIGIN}/e/${encodeURIComponent('моё меро')}`
    );
  });

  it('вопросительный знак не превращается в query', () => {
    const u = eventUrl(ORIGIN, { id: 'x', webreg_slug: 'a?b' });
    expect(u.includes('?')).toBe(false);
  });
});

describe('mskDate — дата считается по Москве, а не срезом строки', () => {
  // Полночь 12-го по Москве — это 21:00 11-го по Гринвичу. Срез первых
  // десяти знаков у источника, отдающего UTC, ошибается ровно на сутки.
  it('момент в UTC приводится к московской дате', () => {
    expect(mskDate('2026-09-11T21:00:00Z')).toBe('2026-09-12');
  });

  it('момент со смещением +03:00 остаётся собой', () => {
    expect(mskDate('2026-09-12T00:00:00+03:00')).toBe('2026-09-12');
  });

  it('вечернее время не уезжает на следующий день', () => {
    expect(mskDate('2026-09-12T19:00:00+03:00')).toBe('2026-09-12');
  });
});

describe('taxonomy — список разделов непротиворечив', () => {
  // `new Map` дубль проглатывает молча: раздел показывал бы чужие события
  // под верным заголовком, отвечая при этом 200.
  it('все слаги уникальны', () => {
    const slugs = ALL_SECTIONS.map((s) => s.slug);
    expect(new Set(slugs).size).toBe(slugs.length);
  });

  it('каждый слаг находится обратно', () => {
    for (const s of ALL_SECTIONS) expect(sectionBySlug(s.slug)?.code).toBe(s.code);
  });

  it('неизвестный слаг не находится — это и есть 404', () => {
    expect(sectionBySlug('nesushchestvuet')).toBeUndefined();
  });
});

describe('eventJsonLd — сортировочный сдвиг не уезжает в разметку', () => {
  // Идущая многодневная выставка показывается в ленте как сегодняшняя, иначе
  // она утонула бы в июне. Человеку это читается верно; роботу, отданное как
  // startDate, это сообщает «выставка начинается сегодня» — заново каждый
  // день. Мы бы от своего имени утверждали ложный факт о чужом событии.
  it('startDate берётся из настоящей даты начала, а не из сдвинутой', () => {
    const n = eventJsonLd(
      ev({
        start_time: '2026-09-06T00:00:00+03:00',
        actual_start_date: '2026-06-14',
        start_time_known: false,
        source: 'tg'
      }),
      ORIGIN
    ) as any;
    expect(n.startDate).toBe('2026-06-14');
  });

  it('без сдвига поведение прежнее', () => {
    const n = eventJsonLd(ev({ start_time_known: false }), ORIGIN) as any;
    expect(n.startDate).toBe('2026-09-12');
  });

  it('конец импортированной программы — дата, а не 23:59', () => {
    const n = eventJsonLd(
      ev({ source: 'tg', end_time: '2026-09-30T23:59:00+03:00' }),
      ORIGIN
    ) as any;
    expect(n.endDate).toBe('2026-09-30');
  });

  it('у нашего события конец остаётся моментом', () => {
    const n = eventJsonLd(ev({ end_time: '2026-09-12T22:00:00+03:00' }), ORIGIN) as any;
    expect(n.endDate).toBe('2026-09-12T22:00:00+03:00');
  });
});

describe('categoryLabel — на экран не попадает код словаря', () => {
  // Регрессия 06.09: карточка печатала `event.category.toUpperCase()`, и как
  // только категория поехала наружу из tgevents, на чипе появилось «CAMPUS».
  // Тот же класс, что сырой ключ `feed.category.campus` в ленте приложения.
  it('код превращается в человеческую подпись', () => {
    expect(categoryLabel('campus')).toBe('Кампус');
    expect(categoryLabel('theatre_cinema')).toBe('Театр и кино');
  });

  it('неизвестный код не подписывается вовсе — пусто честнее идентификатора', () => {
    expect(categoryLabel('meetup')).toBeUndefined();
    expect(categoryLabel('')).toBeUndefined();
    expect(categoryLabel(null)).toBeUndefined();
  });
});
