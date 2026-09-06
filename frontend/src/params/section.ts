import type { ParamMatcher } from '@sveltejs/kit';
import { sectionBySlug } from '$lib/taxonomy';

/**
 * Второй сегмент — раздел из ALL_SECTIONS. Неизвестный слаг сюда не проходит,
 * и SvelteKit отдаёт 404 сам: пустая лента по несуществующему адресу — это
 * страница-дубль в индексе, а не «просто ничего не нашли».
 */
export const match: ParamMatcher = (param) => sectionBySlug(param) !== undefined;
