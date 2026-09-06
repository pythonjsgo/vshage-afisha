import type { PublicEvent } from './types';
import { plural } from './taxonomy';

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

/* ------------------------------------------------------------------------ *
 *  Подпись «когда» — одна на все виды события
 * ------------------------------------------------------------------------ */

const DAY_MS = 24 * 60 * 60 * 1000;

/**
 * Номер календарного дня МСК, в который попадает момент.
 *
 * Считается по разобранным Y/M/D, а не вычитанием миллисекунд: разница двух
 * таких номеров — это ровно число календарных дней между датами, и она не
 * зависит ни от зоны рантайма, ни от перевода часов. Наивное
 * `(end - now) / 86400000` у события, кончающегося сегодня в 21:00, дало бы
 * ноль в 10 утра и единицу в 23:00 — «последний день» мигал бы в течение дня.
 */
function mskDayIndex(d: Date): number {
  const p = mskParts(d);
  return Date.UTC(p.year, p.month - 1, p.day) / DAY_MS;
}

function dayMonth(p: Parts): string {
  return `${p.day} ${MONTHS_SHORT[p.month - 1]}`;
}

/**
 * `actual_start_date` приезжает как «YYYY-MM-DD» — без времени и без зоны.
 * `new Date('2026-09-12')` разбирается как ПОЛНОЧЬ UTC, то есть 03:00 МСК: для
 * извлечения дня это ещё безопасно, но зависит от того, что смещение Москвы
 * положительное. Задаём зону явно и не полагаемся на удачу.
 */
function parseDay(s: string): Date {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) ? new Date(`${s}T00:00:00+03:00`) : new Date(s);
}

export interface WhenLabel {
  /** Готовая подпись в верхнем регистре. */
  text: string;
  /** true — срок поджимает: рисуется акцентом, а не обычным зелёным. */
  urgent: boolean;
}

/** Поля события, из которых собирается подпись. Больше подписи знать нечего. */
export type WhenSource = Pick<
  PublicEvent,
  | 'start_time'
  | 'end_time'
  | 'start_time_known'
  | 'actual_start_date'
  | 'kind'
  | 'multiday'
  | 'open_ended'
>;

/**
 * Подпись даты для карточки и для детальной страницы.
 *
 * Главный запрет: **«идёт с <дата>» не пишется никогда.** У идущих карточек
 * `date` — чаще дата ПОСТА, а не открытия программы: у биеннале, работающей с
 * марта, в базе стоит 5 сентября (замер 07.09). Подпись идущей программы
 * отвечает ровно на один вопрос — «до когда», — и только на него. Дата начала
 * появляется лишь там, где программа ещё НЕ открылась: там она пришла из
 * анонса и ей можно верить.
 *
 * Полосу не выводим из данных: её решает сервер полем `kind`. Наличие
 * `end_time` признаком не является — у концерта 19:00–22:00 конец есть, а
 * длится он один вечер.
 *
 * Время печатается там и только там, где оно и известно, и полезно: у
 * точечного события и у периода ВПЕРЕДИ. У идущей программы его нет — там
 * `start_time` сдвинут на сегодня ради сортировки, и час оттуда был бы
 * выдуманным.
 */
export function formatWhen(ev: WhenSource, now = new Date()): WhenLabel {
  // Точечное событие. Сюда же попадает СТАРЫЙ ответ бэкенда, где ни kind, ни
  // multiday ещё нет: подпись обязана остаться прежней, иначе фронт,
  // выкаченный раньше бэкенда, стёр бы дату у всей доски.
  if (!ev.multiday) {
    return {
      text: formatEventDate(ev.start_time, now, ev.start_time_known !== false),
      urgent: false
    };
  }

  if (ev.kind === 'running') {
    if (ev.open_ended) return { text: 'ИДЁТ ПОСТОЯННО', urgent: false };
    // Идущая программа без конца и без признака бессрочности структурно
    // невозможна: сервер объявляет running по условию «конец не раньше
    // сегодня», а его не проверить, не зная конца. Если такое всё же приедет
    // — говорим только то, что знаем точно, и не выдумываем срок.
    if (!ev.end_time) return { text: 'ИДЁТ СЕЙЧАС', urgent: false };
    const end = new Date(ev.end_time);
    // Отрицательное «осталось» означает, что сервер и даты разошлись. Тогда
    // «последний день» — самое осторожное из правдоподобного: оно не обещает
    // человеку времени, которого может не быть.
    const left = Math.max(0, mskDayIndex(end) - mskDayIndex(now));
    if (left === 0) return { text: 'ПОСЛЕДНИЙ ДЕНЬ', urgent: true };
    if (left <= 3) {
      return { text: `ОСТАЛОСЬ ${left} ${plural(left, 'ДЕНЬ', 'ДНЯ', 'ДНЕЙ')}`, urgent: true };
    }
    return { text: `ИДЁТ ДО ${dayMonth(mskParts(end))}`, urgent: false };
  }

  // Период, который ещё не начался. Здесь дата начала честная — она приехала
  // из анонса, а не из даты поста, и человеку нужна: он планирует.
  // `actual_start_date` перебивает `start_time` там, где тот сдвинут ради
  // сортировки.
  const start = mskParts(ev.actual_start_date ? parseDay(ev.actual_start_date) : new Date(ev.start_time));
  /**
   * Час начала остаётся в подписи периода, когда источник его дал.
   *
   * Замер по проду 07.09: 16 будущих многодневных карточек из первых двухсот
   * несут точное время — спектакль в 20:00, показы в 19:30, концерт в 19:00.
   * Это ровно те события, ради которых полоса «по дате и времени» и заведена,
   * и голый диапазон «8 — 9 СЕН» стирал с них час, то есть ломал
   * ориентирование той же строкой, которая его чинила. Для длинной серии час
   * означает начало сеанса — ближе к правде, чем молчание.
   *
   * Берётся из `start_time`, а НЕ из `start`: датой могла оказаться
   * `actual_start_date` — там сутки без времени, и час вышел бы «00:00».
   * Признака нет вовсе у событий кабинета — «не сказано, что времени нет»
   * читается как «время есть», как и везде в этом файле.
   */
  const at =
    ev.start_time_known !== false ? ` · ${hhmm(mskParts(new Date(ev.start_time)))}` : '';
  if (ev.open_ended) return { text: `С ${dayMonth(start)}${at}, ПОСТОЯННО`, urgent: false };
  // Многодневное без конца и без бессрочности — тоже противоречие в данных.
  // Печатаем то, что знаем: дату начала обычной подписью.
  if (!ev.end_time) {
    return {
      text: formatEventDate(ev.start_time, now, ev.start_time_known !== false),
      urgent: false
    };
  }
  const end = mskParts(new Date(ev.end_time));
  // Месяц пишется один раз только когда он ДЕЙСТВИТЕЛЬНО один — сравнение
  // года обязательно: у годичной программы «15 — 20 СЕН» читалось бы как
  // пять дней.
  if (start.year === end.year && start.month === end.month) {
    return { text: `${start.day} — ${dayMonth(end)}${at}`, urgent: false };
  }
  return { text: `${dayMonth(start)} — ${dayMonth(end)}${at}`, urgent: false };
}
