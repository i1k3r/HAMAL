import { test, expect } from './support/fixtures';
import { createRoom, closeRoom } from './support/api';

test.describe('room pages', () => {
  test('creator and participant pages render @responsive', async ({ page }) => {
    const room = await createRoom();
    const creator = await page.goto(`${room.base}/c/${room.creator_token}`);
    expect(creator?.status()).toBe(200);
    await expect(page.locator('body')).toHaveAttribute('data-page', 'creator');
    await expect(page.locator('#room-active-card')).toBeVisible();
    await expect(page.locator('#file-list')).toBeAttached();

    const participant = await page.goto(`${room.base}/r/${room.participant_token}`);
    expect(participant?.status()).toBe(200);
    await expect(page.locator('body')).toHaveAttribute('data-page', 'participant');
    await expect(page.locator('#room-active-card')).toBeVisible();
    await expect(page.locator('#dropzone')).toBeVisible();
  });

  test('a closed room answers 410; the participant page shows the end card', async ({ page }) => {
    const room = await createRoom();
    expect(await closeRoom(room)).toBe(200);

    const participant = await page.goto(`${room.base}/r/${room.participant_token}`);
    expect(participant?.status()).toBe(410);
    await expect(page.locator('#room-inactive-card')).toBeVisible();
    await expect(page.locator('#room-active-card')).toBeHidden();

    // Baseline (6765e5e): the creator URL also answers 410, but its HTML is cut short (the
    // template stops rendering inside the sidebar), so no end message is shown. That is a
    // backend defect outside the redesign plan; it is reported, not registered. Only the
    // status code is pinned here.
    const creator = await page.request.get(`${room.base}/c/${room.creator_token}`);
    expect(creator.status()).toBe(410);
  });

  test('an unknown room token answers 404', async ({ page }) => {
    const res = await page.goto('http://127.0.0.1:7799/r/p_doesnotexist0000000000000000000');
    expect(res?.status()).toBe(404);
  });
});
