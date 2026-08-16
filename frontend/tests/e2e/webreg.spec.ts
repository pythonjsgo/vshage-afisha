import { expect, test } from '@playwright/test';

/**
 * End-to-end cover for the web registration flow at /e/<slug>.
 *
 * Requires a running afisha-backend with an event seeded under WEBREG_SLUG.
 * Run it against a local stand:
 *
 *   BASE_URL=http://127.0.0.1:5183 \
 *   WEBREG_SLUG=shag WEBREG_MANAGE_KEY=… \
 *   npx playwright test tests/e2e/webreg.spec.ts
 *
 * Skipped when WEBREG_SLUG is unset so the default suite stays green without
 * a seeded event.
 */
const SLUG = process.env.WEBREG_SLUG ?? '';
const MANAGE_KEY = process.env.WEBREG_MANAGE_KEY ?? '';

test.describe('web registration', () => {
	test.skip(!SLUG, 'WEBREG_SLUG not set');

	// Unique per run: registrations dedupe on the Telegram username, so a
	// fixed one would silently exercise the update path on the second run.
	const username = `e2e_${Date.now().toString(36)}`;

	test('event page shows the essentials before any scrolling decision', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);

		await expect(page.locator('h1')).toBeVisible();
		// When / where / the form are the three things a visitor came for.
		await expect(page.getByText('Когда')).toBeVisible();
		await expect(page.locator('#reg')).toBeAttached();
		await expect(page.locator('input[name="name"]')).toBeAttached();
		await expect(page.locator('input[name="tg_username"]')).toBeAttached();
		await expect(page.locator('select[name="affiliation"]')).toBeAttached();

		// The Telegram share preview is the entry point for ~90% of visitors.
		await expect(page.locator('meta[property="og:title"]')).toHaveCount(1);
	});

	test('registration succeeds and lands on the done screen', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);

		await page.locator('input[name="name"]').fill('Тест Приёмка');
		await page.locator('input[name="tg_username"]').fill(`@${username}`);
		await page.locator('select[name="affiliation"]').selectOption({ index: 1 });

		// Fill whatever custom fields the organizer configured.
		for (const select of await page.locator('select[name^="answer:"]').all()) {
			await select.selectOption({ index: 1 });
		}
		for (const input of await page.locator('input[name^="answer:"]').all()) {
			if ((await input.getAttribute('type')) === 'checkbox') continue;
			await input.fill('e2e');
		}

		await page.locator('input[name="consent"]').check();
		await page.getByRole('button', { name: 'Иду' }).click();

		await expect(page).toHaveURL(/done=1/);
		await expect(page.getByText('Ты в списке')).toBeVisible();
	});

	test('re-submitting the same username is idempotent, not a duplicate', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);

		await page.locator('input[name="name"]').fill('Тест Приёмка');
		await page.locator('input[name="tg_username"]').fill(username.toUpperCase());
		await page.locator('select[name="affiliation"]').selectOption({ index: 1 });
		for (const select of await page.locator('select[name^="answer:"]').all()) {
			await select.selectOption({ index: 1 });
		}
		for (const input of await page.locator('input[name^="answer:"]').all()) {
			if ((await input.getAttribute('type')) === 'checkbox') continue;
			await input.fill('e2e');
		}
		await page.locator('input[name="consent"]').check();
		await page.getByRole('button', { name: 'Иду' }).click();

		// Same person, different casing — recognised, not re-added.
		await expect(page).toHaveURL(/again=1/);
		await expect(page.getByText('Ты уже в списке')).toBeVisible();
	});

	test('a bad Telegram username explains how to fix it', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);

		await page.locator('input[name="name"]').fill('Тест');
		await page.locator('input[name="tg_username"]').fill('иванов');
		await page.locator('select[name="affiliation"]').selectOption({ index: 1 });
		for (const select of await page.locator('select[name^="answer:"]').all()) {
			await select.selectOption({ index: 1 });
		}
		await page.locator('input[name="consent"]').check();
		await page.getByRole('button', { name: 'Иду' }).click();

		// The message must tell the visitor what to do, not just say "invalid".
		await expect(page.getByText(/Настройки → Имя пользователя/)).toBeVisible();
		// And it must not blank the name they already typed.
		await expect(page.locator('input[name="name"]')).toHaveValue('Тест');
	});

	test('manage page is gated by the secret key', async ({ page }) => {
		test.skip(!MANAGE_KEY, 'WEBREG_MANAGE_KEY not set');

		const wrong = await page.goto(`/e/${SLUG}/manage?key=definitely-wrong`);
		expect(wrong?.status()).toBe(404);

		const missing = await page.goto(`/e/${SLUG}/manage`);
		expect(missing?.status()).toBe(404);

		await page.goto(`/e/${SLUG}/manage?key=${encodeURIComponent(MANAGE_KEY)}`);
		await expect(page.getByText('Регистрации')).toBeVisible();
		await expect(page.getByText(`@${username}`, { exact: false })).toBeVisible();
		await expect(page.getByRole('link', { name: 'Скачать CSV' })).toBeVisible();
	});

	test('CSV export downloads with the registrations in it', async ({ request }) => {
		test.skip(!MANAGE_KEY, 'WEBREG_MANAGE_KEY not set');

		const res = await request.get(
			`/e/${SLUG}/manage/export?key=${encodeURIComponent(MANAGE_KEY)}`
		);
		expect(res.status()).toBe(200);
		expect(res.headers()['content-disposition']).toContain('attachment');

		const body = await res.text();
		// BOM first — without it Excel renders the Cyrillic as mojibake.
		expect(body.charCodeAt(0)).toBe(0xfeff);
		expect(body).toContain('Имя');
		// Case-insensitive: the export shows the username as the visitor typed
		// it (the idempotency test above re-submitted it upper-cased). Only the
		// hidden dedup key is lower-cased.
		expect(body.toLowerCase()).toContain(username.toLowerCase());
		// Dedup holds end to end: one person, one row, however they typed it.
		const occurrences = body.toLowerCase().split(username.toLowerCase()).length - 1;
		expect(occurrences).toBe(1);
	});
});
