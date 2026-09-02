import satori from 'satori';
import { Resvg } from '@resvg/resvg-js';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import type { RequestHandler } from './$types';

/**
 * Карточка для превью ссылки в мессенджерах (og:image).
 *
 * Шрифты — Unbounded, брендовый дисплей. Это не вкусовщина: до 02.09 карточка
 * рисовалась одним Bowlby One, в котором НЕТ кириллицы, и на проде превью
 * события «Панельная дискуссия…» состояло из чёрного прямоугольника и цифр
 * даты — весь русский текст отрисовывался в пустоту. Отказ был немой:
 * эндпоинт отвечал 200, PNG был валидный, размер правильный.
 *
 * Правило на будущее: любой шрифт, которым мы рисуем СЕРВЕРНЫЙ текст,
 * обязан покрывать кириллицу — на сервере некому подставить системный
 * фолбэк, как это делает браузер.
 */

type Face = { name: string; data: Buffer; weight: 400 | 500 | 800; style: 'normal' };
let facesPromise: Promise<Face[]> | null = null;

function loadFaces(): Promise<Face[]> {
	if (!facesPromise) {
		facesPromise = Promise.all([
			readFile(path.resolve('static/fonts/Unbounded-ExtraBold.ttf')),
			readFile(path.resolve('static/fonts/Unbounded-Medium.ttf'))
		]).then(([bold, medium]) => [
			{ name: 'Unbounded', data: bold, weight: 800 as const, style: 'normal' as const },
			{ name: 'Unbounded', data: medium, weight: 500 as const, style: 'normal' as const },
			{ name: 'Unbounded', data: medium, weight: 400 as const, style: 'normal' as const }
		]);
	}
	return facesPromise;
}

const MONTHS = [
	'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
	'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'
];

/** Дата и время события в МСК — так же, как их видит человек на странице. */
function whenLine(iso: string): string {
	const d = new Date(iso);
	const msk = new Date(d.getTime() + 3 * 3600 * 1000);
	const day = msk.getUTCDate();
	const month = MONTHS[msk.getUTCMonth()];
	const hh = String(msk.getUTCHours()).padStart(2, '0');
	const mm = String(msk.getUTCMinutes()).padStart(2, '0');
	return `${day} ${month} · ${hh}:${mm}`;
}

function placeLine(ev: Record<string, unknown>): string {
	const parts = [ev.venue_name, ev.address ?? ev.location, ev.city]
		.map((v) => (typeof v === 'string' ? v.trim() : ''))
		.filter(Boolean);
	const out: string[] = [];
	for (const p of parts) {
		if (!out.some((s) => s.toLowerCase().includes(p.toLowerCase()))) out.push(p);
	}
	if (!out.length && typeof ev.online_url === 'string' && ev.online_url) return 'Онлайн';
	return out.join(', ');
}

/**
 * Обложка тянется на СЕРВЕРЕ и вшивается в SVG как data-URI: satori не ходит
 * в сеть сам, а внешний адрес в <img> дал бы пустое место без единой ошибки.
 * Внутренний апстрим (organizer-api отдаёт /uploads/*) предпочтительнее
 * публичного адреса — иначе картинка едет из контейнера в интернет и обратно.
 */
type Cover = { uri: string; width: number; height: number };

async function loadCover(photoURL: string, origin: string): Promise<Cover | null> {
	if (!photoURL) return null;
	let target = photoURL;
	if (!/^https?:\/\//i.test(photoURL)) {
		const base = process.env.UPLOADS_INTERNAL_URL ?? origin;
		target = base.replace(/\/$/, '') + (photoURL.startsWith('/') ? photoURL : '/' + photoURL);
	}
	try {
		const ctrl = new AbortController();
		const timer = setTimeout(() => ctrl.abort(), 4000);
		const res = await fetch(target, { signal: ctrl.signal });
		clearTimeout(timer);
		if (!res.ok) return null;
		const type = res.headers.get('content-type') ?? 'image/jpeg';
		if (!type.startsWith('image/')) return null;
		const buf = Buffer.from(await res.arrayBuffer());
		// 8 МБ — потолок здравого смысла: карточка кешируется, но держать
		// в памяти чужую мегапанораму незачем.
		if (buf.byteLength > 8 * 1024 * 1024) return null;
		const size = imageSize(buf);
		if (!size) return null;
		return { uri: `data:${type};base64,${buf.toString('base64')}`, ...size };
	} catch {
		return null;
	}
}

