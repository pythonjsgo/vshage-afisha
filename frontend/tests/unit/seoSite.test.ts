import { describe, it, expect } from 'vitest';
import { siteJsonLd, citySlugByName, eventCrumbs } from '../../src/lib/seo';

const ORIGIN = 'https://afisha.vshage.app';

describe('разметка сайта', () => {
  it('объявляет WebSite с издателем и языком', () => {
    const n = siteJsonLd(ORIGIN);
    expect(n['@type']).toBe('WebSite');
    expect(n.inLanguage).toBe('ru-RU');
    const pub = n.publisher as Record<string, unknown>;
    expect(pub['@type']).toBe('Organization');
    expect((pub.logo as Record<string, unknown>).url).toBe(`${ORIGIN}/icon-512.png`);
  });

  // Поиска на сайте нет. SearchAction — это адрес, по которому Google ХОДИТ:
  // объявив его, мы бы получили строку поиска в выдаче, ведущую в 404.
  it('НЕ объявляет поиск, которого на сайте нет', () => {
    expect(JSON.stringify(siteJsonLd(ORIGIN))).not.toContain('SearchAction');
  });
});

describe('слаг города по имени', () => {
  it('знает Москву в любом регистре и с пробелами', () => {
    expect(citySlugByName('Москва')).toBe('msk');
    expect(citySlugByName('  москва ')).toBe('msk');
  });
  it('незнакомый город честно молчит, а не выдумывает адрес', () => {
    for (const x of ['Казань', '', null, undefined]) {
      expect(citySlugByName(x as string)).toBeUndefined();
    }
  });
});

describe('крошки карточки', () => {
  const ev = { title: 'Выставка «Бабочки»', city: 'Москва', category: 'exhibition' };
  const canonical = `${ORIGIN}/ev_abc123`;

  it('строит цепочку город → раздел → событие', () => {
    expect(eventCrumbs(ORIGIN, ev, canonical)).toEqual([
      { name: 'Афиша Москвы', url: `${ORIGIN}/msk` },
      { name: 'Выставки', url: `${ORIGIN}/msk/exhibition` },
      { name: 'Выставка «Бабочки»', url: canonical }
    ]);
  });

  it('без рубрики оставляет город и событие', () => {
    const c = eventCrumbs(ORIGIN, { ...ev, category: null }, canonical);
    expect(c.map((x) => x.name)).toEqual(['Афиша Москвы', 'Выставка «Бабочки»']);
  });

  // Код вне словаря приезжает из конвейера. Ссылка на /msk/<такой-код> — 404,
  // то есть крошка вела бы робота и человека в никуда.
  it('код вне словаря раздела не даёт — ссылки в никуда быть не должно', () => {
    const c = eventCrumbs(ORIGIN, { ...ev, category: 'нет-такого' }, canonical);
    expect(c.map((x) => x.name)).toEqual(['Афиша Москвы', 'Выставка «Бабочки»']);
  });

  it('незнакомый город убирает крошки целиком — цепочка из одного звена это шум', () => {
    expect(eventCrumbs(ORIGIN, { ...ev, city: 'Казань' }, canonical)).toEqual([]);
    expect(eventCrumbs(ORIGIN, { ...ev, city: null }, canonical)).toEqual([]);
  });
});
