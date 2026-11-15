import { test, expect } from './support/fixtures';
import { createRoom, mustUpload } from './support/api';

test.describe('mobile layout', () => {
  test('the participant page scrolls vertically @responsive', async ({ page }) => {
    const room = await createRoom();
    for (let i = 0; i < 25; i++) await mustUpload(room, `scroll-${i}.txt`, 'x');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(25);
    const overflow = await page.evaluate(() => ({
      html: getComputedStyle(document.documentElement).overflowY,
      body: getComputedStyle(document.body).overflowY,
    }));
    expect(overflow.html).not.toBe('hidden');
    expect(overflow.body).not.toBe('hidden');
    const last = page.locator('#file-list .file-row').last();
    await last.scrollIntoViewIfNeeded();
    await expect(last).toBeInViewport();
  });
});
