import { test, expect } from './support/fixtures';
import { createRoom, mustUpload } from './support/api';

test.describe('close', () => {
  test('the creator closes the room and the participant sees it closed', async ({ page, browser }) => {
    const room = await createRoom();
    const phone = await browser.newPage();
    await phone.goto(`${room.base}/r/${room.participant_token}`);
    await expect(phone.locator('#close-room-btn')).toHaveCount(0);

    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#close-room-btn').click();
    await expect(page.locator('#modal-close-confirm')).toBeVisible();
    await page.locator('#confirm-close-btn').click();
    await expect(page.locator('#inactive-title')).toHaveText('Room Closed');
    await expect(page.locator('#room-active-card')).toBeHidden();

    await expect(phone.locator('#room-inactive-card')).toBeVisible({ timeout: 12_000 });
    await expect(phone.locator('#inactive-title')).toHaveText(/Room (Closed|Inactive)/);
    await phone.close();
  });

  test('a participant cannot close the room through the API', async ({ request }) => {
    const room = await createRoom();
    const res = await request.post(`${room.base}/api/v1/rooms/${room.participant_token}/close`);
    expect(res.status()).toBe(403);
    const still = await request.get(`${room.base}/api/v1/rooms/${room.participant_token}`);
    expect(still.status()).toBe(200);
  });

  test('cancel keeps the room open', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'kept.txt', 'k');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#close-room-btn').click();
    await page.locator('#modal-close-cancel').click();
    await expect(page.locator('#modal-close-confirm')).toBeHidden();
    await expect(page.locator('#room-active-card')).toBeVisible();
  });
});
