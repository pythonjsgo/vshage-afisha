/** Один встроенный контакт формы регистрации: спрашиваем ли и обязателен ли. */
export interface RegFieldToggle {
  enabled: boolean;
  required: boolean;
}

/**
 * Встроенный блок полей формы регистрации.
 *
 * `v` — версия раскладки. 0 или отсутствие всего объекта означает старую
 * форму («Имя» + «Контакт»), и страница обязана нарисовать именно её: форму
 * живого события нельзя менять под людьми, которые её прямо сейчас заполняют.
 */
export interface RegForm {
  v: number;
  name: RegFieldToggle;
  full_name: RegFieldToggle;
  email: RegFieldToggle;
  phone: RegFieldToggle;
  tg: RegFieldToggle;
  contact: RegFieldToggle;
  /** Зачем нужно ФИО — показывается под полем. */
  pass_note?: string;
}

/** Свой вопрос организатора. Ответ уезжает как answers[key]. */
export interface RegField {
  key: string;
  label: string;
  type: 'text' | 'textarea' | 'select' | 'checkbox';
  required?: boolean;
  options?: string[];
  allow_other?: boolean;
  hint?: string;
  max_len?: number;
}

export interface PublicEvent {
  id: string;
  /** Проставлен у событий веб-регистрации: карточка ведёт на /e/<slug>. */
  webreg_slug?: string;
  title: string;
  short_description?: string;
  description?: string;
  location?: string;
  start_time: string; // ISO
  end_time?: string;
  category?: string;
  tags: string[] | Record<string, unknown>;
  max_attendees?: number;
  attendee_count: number;
  photo_url?: string;
  status: 'published' | 'cancelled' | 'draft';
  registration_mode?: 'auto' | 'manual' | 'external';
  external_registration_url?: string;
  registration_deadline?: string;
  price_type?: 'free' | 'paid' | 'donation';
  price_min?: number;
  price_max?: number;
  currency?: string;
  city?: string;
  venue_name?: string;
  address?: string;
  online_url?: string;
  age_limit?: string;
  attendees_note?: string;
  is_featured: boolean;
  featured_position?: number;
  organizer_name?: string;
  organizer_photo?: string;
  photos?: string[];
  /** Ссылка на первоисточник анонса у импортированных событий (tgevents).
   *  Не украшение: показывать чужой анонс мы вправе только с подписанным
   *  источником. */
  source_url?: string;
  /** open / university / invite — пустят ли человека. */
  access_level?: string;
  /** Откуда событие: пусто у наших, 'tg' у импортированных. Явный признак,
   *  а не вывод из source_url: то поле nullable. */
  source?: 'tg';
  /** Настраиваемая форма регистрации. Отсутствует у событий, заведённых до
   *  этой возможности, — тогда рисуется старая форма из двух полей. */
  reg_form?: RegForm;
  /** Свои вопросы организатора, в порядке показа. */
  reg_fields?: RegField[];
  /** false = дата известна, времени нет (у импортированных это частый
   *  случай). Без признака полночь неотличима от начала в 00:00. */
  start_time_known?: boolean;
}

export interface ListResult {
  featured: PublicEvent[];
  all: PublicEvent[];
  total: number;
  /** Источники ленты, которые не ответили. Пусто — все живы. */
  degraded?: string[];
}
