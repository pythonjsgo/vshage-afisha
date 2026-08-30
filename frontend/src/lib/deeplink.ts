// Живая карточка в App Store (id 6760569940). До 30.08 здесь стояла заглушка
// `idTODO`: кнопка «Открыть в приложении» уводила всех, у кого приложение не
// установлено, на несуществующий адрес — то есть работала ровно наоборот.
const APP_STORE_URL = 'https://apps.apple.com/kz/app/vshage/id6760569940';
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
