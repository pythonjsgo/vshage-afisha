import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APP_STORE_URL, eventDeeplink, openInApp } from '../../src/lib/deeplink';

describe('eventDeeplink', () => {
  it('builds vshage:// URL with id', () => {
    expect(eventDeeplink('abc123')).toBe('vshage://event/abc123');
  });
  it('URL-encodes special chars', () => {
    expect(eventDeeplink('a b/c')).toBe('vshage://event/a%20b%2Fc');
  });
});

describe('app launch with App Store fallback', () => {
  let navigations: string[];
  let onNavigate: (url: string) => void;
  beforeEach(() => {
    vi.useFakeTimers();
    navigations = [];
    onNavigate = () => {};
    const fakeWindow = Object.assign(new EventTarget(), {
      location: { set href(url: string) { navigations.push(url); onNavigate(url); } }
    });
    vi.stubGlobal('window', fakeWindow);
    vi.stubGlobal('navigator', { userAgent: 'iPhone', platform: 'iPhone', maxTouchPoints: 5 });
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  });
  afterEach(() => {
    window.dispatchEvent(new Event('pagehide'));
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });
  it('opens the current store listing when iOS stays in the browser', () => {
    openInApp('event-id');
    expect(navigations).toEqual([eventDeeplink('event-id')]);
    vi.advanceTimersByTime(5000);
    expect(navigations).toEqual([eventDeeplink('event-id'), APP_STORE_URL]);
    expect(APP_STORE_URL).toBe('https://apps.apple.com/ru/app/vshage/id6760569940');
  });
  it('cancels fallback even when launching the app hides the page immediately', () => {
    onNavigate = () => {
      vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
      document.dispatchEvent(new Event('visibilitychange'));
    };
    openInApp('event-id');
    vi.advanceTimersByTime(5000);
    expect(navigations).toEqual([eventDeeplink('event-id')]);
  });
  it('uses the real store URL directly on desktop', () => {
    vi.stubGlobal('navigator', { userAgent: 'Desktop', platform: 'MacIntel', maxTouchPoints: 0 });
    openInApp('event-id');
    vi.advanceTimersByTime(5000);
    expect(navigations).toEqual([APP_STORE_URL]);
  });
});
