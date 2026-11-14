import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('presence', () => {
  test('the creator sees connected participants with a device name', async ({ page, browser }) => {
    const room = await createRoom();
    const phone = await browser.newPage();
    await phone.goto(`${room.base}/r/${room.participant_token}`);
    await page.goto(`${room.base}/c/${room.creator_token}`);
    // Presence is keyed by client IP; every test browser is 127.0.0.1, so one entry is expected.
    await expect(page.locator('#participant-count')).toHaveText(/1/, { timeout: 12_000 });
    await expect(page.locator('#participant-list')).toContainText(/Device|Desktop|Phone|Linux|Windows|macOS|Android|iPhone/);
    await expect(phone.locator('#participant-count-val')).toHaveText(/^\d+$/, { timeout: 12_000 });
    await phone.close();
  });
});
