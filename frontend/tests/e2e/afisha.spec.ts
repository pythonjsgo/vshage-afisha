import { test, expect } from '@playwright/test';

test.describe('Afisha', () => {
  test('homepage loads with nav', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.logo')).toContainText('АФИША_ВШАГЕ');
  });

  test('grid label visible when events exist', async ({ page }) => {
    await page.goto('/');
    const label = page.locator('.grid-label');
    await expect(label.or(page.locator('.empty'))).toBeVisible();
  });
});
