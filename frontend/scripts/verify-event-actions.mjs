// Read-only UI verification. No registration, purchase or Telegram message.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { chromium, webkit, expect } from '@playwright/test';

const base = new URL(process.env.AFISHA_CHECK_BASE ?? 'http://127.0.0.1:4187');
assert(['localhost', '127.0.0.1', 'afisha.dev.vshage.app', 'afisha.vshage.app'].includes(base.hostname));
const artifactRoot = process.env.AFISHA_CHECK_ARTIFACTS ?? '/tmp/afisha-event-actions';
const engine = process.env.AFISHA_CHECK_BROWSER ?? 'chromium';
assert(['chromium', 'webkit'].includes(engine));
const artifacts = path.join(artifactRoot, engine);
await fs.mkdir(artifacts, { recursive: true });
const eventPath = '/ev_afb5e114414a';
const browser = await ({ chromium, webkit })[engine].launch({ headless: true });
const results = [];
try {
  for (const [name, viewport] of [['desktop', { width: 1440, height: 1000 }], ['mobile', { width: 390, height: 844 }]]) {
    const context = await browser.newContext({ viewport });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const response = await page.goto(base.origin + eventPath);
    assert.equal(response.status(), 200);
    const cta = page.locator('a.register-btn');
    await expect(cta).toHaveText('Купить билет ↗');
    await expect(cta).toHaveAttribute('href', 'https://arena.unicornfellowship.ru/#tarif');
    await expect(page.locator('.booking-price')).toContainText('10 000');
    await expect(page.locator('.desc')).toContainText('BIZFAKT');
    await expect(page.locator('.desc')).toContainText('Платина — 40 000');
    const cover = page.locator('[data-event-cover] img');
    await expect.poll(() => cover.evaluate(img => img.complete && img.naturalWidth > img.naturalHeight)).toBe(true);
    await expect(page.locator('.share-options')).not.toHaveAttribute('open');
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    const ctaBox = await cta.boundingBox();
    assert(ctaBox && ctaBox.y + ctaBox.height <= viewport.height, 'Ticket action must be visible before scrolling');
    await page.screenshot({ path: path.join(artifacts, `${name}.png`), fullPage: true });

    await page.locator('.share-options > summary').click();
    const share = page.getByRole('link', { name: 'Отправить друзьям в Telegram', exact: true });
    const href = new URL(await share.getAttribute('href'));
    assert.equal(href.hostname, 't.me');
    assert.equal(href.pathname, '/share/url');
    assert.equal(href.searchParams.get('url'), base.origin + eventPath);
    await page.evaluate(() => {
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: {
        writeText: async value => { window.__copiedEvent = value; }
      } });
    });
    await page.getByRole('button', { name: 'Скопировать ссылку', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Ссылка скопирована', exact: true })).toBeVisible();
    assert.equal(await page.evaluate(() => window.__copiedEvent), base.origin + eventPath);
    await page.screenshot({ path: path.join(artifacts, `${name}-share.png`) });
    assert.deepEqual(errors, []);
    results.push({ viewport: name, ticketAction: true, landscapeCover: true, shareIsSeparate: true, clipboard: true, errors });
    await context.close();
  }

  // Existing prod/local fixtures exercise both other registration modes.
  if (base.hostname !== 'afisha.dev.vshage.app') {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    await page.goto(base.origin + '/b5dbe2c4-b19e-4ee1-8b10-7312fa016b8b');
    await expect(page.locator('a.register-btn')).toHaveText('Записаться в Telegram ↗');
    await expect(page.locator('a.register-btn')).toHaveAttribute('href', 'https://t.me/chernyynuar');
    const video = page.locator('[data-cover-video]').first();
    await expect.poll(() => video.evaluate(v => v.currentTime), { timeout: 20000 }).toBeGreaterThan(.2);
    await expect.poll(() => video.evaluate(v => getComputedStyle(v).opacity)).toBe('1');
    assert.equal(await page.locator('[data-event-cover] button').count(), 0);

    await page.goto(base.origin + '/61a1d69f-6b8b-4579-a486-7f6f6ba3d020');
    await expect(page.locator('a.register-btn')).toHaveAttribute('href', '/events/61a1d69f-6b8b-4579-a486-7f6f6ba3d020/register');
    await page.locator('a.register-btn').click();
    await expect(page.locator('input[name="full_name"]')).toHaveAttribute('required', '');
    await expect(page.locator('input[type="email"]')).toHaveAttribute('required', '');
    await expect(page.locator('input[maxlength="10"]')).toHaveAttribute('required', '');
    results.push({ existingTelegramRegistration: true, silentVideo: true, internalRegistrationRequiredFields: true });
    await page.close();
  }
  console.log(JSON.stringify({ base: base.origin, engine, passed: true, results }, null, 2));
  await fs.writeFile(path.join(artifacts, 'result.json'), JSON.stringify({ base: base.origin, engine, passed: true, results }, null, 2));
} finally { await browser.close(); }
