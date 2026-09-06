import type { ParamMatcher } from '@sveltejs/kit';
import { CITY_SLUGS } from '$lib/api';

/**
 * Первый сегмент адреса — город.
 *
 * Матчер здесь не украшение, а единственное, что мешает двухсегментному
 * динамическому маршруту проглотить чужие пути: без него `/whatever/thing`
 * открылся бы страницей раздела и ответил 200 на адрес, которого нет.
 * Список городов зашит (см. CITY_SLUGS в $lib/api) — матчер синхронный и
 * спросить бэкенд не может.
 */
export const match: ParamMatcher = (param) => CITY_SLUGS.includes(param);
