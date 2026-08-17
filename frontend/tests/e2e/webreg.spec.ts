import { expect, test, type Page } from '@playwright/test';

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
 *
 * The form is configurable per event (migration 004), so these tests read the
 * rendered form and assert against THAT, instead of against the one hardcoded
 * shape the form used to have. A suite that knows the field list by heart
 * fails on every correctly-configured event and passes only by accident.
 */
const SLUG = process.env.WEBREG_SLUG ?? '';
const MANAGE_KEY = process.env.WEBREG_MANAGE_KEY ?? '';

/** Which built-in fields this event actually asks for. */
async function formShape(page: Page) {
	const has = async (sel: string) => (await page.locator(sel).count()) > 0;
	return {
		name: await has('input[name="name"]'),
		fullName: await has('input[name="full_name"]'),
		email: await has('input[name="email"]'),
		phone: await has('input[name="phone"]'),
		tg: await has('input[name="tg_username"]'),
		affiliation: await has('select[name="affiliation"]')
	};
}

/**
 * Fills every field the event asks for and returns the value that identifies
 * this registration — the one the dedup key is built from, and the one the
 * CSV must contain exactly once.
 */
async function fillForm(page: Page, id: string): Promise<string> {
	const shape = await formShape(page);
	let identity = '';

	if (shape.name) await page.locator('input[name="name"]').fill('Тест Приёмка');
	if (shape.fullName) await page.locator('input[name="full_name"]').fill('Тестов Тест Тестович');
	if (shape.email) {
		identity = `${id}@example.com`;
		await page.locator('input[name="email"]').fill(identity);
	}
	if (shape.phone) await page.locator('input[name="phone"]').fill('+7 903 000-00-00');
	if (shape.tg) {
		const tg = `@${id}`;
		if (!identity) identity = tg;
		await page.locator('input[name="tg_username"]').fill(tg);
	}
	if (shape.affiliation) {
		await page.locator('select[name="affiliation"]').selectOption({ index: 1 });
	}

	for (const select of await page.locator('select[name^="answer:"]').all()) {
		await select.selectOption({ index: 1 });
	}
	for (const input of await page.locator('input[name^="answer:"]').all()) {
		if ((await input.getAttribute('type')) === 'checkbox') continue;
		await input.fill('e2e');
	}

	await page.locator('input[name="consent"]').check();
	return identity;
}

