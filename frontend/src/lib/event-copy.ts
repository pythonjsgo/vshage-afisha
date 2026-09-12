export type EventLocale = 'ru' | 'en';
const copy = {
  ru: {
    share: 'Поделиться событием', copyLink: 'Скопировать ссылку', copied: 'Ссылка скопирована',
    telegramShare: 'Отправить друзьям в Telegram', copyFailed: 'Не удалось скопировать. Ссылка:',
    buyTicket: 'Купить билет', register: 'Зарегистрироваться', telegramRegister: 'Записаться в Telegram',
    websiteRegister: 'Регистрация на сайте', details: 'Подробнее о событии',
    participation: 'Участие', price: 'Стоимость', free: 'Бесплатно', donation: 'Свободный взнос',
    from: 'от', organizerSite: 'Страница организатора', eventSource: 'О событии',
    ends: 'Окончание', performers: 'Участники программы', salesStart: 'Начало регистрации', when: 'Когда', where: 'Где', organizer: 'Организатор', who: 'Кто идёт', going: 'идут',
    afisha: 'Афиша', cancelled: 'Отменено', students: 'Для студентов', invite: 'По приглашению',
  },
  en: {
    share: 'Share this event', copyLink: 'Copy link', copied: 'Link copied',
    telegramShare: 'Share with friends on Telegram', copyFailed: 'Could not copy. Link:',
    buyTicket: 'Buy a ticket', register: 'Register', telegramRegister: 'Register via Telegram',
    websiteRegister: 'Register on the website', details: 'Event details',
    participation: 'Attend', price: 'Price', free: 'Free', donation: 'Optional donation',
    from: 'from', organizerSite: 'Organizer page', eventSource: 'About the event',
    ends: 'Ends', performers: 'Program participants', salesStart: 'Registration opens', when: 'When', where: 'Where', organizer: 'Organizer', who: 'Who is going', going: 'going',
    afisha: 'Events', cancelled: 'Cancelled', students: 'For students', invite: 'By invitation',
  },
} as const;
export type EventCopyKey = keyof typeof copy.ru;
export function eventCopy(key: EventCopyKey, locale: EventLocale = 'ru'): string { return copy[locale][key]; }
export function documentEventLocale(): EventLocale {
  return typeof document !== 'undefined' && document.documentElement.lang.startsWith('en') ? 'en' : 'ru';
}
