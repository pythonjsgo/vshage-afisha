// Types + helpers for the standalone web registration flow served at
// vshage.app/e/<slug>. Mirrors backend/internal/webreg/models.go.

export type VenueCard = {
	vid?: string;
	name?: string;
	address?: string;
	district?: string;
	lat?: number;
	lon?: number;
	rating_avg?: number | null;
	rating_count?: number;
	tags?: string[];
	maps_url?: string;
	note?: string;
	online_url?: string;
};

export type WebregField = {
	key: string;
	label: string;
	type: 'select' | 'text' | 'textarea' | 'checkbox';
	required?: boolean;
	options?: string[];
	allow_other?: boolean;
	hint?: string;
	max_len?: number;
};

export type WebregBridge = {
	ios_mode?: 'testflight' | 'app_store' | 'waitlist' | 'off';
	testflight_url?: string;
	app_store_url?: string;
	invite_code?: string;
	android_waitlist?: boolean;
	tg_channel_url?: string;
	tg_chat_url?: string;
	privacy_url?: string;
	/** Страница с инструкцией по установке. По умолчанию vshage.app/#beta. */
	install_url?: string;
	/**
	 * Куда ведёт «Открыть во Вшаге». Пока в приложении нет экрана веб-события,
	 * поле пустое и кнопка падает на установку — для человека без приложения
	 * «открыть во Вшаге» и значит «поставить Вшаге».
	 */
	vshage_url?: string;
};

/** Один встроенный контакт: спрашиваем ли вообще и обязателен ли. */
export type FieldToggle = { enabled: boolean; required: boolean };

/** Встроенный блок контактов. Всё остальное — поля организатора. */
export type WebregForm = {
	v: number;
	name: FieldToggle;
	full_name: FieldToggle;
	email: FieldToggle;
	phone: FieldToggle;
	tg: FieldToggle;
	affiliation: FieldToggle;
	/** Зачем нужно ФИО — показывается под полем. */
	pass_note?: string;
};

export type TicketMode = 'qr' | 'code' | 'off';

export type WebregEvent = {
	slug: string;
	title: string;
	tagline?: string;
	description?: string;
	cover_url?: string;
	starts_at: string;
	ends_at?: string;
	timezone: string;
	venue: VenueCard;
	form: WebregForm;
	fields: WebregField[];
	affiliations: string[];
	bridge: WebregBridge;
	organizer_title?: string;
	capacity?: number;
	registration_open: boolean;
	publish_afisha: boolean;
	publish_vshage: boolean;
	ticket_mode: TicketMode;
	registered_count: number;
	seats_left?: number;
};

export type WebregRegistration = {
	id: number;
	name: string;
	full_name?: string;
	email?: string;
	phone?: string;
	tg_username?: string;
	tg_display?: string;
	affiliation?: string;
	answers: Record<string, string>;
	ticket_code?: string;
	checked_in_at?: string;
	created_at: string;
};

export type WebregManageList = {
	slug: string;
	title: string;
	starts_at: string;
	timezone: string;
	form: WebregForm;
	fields: WebregField[];
	ticket_mode: TicketMode;
	capacity?: number;
	total: number;
	checked_in: number;
	registrations: WebregRegistration[];
};

export type WebregTicket = {
	event_slug: string;
	event_title: string;
	starts_at: string;
	timezone: string;
	venue_name?: string;
	venue_address?: string;
	name: string;
	full_name?: string;
	code: string;
	checked_in_at?: string;
};

/**
 * Адрес афиши для текущего стенда. Выводится из хоста запроса, а не из
 * константы: одна и та же сборка обслуживает `vshage.app` и `dev.vshage.app`,
 * и зашитый прод-домен увёл бы проверку на DEV в чужой стенд.
 *
 * `/e/<slug>` отдаётся с обоих хостов, а карточка афиши живёт только на
 * поддомене афиши — поэтому ссылка туда обязана быть абсолютной.
 */
export function afishaOriginFor(host: string): string {
	const h = (host ?? '').toLowerCase().replace(/:\d+$/, '');
	if (h.startsWith('afisha.')) return `https://${h}`;
	if (h === 'vshage.app' || h === 'www.vshage.app') return 'https://afisha.vshage.app';
	if (h.endsWith('dev.vshage.app')) return 'https://afisha.dev.vshage.app';
	// Локальная разработка: афиша и /e/ живут на одном порту.
	return '';
}