test.describe('web registration', () => {
	test.skip(!SLUG, 'WEBREG_SLUG not set');

	// Unique per run: registrations dedupe on the identity field, so a fixed
	// one would silently exercise the update path on the second run.
	const id = `e2e${Date.now().toString(36)}`;

	// A 200 from the HTML endpoint says nothing about whether the page is
	// usable. On DEV the whole app rendered as unstyled HTML with no client JS
	// because /_app/* was not routed to this service — every assertion below
	// still passed, and only a screenshot gave it away. This test is the
	// instrument for that failure: it fails on any asset the page asks for and
	// does not get.
	test('the page gets every asset it asks for', async ({ page }) => {
		const broken: string[] = [];
		page.on('response', (r) => {
			if (r.status() >= 400) broken.push(`${r.status()} ${r.url()}`);
		});
		page.on('requestfailed', (r) => broken.push(`failed ${r.url()}`));

		await page.goto(`/e/${SLUG}`, { waitUntil: 'networkidle' });

		// Ignore third-party hosts (web fonts): only our own origin is ours to fix.
		const ours = broken.filter((b) => b.includes(new URL(page.url()).host));
		expect(ours, `битые ресурсы:\n${ours.join('\n')}`).toEqual([]);

		// And the styles actually took effect, not merely downloaded.
		const font = await page
			.locator('h1')
			.evaluate((el) => getComputedStyle(el).fontFamily.toLowerCase());
		expect(font, 'заголовок отрисован системным шрифтом — CSS не применился').toContain('bowlby');
	});

	// Разделение ссылок (директива 17.08): /e/<slug> — короткая страница для
	// лички, /<slug> в афише — витрина. Тест держит именно РАЗНИЦУ: если
	// обложка вернётся на страницу регистрации или карточка афиши начнёт
	// вести на неё же, оба экрана снова станут одним.
	test('registration page is the short one: no cover on screen', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);

		// Обложка остаётся в og:image (превью в Телеграме), но не на экране.
		await expect(page.locator('img.cover')).toHaveCount(0);
		await expect(page.locator('meta[property="og:title"]')).toHaveCount(1);
		await expect(page.locator('h1')).toBeVisible();
		await expect(page.locator('#reg')).toBeAttached();
	});

	test('afisha card is a separate page that hands off to the form', async ({ page }) => {
		// Идём по ссылке, которую рисует сама страница, а не по угаданному
		// адресу: /e/ отдаётся и с vshage.app, и с поддомена афиши, а карточка
		// живёт только на поддомене. Собранный вручную URL дал бы 404 на
		// «правильном» стенде, и тест молча пропустился бы как «снято с афиши».
		await page.goto(`/e/${SLUG}`);
		const link = page.getByRole('link', { name: 'Открыть в афише' });
		if ((await link.count()) === 0) test.skip(true, 'событие не опубликовано в афише');

		const href = await link.getAttribute('href');
		expect(href, 'ссылка в афишу пустая').toBeTruthy();

		const res = await page.goto(href!);
		expect(res?.status()).toBe(200);
		await expect(page.locator('h1')).toBeVisible();

		// Кнопка регистрации ведёт на короткую страницу, а не на старый
		// /events/<uuid>/register — именно та несогласованность, которую чиним.
		const registerHref = await page
			.getByRole('link', { name: 'ЗАРЕГИСТРИРОВАТЬСЯ' })
			.getAttribute('href');
		expect(registerHref).toBe(`/e/${SLUG}`);
	});

	test('the form asks for what the event configured, and nothing else', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);
		const shape = await formShape(page);

		// Every event needs at least one way to tell one visitor from another;
		// without it the whole list dedupes to noise.
		expect(shape.email || shape.tg || shape.phone, 'событие не спрашивает ни одного контакта').toBe(
			true
		);

		// Consent is not configurable — it is the legal basis for storing any
		// of the above, so it must be present on every event.
		await expect(page.locator('input[name="consent"]')).toBeAttached();
		await expect(page.getByText('Когда')).toBeVisible();
	});

	test('registration succeeds and lands on the done screen', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);
		await fillForm(page, id);
		await page.locator('button.submit').click();

		await expect(page).toHaveURL(/done=1/);
		await expect(page.getByText('Ты в списке')).toBeVisible();
	});

	test('re-submitting the same person is idempotent, not a duplicate', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);
		// Uppercased: the identity is normalised before it becomes the dedup
		// key, so a different casing is the SAME person.
		await fillForm(page, id.toUpperCase());
		await page.locator('button.submit').click();

		await expect(page).toHaveURL(/again=1/);
		await expect(page.getByText('Ты уже в списке')).toBeVisible();
	});

	test('a bad Telegram username explains how to fix it', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);
		const shape = await formShape(page);
		test.skip(!shape.tg, 'событие не спрашивает телеграм');

		await fillForm(page, `${id}bad`);
		await page.locator('input[name="tg_username"]').fill('иванов');
		await page.locator('button.submit').click();

		// The message must tell the visitor what to do, not just say "invalid".
		await expect(page.getByText(/Настройки → Имя пользователя/)).toBeVisible();
		// And it must not blank what they already typed.
		if (shape.name) await expect(page.locator('input[name="name"]')).toHaveValue('Тест Приёмка');
	});

	test('a bad email explains how to fix it', async ({ page }) => {
		await page.goto(`/e/${SLUG}`);
		const shape = await formShape(page);
		test.skip(!shape.email, 'событие не спрашивает почту');

		await fillForm(page, `${id}bade`);
		await page.locator('input[name="email"]').fill('нет-собаки.example.com');
		await page.locator('button.submit').click();

		await expect(page.getByText(/Проверь адрес/)).toBeVisible();
	});

	test('manage page is gated by the secret key', async ({ page }) => {
		test.skip(!MANAGE_KEY, 'WEBREG_MANAGE_KEY not set');

		const wrong = await page.goto(`/e/${SLUG}/manage?key=definitely-wrong`);
		expect(wrong?.status()).toBe(404);

		const missing = await page.goto(`/e/${SLUG}/manage`);
		expect(missing?.status()).toBe(404);

		await page.goto(`/e/${SLUG}/manage?key=${encodeURIComponent(MANAGE_KEY)}`);
		await expect(page.getByText('Регистрации')).toBeVisible();
		await expect(page.getByRole('link', { name: 'Скачать CSV' })).toBeVisible();
	});

	test('CSV export downloads with the registration in it exactly once', async ({ page, request }) => {
		test.skip(!MANAGE_KEY, 'WEBREG_MANAGE_KEY not set');

		// The identity depends on the event's config, so ask the page rather
		// than assuming it.
		await page.goto(`/e/${SLUG}`);
		const shape = await formShape(page);
		const identity = shape.email ? `${id}@example.com` : `${id}`;

		const res = await request.get(
			`/e/${SLUG}/manage/export?key=${encodeURIComponent(MANAGE_KEY)}`
		);
		expect(res.status()).toBe(200);
		expect(res.headers()['content-disposition']).toContain('attachment');

		const body = await res.text();
		// BOM first — without it Excel renders the Cyrillic as mojibake.
		expect(body.charCodeAt(0)).toBe(0xfeff);
		expect(body).toContain('Время регистрации');

		// Dedup holds end to end: one person, one row, however they typed it.
		const lower = body.toLowerCase();
		const occurrences = lower.split(identity.toLowerCase()).length - 1;
		expect(occurrences, `«${identity}» встречается ${occurrences} раз вместо одного`).toBe(1);
	});
});
