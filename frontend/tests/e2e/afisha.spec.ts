import { test, expect } from '@playwright/test';

/**
 * e2e афиши. Гоняется руками против стенда (в CI его нет):
 *   BASE_URL=https://afisha.dev.vshage.app npx playwright test
 *
 * Проверяет то, чего не видит ни один прибор уровнем ниже: что страница
 * действительно СОБИРАЕТСЯ из бэкенда, разделов и разметки. Модульные тесты
 * утверждают функции, svelte-check — типы, а «раздел открывается и в нём те
 * события» не утверждает никто, кроме этого файла.
 */

test.describe('Afisha', () => {
  test('homepage loads with nav', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.logo')).toContainText('АФИША_ВШАГЕ');
  });

  test('grid label visible when events exist', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.grid-label')).toBeVisible();
  });

  test('корень уводит на город постоянным редиректом', async ({ page }) => {
    const res = await page.goto('/');
    await expect(page).toHaveURL(/\/msk$/);
    // Именно 308, а не 302: адрес города канонический, и временный редирект
    // оставил бы поисковику прежнюю главную как основную страницу сайта.
    // Код берём у ПЕРВОГО запроса цепочки — у конечного он уже 200.
    const from = res?.request().redirectedFrom();
    expect(from, 'редиректа с корня не было вовсе').not.toBeNull();
    expect((await from!.response())?.status()).toBe(308);
  });

  test('у страницы города ровно один h1 и он про город', async ({ page }) => {
    await page.goto('/msk');
    await expect(page.locator('h1')).toHaveCount(1);
    // Заголовок сравниваем по textContent, а не innerText: у него
    // text-transform: uppercase, и innerText вернул бы «АФИША МОСКВЫ» —
    // прибор упал бы на собственном форматировании, а не на коде.
    expect(await page.locator('h1').textContent()).toContain('Москв');
  });

  test('плитка ведёт в раздел, и раздел показывает свою категорию', async ({ page }) => {
    await page.goto('/msk');
    const tile = page.locator('a[href="/msk/concert"]').first();
    await expect(tile).toBeVisible();
    await tile.click();
    await expect(page).toHaveURL(/\/msk\/concert$/);
    await expect(page.locator('h1')).toContainText('Концерты');
  });

  test('неизвестный раздел и неизвестный город — 404, а не пустая лента', async ({ page }) => {
    expect((await page.goto('/msk/nesushchestvuet'))?.status()).toBe(404);
    expect((await page.goto('/spb'))?.status()).toBe(404);
  });

  test('карточка события несёт разметку для поисковика', async ({ page }) => {
    await page.goto('/msk');
    const first = page.locator('.grid a[href^="/"]').first();
    const href = await first.getAttribute('href');
    await page.goto(href!);
    const ld = page.locator('script[type="application/ld+json"]');
    await expect(ld.first()).toHaveCount(1);
    const types = await ld.allTextContents();
    expect(types.some((t) => JSON.parse(t)['@type'] === 'Event')).toBe(true);
    await expect(page.locator('link[rel="canonical"]')).toHaveCount(1);
  });

  test('карта сайта полна и ведёт на живые адреса', async ({ page, request }) => {
    const res = await request.get('/sitemap.xml');
    // 503 здесь — это НЕ «сервис лежит», а «карта собралась неполной»
    // (см. sitemap.xml/+server.ts). Для прибора это провал: обрезанная карта
    // выкидывает из индекса всё, чего в ней не оказалось.
    expect(res.status(), 'карта неполная — бэкенд не отдал все страницы').toBe(200);
    const xml = await res.text();
    expect(xml).toContain('<urlset');
    expect(xml).toContain('/msk/concert');

    const locs = [...xml.matchAll(/<loc>([^<]+)<\/loc>/g)].map((m) => m[1]);
    const eventLocs = locs.filter((u) => !/\/msk(\/[a-z_]+)?$/.test(u) && !u.endsWith('/'));
    // Утверждение, а не условие. Прежняя версия делала `if (some) …` и была
    // зелёной ровно в том отказе, ради которого написана: при пустой карте
    // проверять становилось нечего, и тест это устраивало. Это записанная у
    // нас ловушка «тест умеет пропуститься вместо провала».
    expect(eventLocs.length, 'в карте нет ни одного события').toBeGreaterThan(0);
    // Карта, ведущая в 404, хуже отсутствующей — робот тратит на неё обход.
    expect((await request.get(eventLocs[0])).status()).toBe(200);
  });
});
