// Read-only verification of an actual DEV/local event. Never submits signup.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { chromium, expect } from '@playwright/test';

const url = new URL(process.env.VSHAGE_VIDEO_EVENT_URL ?? '');
assert(['localhost', '127.0.0.1', 'afisha.dev.vshage.app'].includes(url.hostname), 'Use DEV/local only');
const artifacts = process.env.VSHAGE_VIDEO_ARTIFACTS ?? '/tmp/vshage-video-browser';
await fs.mkdir(artifacts, { recursive: true });
const browser = await chromium.launch({ headless: true });
const results = [];
const contexts = [];
const errors = [];
async function context(options = {}) {
  const value = await browser.newContext({ viewport: { width: 1440, height: 800 }, ...options });
  contexts.push(value);
  value.on('page', page => page.on('pageerror', error => errors.push(error.message)));
  return value;
}
const state = video => video.evaluate(v => ({
  time: v.currentTime, paused: v.paused, muted: v.muted, controls: v.controls,
  loop: v.loop, inline: v.playsInline, source: v.getAttribute('src'),
  ready: v.dataset.ready, failed: v.dataset.failed,
}));
async function advancing(video) {
  await expect.poll(async () => (await state(video)).paused).toBe(false);
  const before = (await state(video)).time;
  await expect.poll(async () => Math.abs((await state(video)).time - before), { timeout: 20000 }).toBeGreaterThan(0.15);
}
async function stopped(video) {
  await expect.poll(async () => (await state(video)).paused).toBe(true);
  const before = (await state(video)).time;
  await new Promise(resolve => setTimeout(resolve, 350));
  assert(Math.abs((await state(video)).time - before) < 0.05, 'Paused video must stop advancing');
}

try {
  const desktop = await context({ recordVideo: { dir: path.join(artifacts, 'recordings'), size: { width: 1440, height: 800 } } });
  const page = await desktop.newPage();
  const response = await page.goto(url.href);
  assert.equal(response.status(), 200);
  const video = page.locator('[data-cover-video]').first();
  await advancing(video);
  const initial = await state(video);
  assert(initial.muted && initial.inline && initial.loop && !initial.controls);
  await expect(video).toHaveAttribute('aria-hidden', 'true');
  const poster = page.locator('[data-event-cover] img').first();
  await expect.poll(() => poster.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
  await page.screenshot({ path: path.join(artifacts, 'desktop.png'), fullPage: true });
  await page.getByRole('button', { name: /Остановить анимацию|Pause animation/ }).first().click();
  await stopped(video);
  await page.getByRole('button', { name: /Включить анимацию|Enable animation/ }).first().click();
  await advancing(video);
  await page.evaluate(() => {
    // A short event may fit in the viewport. The fixture only supplies enough
    // scroll distance to move the actual cover completely out of view.
    const spacer = document.createElement('div');
    spacer.id = 'video-check-scroll-space'; spacer.style.height = '100vh';
    document.body.append(spacer);
    window.scrollTo(0, document.body.scrollHeight);
  });
  await stopped(video);
  await page.evaluate(() => { document.getElementById('video-check-scroll-space').remove(); window.scrollTo(0, 0); });
  await advancing(video);
  results.push({ scenario: 'desktop: silent loop, pause/resume, offscreen/resume', pass: true });

  // The poster is the negative control: deliberate network failure must
  // stop playback while retaining the actual image, not an empty rectangle.
  for (const scenario of ['reduced-motion', 'save-data', 'blocked-video']) {
    const fallback = await context(scenario === 'reduced-motion' ? { reducedMotion: 'reduce' } : {});
    if (scenario === 'save-data') {
      await fallback.addInitScript(() => {
        const connection = new EventTarget();
        connection.saveData = true;
        Object.defineProperty(navigator, 'connection', { value: connection, configurable: true });
      });
    }
    if (scenario === 'blocked-video') await fallback.route(/\.mp4(?:\?|$)/, route => route.abort('failed'));
    const tab = await fallback.newPage();
    let mp4Requests = 0;
    tab.on('request', request => { if (/\.mp4(?:\?|$)/.test(request.url())) mp4Requests++; });
    await tab.goto(url.href);
    const clip = tab.locator('[data-cover-video]').first();
    const image = tab.locator('[data-event-cover] img').first();
    await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    if (scenario === 'blocked-video') await expect(clip).toHaveAttribute('data-failed', 'true');
    await stopped(clip);
    assert.equal((await state(clip)).ready, undefined);
    if (scenario !== 'blocked-video') {
      assert.equal(mp4Requests, 0, 'System preference must prevent downloading the MP4');
      assert.equal((await state(clip)).source, null);
    }
    await tab.screenshot({ path: path.join(artifacts, `${scenario}.png`) });
    results.push({ scenario, pass: true, mp4Requests });
    await fallback.close();
  }

  const phone = await context({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
  const mobilePage = await phone.newPage();
  await mobilePage.goto(url.href);
  await advancing(mobilePage.locator('[data-cover-video]').first());
  assert(await mobilePage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'No horizontal overflow');
  await mobilePage.screenshot({ path: path.join(artifacts, 'mobile.png'), fullPage: true });
  results.push({ scenario: 'mobile Chromium viewport: inline playback and layout', pass: true });
  await phone.close();

  // The real listing verifies pointer hit targets after converting the card
  // to an article with a stretched link and a separate motion button.
  await page.goto(`${url.origin}/msk/sport`);
  const card = page.locator('article.card').filter({ has: page.locator(`a[href="${url.pathname}"]`) }).first();
  await expect(card).toBeVisible();
  await card.scrollIntoViewIfNeeded();
  const cardVideo = card.locator('[data-cover-video]');
  await advancing(cardVideo);
  assert(await page.locator('[data-cover-video]').evaluateAll(videos => videos.filter(v => !v.paused).length <= 1));
  const listingUrl = page.url();
  await card.getByRole('button', { name: /Остановить анимацию|Pause animation/ }).click();
  await stopped(cardVideo);
  assert.equal(page.url(), listingUrl, 'Motion toggle must not navigate');
  await card.getByRole('button', { name: /Включить анимацию|Enable animation/ }).click();
  await advancing(cardVideo);
  await page.screenshot({ path: path.join(artifacts, 'listing.png') });
  const photo = card.locator('.photo');
  await photo.evaluate(node => node.scrollIntoView({ block: 'center' }));
  const rect = await photo.boundingBox();
  assert(rect);
  // The stretched anchor deliberately sits over the photo. A pointer click
  // must hit that anchor, rather than forcing a click through it to the div.
  await page.mouse.click(rect.x + rect.width / 2, rect.y + rect.height / 2);
  await expect(page).toHaveURL(url.href);
  await advancing(page.locator('[data-cover-video]').first());
  results.push({ scenario: 'listing: single active clip, motion button, cover navigation', pass: true });
  assert.deepEqual(errors, [], 'No uncaught page errors');
  console.log(JSON.stringify({ url: url.href, results, errors }, null, 2));
} finally {
  for (const value of contexts) await value.close().catch(() => {});
  await browser.close();
  await fs.writeFile(path.join(artifacts, 'result.json'), JSON.stringify({ url: url.href, results, errors }, null, 2));
}
