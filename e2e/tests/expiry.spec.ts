import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('expiry', () => {
  test('the countdown runs, the room expires, and polling stops', async ({ page }) => {
    test.setTimeout(120_000);
    const room = await createRoom({ ttl: 60 });
    await page.goto(`${room.base}/c/${room.creator_token}`);
    const countdown = page.locator('#countdown');
    await expect(countdown).toHaveText(/^0?0:5\d|^00:[0-5]\d|^0:[0-5]\d/);
    const first = await countdown.textContent();
    await page.waitForTimeout(2_000);
    expect(await countdown.textContent()).not.toBe(first);

    await expect(page.locator('#room-inactive-card')).toBeVisible({ timeout: 75_000 });
    await expect(page.locator('#inactive-title')).toHaveText(/Room (Expired|Inactive|Closed)/);

    let apiCalls = 0;
    page.on('request', (r) => {
      if (r.url().includes('/api/v1/rooms/')) apiCalls++;
    });
    await page.waitForTimeout(8_000);
    expect(apiCalls, 'polling stops once the room has ended').toBe(0);
  });
});