/**
 * Размер картинки из заголовка. Нужен, чтобы выбрать раскладку: широкую
 * обложку можно тянуть на весь кадр, а квадратный ПОСТЕР — нельзя, кроп
 * срежет с него и лицо, и заголовок (ровно это требование фаундер поставил
 * отдельной строкой, когда заводили событие 10.09).
 */
function imageSize(buf: Buffer): { width: number; height: number } | null {
	// PNG: IHDR сразу за восьмибайтовой сигнатурой.
	if (buf.length > 24 && buf.readUInt32BE(0) === 0x89504e47) {
		return { width: buf.readUInt32BE(16), height: buf.readUInt32BE(20) };
	}
	// JPEG: идём по маркерам до SOF0..SOF3 / SOF5..SOF7 / SOF9..SOF11.
	if (buf.length > 4 && buf[0] === 0xff && buf[1] === 0xd8) {
		let i = 2;
		while (i + 9 < buf.length) {
			if (buf[i] !== 0xff) {
				i++;
				continue;
			}
			const marker = buf[i + 1];
			if (marker === 0xd8 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
				i += 2;
				continue;
			}
			const len = buf.readUInt16BE(i + 2);
			const isSOF =
				(marker >= 0xc0 && marker <= 0xc3) ||
				(marker >= 0xc5 && marker <= 0xc7) ||
				(marker >= 0xc9 && marker <= 0xcb);
			if (isSOF) return { height: buf.readUInt16BE(i + 5), width: buf.readUInt16BE(i + 7) };
			i += 2 + len;
		}
	}
	return null;
}

/**
 * Заголовок ужимается по длине и по ширине колонки: в узкой (постер справа)
 * места вдвое меньше, и тот же кегль уехал бы под картинку.
 */
function titleSize(title: string, wide: boolean): number {
	const scale = wide ? 1 : 0.62;
	const base = title.length <= 24 ? 88 : title.length <= 44 ? 68 : title.length <= 70 ? 54 : 46;
	return Math.round(base * scale);
}

function div(style: Record<string, unknown>, children: unknown): unknown {
	return { type: 'div', props: { style, children } };
}