/**
 * Карточка события в афише. Отдельная от ссылки регистрации by design
 * (директива фаундера 17.08): афиша — витрина с обложкой и полным описанием,
 * /e/<slug> — короткая страница, которую шлют в личку, чтобы записаться.
 */
export function afishaEventURL(origin: string, slug: string): string {
	return `${origin}/${slug}`;
}

export type WebregApiError = {
	code?: string;
	message?: string;
	fields?: Record<string, string>;
};

/** The literal free-text option appended to every вуз/статус picker. */
export const OTHER_OPTION = 'Другое';

const MONTHS = [
	'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
	'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'
];

/**
 * Formats a date in the EVENT's timezone, not the viewer's.
 *
 * Both matter: the server renders in UTC while a phone renders in local time,
 * so a naive `new Date().getHours()` prints a different hour before and after
 * hydration. And an attendee in another timezone must still read the Moscow
 * start time — the event happens where the event happens.
 */
export function formatEventWhen(iso: string, timeZone: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	const parts = new Intl.DateTimeFormat('ru-RU', {
		timeZone,
		weekday: 'short',
		day: 'numeric',
		month: 'numeric',
		year: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	}).formatToParts(d);

	const get = (t: string) => parts.find((p) => p.type === t)?.value ?? '';
	const day = Number(get('day'));
	const monthIdx = Number(get('month')) - 1;
	// Weekday must come from the formatted parts, not from the Date object —
	// getDay() would answer in UTC (server) or the viewer's zone (browser),
	// both of which flip around midnight in the event's zone.
	const weekday = get('weekday').replace(/\.$/, '');
	const month = MONTHS[monthIdx] ?? '';
	return `${day} ${month}, ${weekday} · ${get('hour')}:${get('minute')}`;
}

/** Short "18 авг · 19:00" form used in the manage header. */
export function formatShort(iso: string, timeZone: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return new Intl.DateTimeFormat('ru-RU', {
		timeZone,
		day: 'numeric',
		month: 'short',
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	}).format(d);
}

export function formatTime(iso: string, timeZone: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return new Intl.DateTimeFormat('ru-RU', {
		timeZone,
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	}).format(d);
}

/**
 * Route link for the venue block. Prefers the curated maps_url from the geo
 * catalog, falls back to coordinates, then to an address search — so the
 * button is never dead as long as we know *something* about the place.
 */
export function routeURL(venue: VenueCard): string | null {
	// OSM-sourced catalog rows carry an openstreetmap.org maps_url. It is a
	// valid link but a poor one for a Moscow attendee — no transit, no
	// «поехали». Coordinates on Yandex beat it, so only a non-OSM curated URL
	// wins outright.
	if (venue.maps_url && !/openstreetmap\.org/i.test(venue.maps_url)) return venue.maps_url;
	if (typeof venue.lat === 'number' && typeof venue.lon === 'number' && (venue.lat || venue.lon)) {
		return `https://yandex.ru/maps/?pt=${venue.lon},${venue.lat}&z=17&l=map`;
	}
	if (venue.maps_url) return venue.maps_url;
	const q = [venue.name, venue.address].filter(Boolean).join(' ');
	if (q) return `https://yandex.ru/maps/?text=${encodeURIComponent(q)}`;
	return null;
}

export function has2GIS(venue: VenueCard): string | null {
	const q = [venue.name, venue.address].filter(Boolean).join(' ');
	if (!q) return null;
	return `https://2gis.ru/search/${encodeURIComponent(q)}`;
}

/** iOS / Android split for the post-registration bridge, from the User-Agent. */
export function detectPlatform(ua: string | null | undefined): 'ios' | 'android' | 'other' {
	const s = (ua ?? '').toLowerCase();
	if (/iphone|ipad|ipod/.test(s)) return 'ios';
	// iPadOS 13+ reports as Macintosh; treat a touch Mac as iOS only when it
	// also looks mobile. Desktop Safari must not get an install button.
	if (/android/.test(s)) return 'android';
	return 'other';
}
