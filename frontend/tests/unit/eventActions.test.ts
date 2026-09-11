import { describe, expect, it } from 'vitest';
import type { PublicEvent } from '../../src/lib/types';
import { eventAction, eventPrice, eventSource } from '../../src/lib/event-actions';
import { eventCopy } from '../../src/lib/event-copy';

const base: PublicEvent = { id: 'event', title: 'Event', start_time: '2026-09-22T11:30:00+03:00', status: 'published', tags: [], attendee_count: 0, is_featured: false };
describe('event registration and sharing are different actions', () => {
  it('opens paid external tickets, preserving the tariff anchor', () => {
    const event = { ...base, source: 'tg' as const, price_type: 'paid' as const, external_registration_url: 'https://arena.unicornfellowship.ru/#tarif' };
    expect(eventAction(event)).toEqual({ href: event.external_registration_url, external: true, label: 'buyTicket' });
    expect(eventCopy('buyTicket')).toBe('Купить билет');
    expect(eventCopy('buyTicket', 'en')).toBe('Buy a ticket');
  });
  it('names the actual Telegram registration destination', () => {
    expect(eventAction({ ...base, registration_mode: 'external', price_type: 'paid', external_registration_url: 'https://t.me/chernyynuar' })?.label).toBe('telegramRegister');
    expect(eventCopy('telegramRegister')).not.toBe(eventCopy('telegramShare'));
  });
  it('never uses a Telegram share link as a registration action', () => {
    expect(eventAction({ ...base, source: 'tg', external_registration_url: 'https://t.me/share/url?url=anything' })).toBeUndefined();
    expect(eventAction({ ...base, source: 'tg', price_type: 'paid', external_registration_url: 'https://t.me.example.org/ticket' })?.label).toBe('buyTicket');
  });
  it('preserves internal registration and avoids making a form for an external announcement', () => {
    expect(eventAction(base)?.href).toBe('/events/event/register');
    expect(eventAction({ ...base, webreg_slug: 'panel' })?.href).toBe('/e/panel');
    expect(eventAction({ ...base, registration_mode: 'external', external_registration_url: '/e/panel' })).toEqual({ href: '/e/panel', label: 'register', external: false });
    expect(eventAction({ ...base, source: 'tg' })).toBeUndefined();
    expect(eventAction({ ...base, source: 'tg', source_url: 'https://example.org/' })?.label).toBe('details');
  });
  it('does not turn a free event into a ticket purchase', () => {
    expect(eventAction({ ...base, source: 'tg', price_type: 'free', external_registration_url: 'https://example.org/' })?.label).toBe('websiteRegister');
  });
  it('renders external price wording and tolerates old incomplete records', () => {
    expect(eventPrice({ ...base, price_type: 'paid', price_text: 'от 10 000 ₽' })).toBe('от 10 000 ₽');
    expect(eventPrice(base)).toBeUndefined();
    expect(eventPrice({ ...base, price_type: 'free' })).toBe('Бесплатно');
    expect(() => eventPrice({ ...base, price_type: 'paid', price_min: 5000, currency: '₽' })).not.toThrow();
  });
  it('keeps source attribution while rejecting executable URLs', () => {
    expect(eventSource({ ...base, source_url: 'https://arena.unicornfellowship.ru/' })?.host).toBe('arena.unicornfellowship.ru');
    expect(eventSource({ ...base, source_url: 'javascript:alert(1)' })).toBeUndefined();
  });
});
