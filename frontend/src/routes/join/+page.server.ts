import { fail, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { JOIN_DONE_PATH, readTracking, TRACKING_KEYS } from '$lib/join';
import type { Actions, PageServerLoad } from './$types';

// Тот же адрес и тот же таймаут, что у веб-регистрации (/e/[slug]): анкета
// уходит на afisha-backend по внутренней сети compose.
//
// Фолбэк на localhost внутри контейнера не работает НИКОГДА, поэтому о его
// использовании кричим в лог на старте: молчаливый фолбэк вместе с молчаливым
// catch давал бы полную тишину при неверной конфигурации.
const BACKEND = env.BACKEND_INTERNAL_URL || 'http://localhost:3004';
if (!env.BACKEND_INTERNAL_URL) {
  console.warn('join: BACKEND_INTERNAL_URL не задан, беру', BACKEND);
}
const SUBMIT_TIMEOUT_MS = 10_000;

/** Адрес посетителя, либо пусто. adapter-node БРОСАЕТ, если задан
 *  ADDRESS_HEADER (он задан в compose), а заголовка в запросе нет — и тогда
 *  отказ конфигурации прокси доложили бы человеку как «не дошло до сервера». */
function safeClientAddress(get: () => string): string {
  try {
    return get();
  } catch (e) {
    console.error('join: getClientAddress:', e);
    return '';
  }
}

export const load: PageServerLoad = ({ url, request }) => {
  return {
    tracking: readTracking(url),
    // Referer СЮДА приезжает только на первом GET — на POST'е им будет уже
    // сама страница. Поэтому источник перехода забираем здесь и везём
    // скрытым полем; без этого в базе у всех был бы один и тот же /join.
    referrer: (request.headers.get('referer') ?? '').slice(0, 400)
  };
};

type Values = Record<string, string | boolean>;

export const actions: Actions = {
  default: async ({ request, fetch, getClientAddress }) => {
    const data = await request.formData();
    const str = (k: string) => String(data.get(k) ?? '').trim();

    const payload = {
      name: str('name'),
      university: str('university'),
      university_other: str('university_other'),
      course: str('course'),
      about: str('about'),
      telegram: str('telegram'),
      consent: data.get('consent') === 'on' || data.get('consent') === 'true',
      // Honeypot. Поле есть в разметке, спрятано от человека; заполнено ⇒ бот.
      hp_note: str('hp_note'),
      referrer: str('referrer'),
      ...Object.fromEntries(TRACKING_KEYS.map((k) => [k, str(k)]))
    };

    // Возвращаем человеку ровно то, что он набрал: форма перерисовывается с
    // ошибкой, и пустые поля вместо набранного читаются как «всё стёрлось».
    // referrer здесь тоже НЕ случайно: без JS страница перезагружается, load
    // выполняется заново, и во второй попытке источником перехода стала бы
    // сама /join вместо Яндекса.
    const values: Values = {
      name: payload.name,
      university: payload.university,
      university_other: payload.university_other,
      course: payload.course,
      about: payload.about,
      telegram: payload.telegram,
      consent: payload.consent,
      referrer: payload.referrer
    };

    const address = safeClientAddress(getClientAddress);

    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), SUBMIT_TIMEOUT_MS);
    let res: Response;
    try {
      res = await fetch(`${BACKEND}/api/join`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          // Без этого заголовка бэкенд видит адрес SSR-контейнера, один на
          // всех посетителей, и per-IP лимитер перестаёт что-либо значить.
          'X-Forwarded-For': address,
          'User-Agent': request.headers.get('user-agent') ?? ''
        },
        body: JSON.stringify(payload),
        signal: ctrl.signal
      });
    } catch (e) {
      // Без этой строки поломка видна ТОЛЬКО посетителю: Go-сторона логирует
      // каждый свой отказ, а SSR-слой молчал бы и про лежащий бэкенд, и про
      // неверный BACKEND_INTERNAL_URL, и про таймаут.
      console.error('join: запрос к бэкенду не прошёл:', e);
      // fieldErrors пустой, но присутствует: у всех веток отказа одна форма
      // ответа, иначе страница разбирает то один тип, то другой.
      return fail(503, {
        values,
        code: 'network',
        fieldErrors: {} as Record<string, string>,
        error: 'Не дошло до сервера. Попробуй ещё раз через минуту.'
      });
    } finally {
      clearTimeout(timer);
    }

    if (!res.ok) {
      const raw = await res.text();
      let body: Record<string, unknown> = {};
      try {
        body = JSON.parse(raw) as Record<string, unknown>;
      } catch {
        // Не-JSON от прокси (502/504) — отдельная поломка, и её причина
        // должна остаться в логе, а не превратиться в «Проверь поля».
        console.error(`join: бэкенд ответил ${res.status} не-JSON:`, raw.slice(0, 300));
      }
      if (res.status >= 500) {
        console.error(`join: бэкенд ответил ${res.status}`, body.code ?? '');
      }
      return fail(res.status, {
        values,
        code: String(body.code ?? 'error'),
        fieldErrors: (body.fields ?? {}) as Record<string, string>,
        error: String(body.message ?? 'Не получилось отправить анкету.')
      });
    }

    // Отдельный URL, а не «спасибо» внутри той же страницы: под цель Метрики
    // по посещению страницы нужен адрес, и он же переживает перезагрузку.
    redirect(303, JOIN_DONE_PATH);
  }
};
