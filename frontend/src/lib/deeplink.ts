import { APP_STORE_URL } from './join';
export { APP_STORE_URL } from './join';

export function eventDeeplink(eventId: string): string {
  return `vshage://event/${encodeURIComponent(eventId)}`;
}

let cancelPending: (() => void) | undefined;

export function openInApp(eventId: string) {
  cancelPending?.();
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  if (!ios) {
    window.location.href = APP_STORE_URL;
    return;
  }

  const cleanup = () => {
    clearTimeout(timer);
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('pagehide', cleanup);
    cancelPending = undefined;
  };
  const onVisibility = () => {
    if (document.visibilityState === 'hidden') cleanup();
  };
  const timer = setTimeout(() => {
    const visible = document.visibilityState !== 'hidden';
    cleanup();
    if (visible) window.location.href = APP_STORE_URL;
  }, 1500);
  cancelPending = cleanup;
  // Register before launching: an installed app can hide the page immediately.
  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('pagehide', cleanup);
  window.location.href = eventDeeplink(eventId);
}
