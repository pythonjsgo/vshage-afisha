import type { PageServerLoad } from './$types';
import type { ListResult } from '$lib/types';

export const load: PageServerLoad = async ({ fetch, setHeaders }) => {
  const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
  // Лимит задаём ЯВНО. По умолчанию бэкенд отдаёт 30 на всю ленту сразу из
  // обоих сторов, а «показать ещё» на главной нет — значит всё, что не
  // влезло в тридцатку, недостижимо из интерфейса вообще, включая живое
  // событие веб-регистрации, которое директива 17.08 обязывает показывать.
  // 90 хватало, пока импортированных было двадцать. С городским слоем лента
  // выросла до полутора сотен, и подпись честно написала «СОБЫТИЯ · 90 ИЗ
  // 150»: пятьдесят событий стали недостижимы. 200 — с запасом к нынешнему
  // объёму, потолок бэкенда теперь 300 (maxPageSize == maxWindow).
  // Дальше нужна не большая цифра, а «показать ещё»: offset у /api/events
  // работает, страница его пока не использует.
  const res = await fetch(`${backend}/api/events?limit=200`);
  if (!res.ok) {
    return { featured: [], all: [], total: 0 } as ListResult;
  }
  const data = (await res.json()) as ListResult;
  // Отказ одного из источников ленты приходит в ответе 200 с непустым
  // degraded — снаружи это неотличимо от «в источнике просто ничего нет».
  // Логи PROD в Loki не доезжают, поэтому строка идёт в stdout контейнера,
  // и на неё можно повесить смок: непустой degraded = провал прогона.
  if (data.degraded?.length) {
    console.error('afisha: лента деградировала, молчат источники:', data.degraded.join(', '));
  }
  setHeaders({ 'Cache-Control': 'public, max-age=30, stale-while-revalidate=300' });
  return data;
};
