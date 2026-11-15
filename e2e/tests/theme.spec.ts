import { test, expect } from './support/fixtures';
import { createRoom, PRIMARY_URL } from './support/api';

test.describe('theme', () => {
  test('the theme choice is saved and applies on every page', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.locator('#theme-btn-light').click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    expect(await page.evaluate(() => localStorage.getItem('hamal_theme'))).toBe('light');
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await page.locator('#theme-btn-dark').click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    expect(await page.evaluate(() => localStorage.getItem('hamal_theme'))).toBe('dark');
  });
});
