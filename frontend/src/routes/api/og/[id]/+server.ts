import satori from 'satori';
import { Resvg } from '@resvg/resvg-js';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import type { RequestHandler } from './$types';

// Load Bowlby One TTF once (satori needs TTF/OTF, not woff2) — commit at static/fonts/
let fontDataPromise: Promise<Buffer> | null = null;
function loadFont(): Promise<Buffer> {
  if (!fontDataPromise) {
    const fontPath = path.resolve('static/fonts/BowlbyOne-Regular.ttf');
    fontDataPromise = readFile(fontPath);
  }
  return fontDataPromise;
}

// Simple text-based OG — cyber styled
export const GET: RequestHandler = async ({ params, fetch }) => {
  const backend = process.env.BACKEND_INTERNAL_URL ?? 'http://localhost:3003';
  const res = await fetch(`${backend}/api/events/${encodeURIComponent(params.id)}`);
  if (!res.ok) return new Response('Not found', { status: 404 });
  const ev = await res.json();

  const fontData = await loadFont();

  const svg = await satori(
    {
      type: 'div',
      props: {
        style: {
          display: 'flex', flexDirection: 'column', justifyContent: 'space-between',
          width: '1200px', height: '630px', padding: '60px',
          background: '#0a0a0a', color: '#fafafa',
          fontFamily: 'Bowlby One'
        },
        children: [
          { type: 'div', props: { style: { color: '#00ff88', fontSize: 24, letterSpacing: 4 }, children: 'АФИША · ВШАГЕ' } },
          { type: 'div', props: { style: { fontSize: 120, lineHeight: 0.9, textTransform: 'uppercase', color: '#fff', display: 'block' }, children: (ev.title as string).slice(0, 40) } },
          { type: 'div', props: { style: { fontSize: 28, color: '#ff00cc', letterSpacing: 2 }, children: `${ev.attendee_count} ИДУТ · ${new Date(ev.start_time).toLocaleDateString('ru-RU')}` } }
        ]
      }
    },
    { width: 1200, height: 630, fonts: [{ name: 'Bowlby One', data: fontData, weight: 400, style: 'normal' }] }
  );

  const png = new Resvg(svg).render().asPng();
  // Re-wrap node Buffer into an ArrayBuffer view (web-standard BodyInit)
  const body = png.buffer.slice(png.byteOffset, png.byteOffset + png.byteLength) as ArrayBuffer;
  return new Response(body, {
    headers: {
      'Content-Type': 'image/png',
      'Cache-Control': 'public, max-age=3600, stale-while-revalidate=86400'
    }
  });
};
