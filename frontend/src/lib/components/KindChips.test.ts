// @vitest-environment node
//
// Компонент рисуется НА СЕРВЕРЕ (`svelte/server`), а не в jsdom: разметка —
// это ровно то, что уезжает человеку и роботу, и в одной строке видны обе
// половины обещания сразу: число и адрес, по которому его можно проверить.
// Докблок среды стоит здесь, а не в конфиге: остальному прогону нужен jsdom.
import { describe, it, expect } from 'vitest';
import { render } from 'svelte/server';
import KindChips from './KindChips.svelte';

/**
 * Прибор на переключатель полос.
 *
 * Мерится одно: число на пилюле и страница, куда пилюля ведёт, — это ОДНА И
 * ТА ЖЕ страница. Пока числа брались из фасетов, они были числами по всему
 * городу, а адрес нёс раздел: на `/msk/concert` пилюля обещала «ИДЁТ СЕЙЧАС ·
 * 88» и открывала пустую сетку. Отказ немой — код 200, ни строки в логах,
 * нарушено только обещание тому, кто нажал. Это тот же класс, что подпись
 * «СОБЫТИЯ · 90 ИЗ 150»: число рядом с элементом — обещание содержимого.
 */

interface WPill {
  href: string;
  label: string;
  /** null — цифры на пилюле нет вовсе (а не ноль). */
  count: number | null;
}

/** Пилюли из разметки: адрес, подпись и число — в том виде, в каком уедут. */
function wPills(html: string): WPill[] {
  return [...html.matchAll(/<a\b[^>]*\shref="([^"]*)"[^>]*>([\s\S]*?)<\/a>/g)].map((m) => {
    const inner = m[2];
    const n = inner.match(/<span class="n[^"]*">([^<]*)<\/span>/)?.[1];
    return {
      href: m[1],
      label: inner.match(/<span>([^<]*)<\/span>/)?.[1] ?? '',
      count: n === undefined ? null : Number(n)
    };
  });
}

function wRender(props: {
  basePath: string;
  counts: { timed: number; running: number } | null;
  active?: 'running' | 'timed' | null;
  showCounts?: boolean;
}): WPill[] {
  return wPills(render(KindChips, { props }).body);
}

describe('KindChips — число обещает ту страницу, на которую ведёт', () => {
  it('в разделе рисуются числа раздела, а не города', () => {
    // Раздел «Концерты»: по дате и времени 34, идущих нет ни одного. Числа
    // города (88 идущих) сюда попасть не могут — по этому адресу они не
    // откроются.
    expect(wRender({ basePath: '/msk/concert', counts: { timed: 34, running: 0 } })).toEqual([
      { href: '/msk/concert?kind=timed', label: 'ПО ДАТЕ И ВРЕМЕНИ', count: 34 },
      { href: '/msk/concert?kind=running', label: 'ИДЁТ СЕЙЧАС', count: 0 }
    ]);
  });

  it('обе полосы пусты — ряда нет вовсе', () => {
    // У пустого раздела переключать нечего, а два нуля над «ПОКА ПУСТО»
    // ничего не сообщают.
    expect(wRender({ basePath: '/msk/concert', counts: { timed: 0, running: 0 } })).toEqual([]);
  });

  // СТОРОЖ (был зелёным и до правки): ряд обязан остаться дорогой обратно,
  // когда чисел нет. Пустая выбранная полоса — ровно этот случай: уйти из неё
  // больше нечем, кроме кнопки «назад» браузера.
  it('чисел нет — ряд без цифр, но с дорогой в соседнюю полосу', () => {
    const pills = wRender({ basePath: '/msk', counts: null, active: 'running' });
    expect(pills.map((p) => p.count)).toEqual([null, null]);
    // Активная пилюля ведёт на адрес без параметра: повторное нажатие снимает
    // полосу, иначе ссылка вела бы сама на себя.
    expect(pills.map((p) => p.href)).toEqual(['/msk?kind=timed', '/msk']);
  });

  // СТОРОЖ (был зелёным и до правки): правило §5.3 контракта — «число не
  // рисуется вовсе, когда showCounts === false». Молчащий источник занижает
  // счётчики, а заниженное число выглядит достоверным.
  it('счётчикам не верим — цифры не рисуются, ряд остаётся', () => {
    const pills = wRender({
      basePath: '/msk',
      counts: { timed: 34, running: 12 },
      showCounts: false
    });
    expect(pills.map((p) => p.count)).toEqual([null, null]);
    expect(pills.length).toBe(2);
  });
});
