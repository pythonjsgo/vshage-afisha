// TODO: parent fills in actual App Store ID at App Store launch (replace `idTODO`)
const APP_STORE_URL = 'https://apps.apple.com/app/вшаге/idTODO';
// TODO: replace with real TestFlight invite when public link exists
const TESTFLIGHT_URL = 'https://testflight.apple.com/join/TODO';

export function eventDeeplink(eventId: string): string {
  return `vshage://event/${encodeURIComponent(eventId)}`;
}

export function openInApp(eventId: string) {
  const scheme = eventDeeplink(eventId);
  const fallback = APP_STORE_URL;
  const start = Date.now();
  const timer = setTimeout(() => {
    // still here after 1.5s → app not installed, go to App Store
    if (Date.now() - start < 2500) {
      window.location.href = fallback;
    }
  }, 1500);
  window.location.href = scheme;
  // Note: if app installed, page will blur (visibility) and timer cleared
  document.addEventListener('visibilitychange', function onHide() {
    if (document.visibilityState === 'hidden') {
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onHide);
    }
  });
}
