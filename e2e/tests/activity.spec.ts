import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('recent activity', () => {
  test('uploads and downloads appear in recent activity, and Clear All empties it', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#file-input').setInputFiles({ name: 'logged.txt', mimeType: 'text/plain', buffer: Buffer.from('l') });
    await expect(page.locator('#activity-list')).toContainText('logged.txt', { timeout: 12_000 });

    const row = page.locator('#file-list .file-row', { hasText: 'logged.txt' });
    await Promise.all([page.waitForEvent('download'), row.locator('a[download]').click()]);
    await expect(page.locator('#activity-list')).toContainText('Download started: logged.txt');

    await page.locator('#btn-clear-activity').click();
    await expect(page.locator('#activity-list')).toContainText('No transfer activity yet');
  });
});
