import { describe, it, expect } from 'vitest';
import { formatEventDate, formatEventDateLong } from '../../src/lib/dateFormat';

describe('formatEventDate', () => {
  it('handles today', () => {
    const now = new Date('2026-04-25T10:00:00');
    expect(formatEventDate('2026-04-25T19:00:00', now)).toBe('СЕГОДНЯ В 19:00');
  });
  it('handles tomorrow', () => {
    const now = new Date('2026-04-25T10:00:00');
    expect(formatEventDate('2026-04-26T19:00:00', now)).toBe('ЗАВТРА В 19:00');
  });
  it('handles far future', () => {
    const now = new Date('2026-04-25T10:00:00');
    expect(formatEventDate('2026-05-15T19:00:00', now)).toBe('15 МАЯ · 19:00');
  });
});

describe('formatEventDateLong', () => {
  it('formats with month', () => {
    expect(formatEventDateLong('2026-04-25T19:30:00')).toBe('25 АПР · 19:30');
  });
});