export const GET: RequestHandler = async ({ params, fetch, url }) => {
	const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
	const res = await fetch(`${backend}/api/events/${encodeURIComponent(params.id)}`);
	if (!res.ok) return new Response('Not found', { status: 404 });
	const ev = (await res.json()) as Record<string, any>;

	const registering = url.searchParams.get('kind') === 'register';
	const title = String(ev.title ?? '');
	const cover = await loadCover(String(ev.photo_url ?? ''), url.origin);
	const fonts = await loadFaces();

	const kicker = registering ? 'ЗАПИСЬ ОТКРЫТА' : 'АФИША · ВШАГЕ';
	const place = placeLine(ev);
	const when = ev.start_time ? whenLine(String(ev.start_time)) : '';

	// Широкая обложка переживает кроп в 1200×630 без потерь — её тянем на
	// весь кадр. Квадрат и вертикаль почти всегда оказываются ПОСТЕРОМ: там
	// уже есть и заголовок, и лицо, и резать их нельзя. Такую обложку
	// показываем целиком справа, а слева ставим свой блок.
	const wide = cover ? cover.width / cover.height >= 1.55 : false;
	// split — единственный признак «текст живёт в узкой колонке». Без
	// отдельного имени сюда легко подставить wide, и событие БЕЗ обложки
	// получает мелкий заголовок в пустом кадре во всю ширину.
	const split = Boolean(cover) && !wide;
	const textWidth = split ? 560 : 1200;

	const layers: unknown[] = [];

	// Фон одинаков для обеих раскладок: фирменное свечение, чтобы поле рядом
	// с постером не читалось как незагрузившаяся картинка.
	layers.push(
		div(
			{
				position: 'absolute',
				top: 0,
				left: 0,
				width: 1200,
				height: 630,
				backgroundImage:
					'radial-gradient(900px 520px at 74% 10%, rgba(255,0,204,0.28) 0%, rgba(10,10,10,0) 70%), radial-gradient(720px 480px at 4% 98%, rgba(0,255,136,0.20) 0%, rgba(10,10,10,0) 70%)'
			},
			''
		)
	);

	if (cover && wide) {
		layers.push({
			type: 'img',
			props: {
				src: cover.uri,
				width: 1200,
				height: 630,
				style: { position: 'absolute', top: 0, left: 0, width: 1200, height: 630, objectFit: 'cover' }
			}
		});
		// Затемнение: без него белый текст на светлой обложке нечитаем, а
		// проверить каждую обложку глазами мы не можем.
		layers.push(
			div(
				{
					position: 'absolute',
					top: 0,
					left: 0,
					width: 1200,
					height: 630,
					backgroundImage:
						'linear-gradient(180deg, rgba(10,10,10,0.70) 0%, rgba(10,10,10,0.38) 34%, rgba(10,10,10,0.94) 100%)'
				},
				''
			)
		);
	} else if (cover) {
		const boxW = 630;
		layers.push({
			type: 'img',
			props: {
				src: cover.uri,
				style: {
					position: 'absolute',
					top: 0,
					right: 0,
					width: boxW,
					height: 630,
					objectFit: 'contain'
				}
			}
		});
		// Мягкий стык: постер не должен упираться в текст обрубленным краем.
		layers.push(
			div(
				{
					position: 'absolute',
					top: 0,
					right: boxW - 1,
					width: 90,
					height: 630,
					backgroundImage: 'linear-gradient(90deg, rgba(10,10,10,0) 0%, rgba(10,10,10,0.55) 100%)'
				},
				''
			)
		);
	}

	// Фирменная полоса сверху — единственный элемент, который остаётся
	// узнаваемым в маленьком превью списка чатов.
	layers.push(
		div({ position: 'absolute', top: 0, left: 0, width: 1200, height: 8, backgroundColor: '#00ff88' }, '')
	);

	layers.push(
		div(
			{
				position: 'absolute',
				top: 0,
				left: 0,
				width: textWidth,
				height: 630,
				display: 'flex',
				flexDirection: 'column',
				justifyContent: 'space-between',
				padding: '54px 46px 52px 60px'
			},
			[
				div(
					{
						display: 'flex',
						fontSize: 20,
						fontWeight: 500,
						letterSpacing: 5,
						color: registering ? '#00ff88' : '#ff00cc',
						textTransform: 'uppercase'
					},
					kicker
				),
				div({ display: 'flex', flexDirection: 'column' }, [
					when
						? div(
								{
									display: 'flex',
									fontSize: split ? 26 : 30,
									fontWeight: 500,
									letterSpacing: 2,
									color: '#00ff88',
									paddingBottom: 16,
									textTransform: 'uppercase'
								},
								when
							)
						: div({ display: 'flex' }, ''),
					div(
						{
							display: 'block',
							fontSize: titleSize(title, !split),
							fontWeight: 800,
							lineHeight: 1.06,
							letterSpacing: -1,
							color: '#ffffff',
							textTransform: 'uppercase'
						},
						title.slice(0, 96)
					),
					place
						? div(
								{
									display: 'flex',
									fontSize: split ? 21 : 26,
									fontWeight: 400,
									color: '#d6d6dd',
									paddingTop: 18
								},
								place.slice(0, split ? 44 : 64)
							)
						: div({ display: 'flex' }, '')
				])
			]
		)
	);

	const svg = await satori(
		div(
			{
				position: 'relative',
				display: 'flex',
				width: 1200,
				height: 630,
				backgroundColor: '#0a0a0a',
				fontFamily: 'Unbounded'
			},
			layers
		) as never,
		{ width: 1200, height: 630, fonts }
	);

	const png = new Resvg(svg).render().asPng();
	const body = png.buffer.slice(png.byteOffset, png.byteOffset + png.byteLength) as ArrayBuffer;
	return new Response(body, {
		headers: {
			'Content-Type': 'image/png',
			'Cache-Control': 'public, max-age=3600, stale-while-revalidate=86400'
		}
	});
};
