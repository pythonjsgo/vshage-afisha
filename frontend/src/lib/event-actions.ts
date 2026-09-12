import type { PublicEvent } from './types';
import { eventCopy, type EventCopyKey, type EventLocale } from './event-copy';

export function httpURL(value?: string): URL | undefined {
  try {
    const url = new URL(value ?? '');
    return ['https:', 'http:'].includes(url.protocol) ? url : undefined;
  } catch { return undefined; }
}

export function eventAction(event: PublicEvent): { href: string; label: EventCopyKey; external: boolean } | undefined {
  if (event.webreg_slug) return { href: `/e/${event.webreg_slug}`, label: 'register', external: false };
  // The legacy web-registration adapter uses external mode with our own /e
  // path. It is still a working internal form, not an off-site ticket seller.
  if (!event.source && event.registration_mode === 'external' && /^\/e\/[a-z0-9_-]+$/i.test(event.external_registration_url ?? '')) {
    return { href: event.external_registration_url!, label: 'register', external: false };
  }
  const external = httpURL(event.external_registration_url);
  if (external && (event.source === 'tg' || event.registration_mode === 'external')) {
    const telegram = ['t.me', 'telegram.me', 'www.t.me', 'www.telegram.me'].includes(external.hostname);
    // A share link cannot register someone. Keep it out of the primary action.
    if (!telegram || !external.pathname.startsWith('/share')) {
      return { href: external.href, external: true,
        label: telegram ? 'telegramRegister' : event.price_type === 'paid' ? 'buyTicket' : 'websiteRegister' };
    }
  }
  if (event.source === 'tg' || event.registration_mode === 'external') {
    const source = httpURL(event.source_url);
    return source ? { href: source.href, external: true, label: 'details' } : undefined;
  }
  return { href: `/events/${event.id}/register`, label: 'register', external: false };
}

export function eventPrice(event: PublicEvent, locale: EventLocale = 'ru'): string | undefined {
  if (event.price_type === 'free') return eventCopy('free', locale);
  if (event.price_text?.trim()) return event.price_text.trim();
  if (event.price_type === 'donation') return eventCopy('donation', locale);
  if (event.price_type !== 'paid' || event.price_min == null) return undefined;
  const currency = event.currency || 'RUB';
  const format = (value: number) => {
    try { return new Intl.NumberFormat(locale, { style: 'currency', currency, maximumFractionDigits: 0 }).format(value); }
    catch { return `${new Intl.NumberFormat(locale).format(value)} ${currency}`; }
  };
  if (event.price_max != null && event.price_max !== event.price_min) return `${format(event.price_min)} — ${format(event.price_max)}`;
  if (event.price_max == null) return `${eventCopy('from', locale)} ${format(event.price_min)}`;
  return format(event.price_min);
}

export function eventSource(event: PublicEvent): { href: string; host: string } | undefined {
  const url = httpURL(event.source_url || (event.source === 'tg' || event.registration_mode === 'external' ? event.external_registration_url : undefined));
  return url ? { href: url.href, host: url.hostname.replace(/^www\./, '') } : undefined;
}
