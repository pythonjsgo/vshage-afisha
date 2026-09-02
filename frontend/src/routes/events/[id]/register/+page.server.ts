import { error, fail } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import type { PublicEvent } from '$lib/types';

type RegistrationErrorBody = {
  code?: string;
  message?: string;
  error?: string;
  external_registration_url?: string;
  /** {"<имя поля>": "текст ошибки"} — приходит вместе с code=invalid_form. */
  fields?: Record<string, string>;
};

type RegistrationResultBody = {
  status?: string;
  already_registered?: boolean;
};

/** Свободный вариант в выпадающем списке с allow_other. */
const OTHER_OPTION = 'Другое';

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';

export const load: PageServerLoad = async ({ params, fetch }) => {
  const res = await fetch(`${backendURL()}/api/events/${encodeURIComponent(params.id)}`);
  if (res.status === 404) throw error(404, 'Событие не найдено');
  if (!res.ok) throw error(500, 'Сервер недоступен');
  const event = (await res.json()) as PublicEvent;
  return { event };
};

export const actions: Actions = {
  default: async ({ request, params, fetch }) => {
    const form = await request.formData();
    const field = (key: string) => String(form.get(key) ?? '').trim();

    // Свои вопросы приезжают как answer:<key>; у select с allow_other выбор
    // «Другое» дополняется свободным вводом в answer_other:<key>.
    //
    // Разложено на две записи не для красоты: серверу уезжает РАЗОБРАННЫЙ
    // ответ («Увидел афишу»), а форме надо вернуть ВЫБРАННЫЙ («Другое») —
    // иначе после неудачной отправки список не находит своё значение среди
    // вариантов и человек видит пустой выпадающий список без своего текста.
    const answers: Record<string, string> = {};
    const pickedAnswers: Record<string, string> = {};
    const answersOther: Record<string, string> = {};
    for (const [k, v] of form.entries()) {
      if (!k.startsWith('answer:')) continue;
      const key = k.slice('answer:'.length);
      const value = String(v).trim();
      // Неотмеченный чекбокс в FormData не приходит вовсе — ключа не будет,
      // и это ровно то, чего ждёт сервер.
      if (!value) continue;
      pickedAnswers[key] = value;
      if (value === OTHER_OPTION) {
        const other = String(form.get(`answer_other:${key}`) ?? '').trim();
        answersOther[key] = other;
        if (other) answers[key] = other;
      } else {
        answers[key] = value;
      }
    }

    const payload = {
      name: field('name'),
      full_name: field('full_name'),
      email: field('email'),
      phone: field('phone'),
      tg_username: field('tg_username'),
      contact: field('contact'),
      answers
    };

    // Возвращаем обратно, чтобы неудачная отправка не стирала набранное.
    const values = { ...payload, answers: pickedAnswers, answers_other: answersOther };

    const res = await fetch(`${backendURL()}/api/events/${encodeURIComponent(params.id)}/registrations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });

    const body = (await res.json().catch(() => ({}))) as RegistrationErrorBody & RegistrationResultBody;
    if (!res.ok) {
      return fail(res.status, {
        values,
        code: body.code,
        fields: (body.fields ?? {}) as Record<string, string>,
        error: body.message ?? body.error ?? 'Не удалось отправить регистрацию',
        external_registration_url: body.external_registration_url
      });
    }

    return {
      success: true,
      status: body.status,
      already_registered: body.already_registered
    };
  }
};
