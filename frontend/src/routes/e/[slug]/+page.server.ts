import { error, fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import {
	detectPlatform,
	OTHER_OPTION,
	type WebregApiError,
	type WebregEvent
} from '$lib/webreg';

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3004';

/** Anything slower than this and the visitor is staring at a blank screen. */
const LOAD_TIMEOUT_MS = 6000;
const SUBMIT_TIMEOUT_MS = 10000;

async function withTimeout<T>(ms: number, run: (signal: AbortSignal) => Promise<T>): Promise<T> {
	const ctrl = new AbortController();
	const timer = setTimeout(() => ctrl.abort(), ms);
	try {
		return await run(ctrl.signal);
	} finally {
		clearTimeout(timer);
	}
}

/** Visitor address, or empty when the platform cannot determine one. */
function safeClientAddress(get: () => string): string {
	try {
		return get();
	} catch {
		return '';
	}
}

export const load: PageServerLoad = async ({ params, fetch, request, setHeaders, url }) => {
	let res: Response;
	try {
		res = await withTimeout(LOAD_TIMEOUT_MS, (signal) =>
			fetch(`${backendURL()}/api/e/${encodeURIComponent(params.slug)}`, { signal })
		);
	} catch (e) {
		// A backend hiccup during an announcement burst must read as "try
		// again", never as a blank page with no explanation.
		console.error(`webreg load ${params.slug}:`, e);
		throw error(503, 'Страница не открылась. Обнови через пару секунд.');
	}
	if (res.status === 404) throw error(404, 'Событие не найдено');
	if (!res.ok) throw error(503, 'Страница не открылась. Обнови через пару секунд.');

	const event = (await res.json()) as WebregEvent;

	const done = url.searchParams.get('done') === '1';

	// Only the shared, anonymous view is cacheable. The post-registration
	// screen is per-visitor and must never be served to somebody else.
	setHeaders(
		done
			? { 'Cache-Control': 'no-store' }
			: { 'Cache-Control': 'public, max-age=15, stale-while-revalidate=120' }
	);

	return {
		event,
		platform: detectPlatform(request.headers.get('user-agent')),
		done,
		position: Number(url.searchParams.get('pos') ?? 0) || 0,
		alreadyRegistered: url.searchParams.get('again') === '1'
	};
};

export const actions: Actions = {
	register: async ({ request, params, fetch, getClientAddress }) => {
		const clientAddress = () => safeClientAddress(getClientAddress);
		const form = await request.formData();

		const affiliationRaw = String(form.get('affiliation') ?? '').trim();
		const affiliationOther = String(form.get('affiliation_other') ?? '').trim();
		const affiliation = affiliationRaw === OTHER_OPTION ? affiliationOther : affiliationRaw;

		// Custom organizer fields arrive as answer:<key> (and answer_other:<key>
		// for a select whose «Другое» branch was chosen).
		const answers: Record<string, string> = {};
		for (const [k, v] of form.entries()) {
			if (!k.startsWith('answer:')) continue;
			const key = k.slice('answer:'.length);
			const value = String(v).trim();
			if (value === OTHER_OPTION) {
				answers[key] = String(form.get(`answer_other:${key}`) ?? '').trim();
			} else if (value) {
				answers[key] = value;
			}
		}

		const payload = {
			name: String(form.get('name') ?? '').trim(),
			tg_username: String(form.get('tg_username') ?? '').trim(),
			affiliation,
			answers,
			consent: form.get('consent') === 'on' || form.get('consent') === 'true',
			source: String(form.get('source') ?? '').trim()
		};

		// Echoed back so a failed submit never blanks what the visitor typed.
		const values = {
			name: payload.name,
			tg_username: payload.tg_username,
			affiliation: affiliationRaw,
			affiliation_other: affiliationOther,
			answers,
			consent: payload.consent
		};

		let res: Response;
		try {
			res = await withTimeout(SUBMIT_TIMEOUT_MS, (signal) =>
				fetch(`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/register`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						// Without this every write looks like it came from the
						// frontend container, and the backend's per-IP limiter
						// would bucket the whole audience together.
						'X-Forwarded-For': clientAddress()
					},
					body: JSON.stringify(payload),
					signal
				})
			);
		} catch (e) {
			console.error(`webreg register ${params.slug}:`, e);
			return fail(503, {
				values,
				code: 'network',
				fieldErrors: {} as Record<string, string>,
				error: 'Не дошло до сервера. Проверь связь и нажми ещё раз — двойная запись не появится.'
			});
		}

		const body = (await res.json().catch(() => ({}))) as WebregApiError & {
			position?: number;
			already_registered?: boolean;
		};

		if (!res.ok) {
			return fail(res.status, {
				values,
				code: body.code,
				fieldErrors: (body.fields ?? {}) as Record<string, string>,
				error: body.message ?? 'Не удалось записаться. Попробуй ещё раз.'
			});
		}

		// The "done" state lives in the URL, not in `form`. Two reasons:
		// submitting the Android waitlist afterwards replaces `form` and would
		// otherwise throw the visitor back to an empty registration form, and
		// a refresh on a flaky connection would do the same. No name in the
		// query string — it would leak into referrers and access logs.
		const q = new URLSearchParams({ done: '1' });
		if (body.position) q.set('pos', String(body.position));
		if (body.already_registered) q.set('again', '1');
		redirect(303, `/e/${encodeURIComponent(params.slug)}?${q}`);
	},

	waitlist: async ({ request, params, fetch, getClientAddress }) => {
		const clientAddress = () => safeClientAddress(getClientAddress);
		const form = await request.formData();
		const payload = {
			platform: String(form.get('platform') ?? 'android').trim(),
			tg_username: String(form.get('tg_username') ?? '').trim(),
			name: String(form.get('name') ?? '').trim()
		};

		let res: Response;
		try {
			res = await withTimeout(SUBMIT_TIMEOUT_MS, (signal) =>
				fetch(`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/waitlist`, {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
						// Without this every write looks like it came from the
						// frontend container, and the backend's per-IP limiter
						// would bucket the whole audience together.
						'X-Forwarded-For': clientAddress()
					},
					body: JSON.stringify(payload),
					signal
				})
			);
		} catch (e) {
			console.error(`webreg waitlist ${params.slug}:`, e);
			return fail(503, {
				fieldErrors: {} as Record<string, string>,
				waitlistError: 'Не дошло до сервера. Попробуй ещё раз.'
			});
		}

		if (!res.ok) {
			const body = (await res.json().catch(() => ({}))) as WebregApiError;
			return fail(res.status, {
				fieldErrors: {} as Record<string, string>,
				waitlistError: body.message ?? 'Не получилось. Попробуй ещё раз.'
			});
		}
		return { waitlisted: true, fieldErrors: {} as Record<string, string> };
	}
};
