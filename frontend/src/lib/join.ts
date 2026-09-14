// Анкета на вход в закрытую сеть — vshage.app/join.
//
// Страница живёт под платным трафиком Яндекс.Директа и отдаётся ЭТИМ
// приложением с ДВУХ адресов: afisha.vshage.app/join (свой origin) и
// vshage.app/join (apex, проксируется на afisha-frontend, как /e/*).
// Канонический — всегда apex: он в объявлении, и по нему считает Метрика.

export const JOIN_CANONICAL = 'https://vshage.app/join';
export const JOIN_DONE_PATH = '/join/done';

/** Действующая политика конфиденциальности приложения. Страница уже есть
 *  (vshage-landing, content/privacy.md → /privacy/); новую не заводим. */
export const PRIVACY_URL = 'https://vshage.app/privacy/';

export const APP_STORE_URL = 'https://apps.apple.com/ru/app/vshage/id6760569940';
export const AFISHA_URL = 'https://afisha.vshage.app/';

/** OG-картинка. Абсолютный адрес НА ОРИГИН АФИШИ, а не на apex: на
 *  vshage.app проксированы только /e/*, /_app/* и /join*, и /og-default.png
 *  там отдал бы 404 — ровно тот класс ошибки, из-за которого страница
 *  однажды приехала без стилей с кодом 200. */
export const JOIN_OG_IMAGE = 'https://afisha.vshage.app/og-default.png';

export type Option = { code: string; label: string };

/**
 * Вузы в выпадающем списке — В ПОРЯДКЕ ПОКАЗА.
 *
 * ВТОРАЯ КОПИЯ НАБОРА ЖИВЁТ НА СЕРВЕРЕ: backend/internal/join/join.go.
 * Связаны они только кодами; разъехавшись, дают тихий отказ — человек
 * выбирает вуз, которого сервер не знает, и теряется на валидации. Поэтому у
 * каждой стороны есть тест, перечисляющий коды целиком (join.test.ts и
 * join_test.go::TestUniversityCodes): правка одной стороны обязана уронить
 * её тест, а не всплыть на живом трафике.
 */
export const UNIVERSITIES: Option[] = [
  { code: 'msu', label: 'МГУ' },
  { code: 'hse', label: 'ВШЭ' },
  { code: 'mgimo', label: 'МГИМО' },
  { code: 'ranepa', label: 'РАНХиГС' },
  { code: 'finu', label: 'Финансовый университет' },
  { code: 'mipt', label: 'МФТИ' },
  { code: 'bmstu', label: 'Бауманка' },
  { code: 'plekhanov', label: 'РЭУ им. Плеханова' },
  { code: 'other', label: 'Другой' }
];

export const OTHER_UNIVERSITY = 'other';

export const COURSES: Option[] = [
  { code: '1', label: '1 курс' },
  { code: '2', label: '2 курс' },
  { code: '3', label: '3 курс' },
  { code: '4', label: '4 курс' },
  { code: '5', label: '5 курс' },
  { code: '6', label: '6 курс' },
  { code: 'graduate', label: 'Выпускник' }
];

export const ABOUT_MAX = 140;

/** Метки рекламной кампании. Приезжают в query страницы и уходят скрытыми
 *  полями: Метрика отвечает на «сколько кликов», а на «из какой кампании
 *  человек, которого мы взяли» — только строка в базе. */
export const TRACKING_KEYS = [
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_content',
  'utm_term',
  'yclid'
] as const;

export type Tracking = Partial<Record<(typeof TRACKING_KEYS)[number], string>>;

export function readTracking(url: URL): Tracking {
  const out: Tracking = {};
  for (const key of TRACKING_KEYS) {
    const v = url.searchParams.get(key);
    // Обрезаем: метка приезжает из чужой системы, и её длину мы не выбираем.
    if (v) out[key] = v.slice(0, 400);
  }
  return out;
}

const TG_PREFIXES = [
  'https://t.me/',
  'http://t.me/',
  't.me/',
  'https://telegram.me/',
  'http://telegram.me/',
  'telegram.me/',
  // Список обязан совпадать с серверным (webreg/validate.go): префикс,
  // который сервер примет, а клиент подсветит красным, — это подсказка,
  // сообщающая человеку неправду.
  'https://telegram.dog/',
  'telegram.dog/'
];

/**
 * Мягкая клиентская проверка юзернейма — ровно чтобы подсветить поле до
 * отправки. Судья всё равно сервер (webreg.NormalizeTG): дублировать там
 * правило целиком незачем, а вот отправлять человека в круг «нажал →
 * подождал → ошибка» из-за забытой буквы — стоит одного лишнего лида.
 */
export function looksLikeTelegram(raw: string): boolean {
  let s = raw.trim();
  if (!s) return false;
  const cut = s.search(/[?#]/);
  if (cut >= 0) s = s.slice(0, cut);
  const lower = s.toLowerCase();
  for (const p of TG_PREFIXES) {
    if (lower.startsWith(p)) {
      s = s.slice(p.length);
      break;
    }
  }
  s = s.replace(/^\/+|\/+$/g, '').replace(/^@/, '').trim();
  return /^[A-Za-z0-9_]{5,32}$/.test(s);
}

export type Platform = 'ios' | 'other' | 'unknown';

/**
 * Платформа по user-agent — для единственной кнопки на странице «спасибо».
 *
 * Планшет Apple с iPadOS 13+ представляется Macintosh, поэтому Mac попадает
 * в 'unknown' и получает ОБЕ ссылки: показать маку кнопку App Store — значит
 * соврать половине из них, а спрятать — соврать другой половине.
 */
export function platformFromUA(ua: string | null | undefined): Platform {
  const s = (ua ?? '').toLowerCase();
  if (!s) return 'unknown';
  if (/iphone|ipad|ipod/.test(s)) return 'ios';
  if (/android|windows|linux|cros/.test(s)) return 'other';
  return 'unknown';
}
