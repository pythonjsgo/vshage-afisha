import { error, fail } from '@sveltejs/kit';
import QRCode from 'qrcode';
import type { Actions, PageServerLoad } from './$types';
import type { WebregTicket } from '$lib/webreg';

const backendURL = () => process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3004';

const TIMEOUT_MS = 6000;

async function withTimeout<T>(ms: number, run: (signal: AbortSignal) => Promise<T>): Promise<T> {
	const ctrl = new AbortController();
	const timer = setTimeout(() => ctrl.abort(), ms);
	try {
		return await run(ctrl.signal);
	} finally {
		clearTimeout(timer);
	}
}

/**
 * The QR is rendered as inline SVG on the server, not by a script in the
 * browser. A ticket has to survive the worst network a venue door offers —
 * and a phone in airplane mode showing a saved page must still display the
 * code, which a client-side renderer would not.
 */
async function qrSVG(text: string): Promise<string> {
	return QRCode.toString(text, {
		type: 'svg',
		errorCorrectionLevel: 'M',
		margin: 1,
		// No colours here: the SVG inherits currentColor via the CSS below, so
		// the ticket follows the light/dark theme instead of fighting it.
		color: { dark: '#000000', light: '#00000000' }
	});
}

export const load: PageServerLoad = async ({ params, fetch, url, setHeaders }) => {
	let res: Response;
	try {
		res = await withTimeout(TIMEOUT_MS, (signal) =>
			fetch(
				`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/t/${encodeURIComponent(params.code)}`,
				{ signal }
			)
		);
	} catch (e) {
		console.error(`webreg ticket ${params.slug}/${params.code}:`, e);
		throw error(503, 'Билет не открылся. Обнови через пару секунд.');
	}
	if (res.status === 404) throw error(404, 'Билет не найден');
	if (!res.ok) throw error(503, 'Билет не открылся. Обнови через пару секунд.');

	const ticket = (await res.json()) as WebregTicket;

	// A ticket is one person's, never a shared page.
	setHeaders({ 'Cache-Control': 'no-store' });

	// The QR carries the ticket's own URL, so any camera app opens this page
	// rather than showing a bare code the door staff would have to type.
	const ticketURL = `${url.origin}/e/${ticket.event_slug}/t/${ticket.code}`;

	return {
		ticket,
		qr: await qrSVG(ticketURL),
		// The organizer arrives with ?key=… in the link they already hold;
		// that, and only that, turns this page into a door scanner.
		manageKey: url.searchParams.get('key') ?? ''
	};
};

export const actions: Actions = {
	checkin: async ({ request, params, fetch }) => {
		const form = await request.formData();
		const key = String(form.get('key') ?? '').trim();
		if (!key) return fail(403, { checkinError: 'Нужна ссылка организатора' });

		let res: Response;
		try {
			res = await withTimeout(TIMEOUT_MS, (signal) =>
				fetch(
					`${backendURL()}/api/e/${encodeURIComponent(params.slug)}/t/${encodeURIComponent(params.code)}/checkin?key=${encodeURIComponent(key)}`,
					{ method: 'POST', signal }
				)
			);
		} catch (e) {
			console.error(`webreg checkin ${params.slug}/${params.code}:`, e);
			return fail(503, { checkinError: 'Не дошло до сервера. Нажми ещё раз.' });
		}
		if (!res.ok) {
			return fail(res.status, {
				checkinError:
					res.status === 404
						? 'Ссылка организатора не подошла'
						: 'Не получилось отметить. Попробуй ещё раз.'
			});
		}
		return { checkedIn: true };
	}
};
