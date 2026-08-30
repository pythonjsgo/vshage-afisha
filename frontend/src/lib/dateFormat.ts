const MONTHS_SHORT = ['ЯНВ','ФЕВ','МАР','АПР','МАЯ','ИЮН','ИЮЛ','АВГ','СЕН','ОКТ','НОЯ','ДЕК'];

/**
 * Афиша московская, а рисуется в двух местах с разными часовыми поясами:
 * SSR-контейнер живёт в UTC (TZ ему не задан), браузер посетителя — где
 * угодно. Прежняя реализация читала getHours()/getDate() в зоне рантайма,
 * поэтому событие «1 сентября 00:00 МСК» уходило в разметку как «31 АВГ ·
 * 21:00» — и именно эту разметку видят краулер, превью в телеге и первый
 * кадр до гидрации, а после гидрации текст менялся на глазах.
 *
 * Зона задана явно и одна на всё: MSK.
 */
const MSK = 'Europe/Moscow';

type Parts = { year: number; month: number; day: number; hour: number; minute: number };

function mskParts(d: Date): Parts {
  const f = new Intl.DateTimeFormat('en-GB', {
    timeZone: MSK, year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false
  });
  const p: Record<string, string> = {};
  for (const { type, value } of f.formatToParts(d)) p[type] = value;
  return {
    year: Number(p.year), month: Number(p.month), day: Number(p.day),
    hour: Number(p.hour === '24' ? '0' : p.hour), minute: Number(p.minute)
  };
}

function sameDay(a: Parts, b: Parts) {
  return a.year === b.year && a.month === b.month && a.day === b.day;
}

function hhmm(p: Parts) {
  return `${String(p.hour).padStart(2, '0')}:${String(p.minute).padStart(2, '0')}`;
}

/**
 * `timeKnown = false` означает «дата известна, времени нет» — у
 * импортированных анонсов это частый случай (у 7 карточек из 20 на замере
 * 30.08). Печатать там «00:00» значило бы выдумывать время: полночь в данных
 * это отсутствие времени, а не начало события ночью.
 */
export function formatEventDate(iso: string, now = new Date(), timeKnown = true): string {
  const d = mskParts(new Date(iso));
  const n = mskParts(now);
  const t = mskParts(new Date(now.getTime() + 24 * 60 * 60 * 1000));
  if (sameDay(d, n)) return timeKnown ? `СЕГОДНЯ В ${hhmm(d)}` : 'СЕГОДНЯ';
  if (sameDay(d, t)) return timeKnown ? `ЗАВТРА В ${hhmm(d)}` : 'ЗАВТРА';
  const date = `${d.day} ${MONTHS_SHORT[d.month - 1]}`;
  return timeKnown ? `${date} · ${hhmm(d)}` : date;
}

export function formatEventDateLong(iso: string, timeKnown = true): string {
  const d = mskParts(new Date(iso));
  const date = `${d.day} ${MONTHS_SHORT[d.month - 1]}`;
  return timeKnown ? `${date} · ${hhmm(d)}` : date;
}

/** «до 13 СЕН» — конец многодневной программы. Без него идущая выставка
 *  читается как разовое событие сегодняшнего дня. */
export function formatEndDate(iso: string): string {
  const d = mskParts(new Date(iso));
  return `до ${d.day} ${MONTHS_SHORT[d.month - 1]}`;
}
