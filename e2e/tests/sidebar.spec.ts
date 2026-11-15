import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

/*
 * The bottom-left sidebar no longer shows the application version (v0.8.14 release). The footer
 * row keeps the theme switch on the left and the settings button on the right, with nothing
 * between them. The version stays available in Settings > About and in the build metadata.
 */

const PAGES = [
  { name: 'home', url: (_: any) => 'http://127.0.0.1:7799/' },
] as const;

for (const p of PAGES) {
  test(`the sidebar footer shows no version label (${p.name})`, async ({ page }) => {
    const room = await createRoom();
    await page.goto(p.url(room));
    await expect(page.locator('.sidebar-version-tag')).toHaveCount(0);
    const row = page.locator('.sidebar-footer-row');
    await expect(row).toBeVisible();
    await expect(row).not.toContainText(/v\d+\.\d+\.\d+/);
    // Exactly the theme switch and the settings button, pinned to opposite edges.
    expect(await row.evaluate((el) => Array.from(el.children).map((c) => c.id || c.className))).toEqual([
      'theme-toggle-pill',
      'nav-btn-settings',
    ]);
    const rowBox = (await row.boundingBox())!;
    const pill = (await row.locator('.theme-toggle-pill').boundingBox())!;
    const settings = (await page.locator('#nav-btn-settings').boundingBox())!;
    expect(Math.abs(pill.x - rowBox.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(settings.x + settings.width - (rowBox.x + rowBox.width))).toBeLessThanOrEqual(1);
    // The row is no taller than its tallest control: no empty line was left behind.
    expect(rowBox.height).toBeLessThanOrEqual(Math.max(pill.height, settings.height) + 1);
  });
}

test('the Light Table room pages show no version label in their chrome', async ({ page }) => {
  const room = await createRoom();
  for (const url of [`${room.base}/c/${room.creator_token}`, `${room.base}/r/${room.participant_token}`]) {
    await page.goto(url);
    await expect(page.locator('.sidebar-version-tag')).toHaveCount(0);
    await expect(page.locator('#room-active-card .lt-top')).not.toContainText(/v\d+\.\d+\.\d+/);
    await expect(page.locator('.lt-dock')).not.toContainText(/v\d+\.\d+\.\d+/);
  }
});
