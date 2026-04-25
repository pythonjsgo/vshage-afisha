import { error, fail } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import type { PublicEvent } from '$lib/types';

type RegistrationErrorBody = {
  code?: string;
  message?: string;
  error?: string;
  external_registration_url?: string;
};

type RegistrationResultBody = {
  status?: string;
  already_registered?: boolean;
};

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
    const name = String(form.get('name') ?? '').trim();
    const contact = String(form.get('contact') ?? '').trim();
    const values = { name, contact };

    const res = await fetch(`${backendURL()}/api/events/${encodeURIComponent(params.id)}/registrations`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(values)
    });

    const body = (await res.json().catch(() => ({}))) as RegistrationErrorBody & RegistrationResultBody;
    if (!res.ok) {
      return fail(res.status, {
        values,
        code: body.code,
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
