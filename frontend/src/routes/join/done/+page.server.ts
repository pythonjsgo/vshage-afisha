import { platformFromUA } from '$lib/join';
import type { PageServerLoad } from './$types';

// Платформа определяется НА СЕРВЕРЕ, из user-agent запроса: кнопка тогда
// нарисована сразу и правильно — без подмены после гидрации и без пустого
// места у человека с выключенным JS.
export const load: PageServerLoad = ({ request }) => {
  return { platform: platformFromUA(request.headers.get('user-agent')) };
};
