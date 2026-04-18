import { describe, it, expect } from 'vitest';
import { eventDeeplink } from '../../src/lib/deeplink';

describe('eventDeeplink', () => {
  it('builds vshage:// URL with id', () => {
    expect(eventDeeplink('abc123')).toBe('vshage://event/abc123');
  });
  it('URL-encodes special chars', () => {
    expect(eventDeeplink('a b/c')).toBe('vshage://event/a%20b%2Fc');
  });
});
