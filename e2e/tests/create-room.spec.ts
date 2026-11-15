import { test, expect } from './support/fixtures';
import { PRIMARY_URL } from './support/api';

test.describe('create a room', () => {
  test('the home form creates a room and opens the creator page @responsive', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.locator('#ttl-select').selectOption('900');
    await page.locator('#create-submit-btn').click();
    await page.waitForURL(/\/c\/c_[A-Za-z0-9_-]+$/);
    await expect(page.locator('body')).toHaveAttribute('data-page', 'creator');
    await expect(page.locator('body')).toHaveAttribute('data-pin-required', 'false');
    const expires = Date.parse((await page.locator('body').getAttribute('data-expires')) ?? '');
    const minutes = (expires - Date.now()) / 60_000;
    expect(minutes).toBeGreaterThan(13);
    expect(minutes).toBeLessThanOrEqual(15.1);
  });

  test('a room can be created with a PIN', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.locator('#pin-input').fill('482913');
    await page.locator('#create-submit-btn').click();
    await page.waitForURL(/\/c\//);
    await expect(page.locator('body')).toHaveAttribute('data-pin-required', 'true');
  });

  test('an invalid PIN shows the form message instead of native validation', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await expect(page.locator('#create-room-form')).toHaveAttribute('novalidate', '');
    await page.locator('#pin-input').fill('12a');
    await page.locator('#create-submit-btn').click();
    await expect(page.locator('#form-error')).toBeVisible();
    await expect(page.locator('#form-error')).toHaveText('PIN must be 4 to 8 digits');
    expect(new URL(page.url()).pathname).toBe('/');
  });
});
