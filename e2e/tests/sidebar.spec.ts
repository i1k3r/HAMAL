import { test, expect } from './support/fixtures';
import { createRoom, PRIMARY_URL } from './support/api';

/*
 * No page shows the application version in its chrome (the old bottom-left sidebar label was
 * removed in v0.8.14, and the Light Table pages have no sidebar). The version is still available:
 * About on the home page, the keyboard-shortcuts dialog on room pages, and the build metadata.
 */

test('no page shows a version label in its chrome', async ({ page }) => {
  const room = await createRoom();
  for (const url of [`${PRIMARY_URL}/`, `${room.base}/c/${room.creator_token}`, `${room.base}/r/${room.participant_token}`]) {
    await page.goto(url);
    await expect(page.locator('.sidebar-version-tag')).toHaveCount(0);
    await expect(page.locator('.lt-top').first()).not.toContainText(/v\d+\.\d+\.\d+/);
    if (await page.locator('.lt-dock').count()) await expect(page.locator('.lt-dock')).not.toContainText(/v\d+\.\d+\.\d+/);
  }
});

test('the version is shown in About on the home page', async ({ page }) => {
  await page.goto(`${PRIMARY_URL}/`);
  await page.locator('#nav-btn-about').click();
  await expect(page.locator('#modal-about')).toBeVisible();
  await expect(page.locator('#modal-about')).toContainText('HAMAL v0.8.14');
  await page.locator('#modal-about-dismiss').click();
  await expect(page.locator('#modal-about')).toBeHidden();
});
