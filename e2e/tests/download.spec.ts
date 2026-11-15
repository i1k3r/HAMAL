import { test, expect } from './support/fixtures';
import { createRoom, mustUpload } from './support/api';
import { waitForPolls } from './support/page';
import fs from 'node:fs';

test.describe('download', () => {
  test('download keeps the original filename and content, and is marked for the session', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'Quarterly Report (final).txt', 'numbers go here');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await waitForPolls(page, 1); // template-rendered rows get their handlers on the first poll
    const row = page.locator('#file-list .file-row', { hasText: 'Quarterly Report (final).txt' });
    await expect(row).toBeVisible();

    const [download] = await Promise.all([page.waitForEvent('download'), row.locator('a[download]').click()]);
    expect(download.suggestedFilename()).toBe('Quarterly Report (final).txt');
    expect(fs.readFileSync(await download.path(), 'utf8')).toBe('numbers go here');
    await expect(row.locator('a[download]')).toHaveText('Save again');

    // The mark lives in sessionStorage, so it survives a reload of the same tab.
    await page.reload();
    await expect(page.locator('#file-list .file-row', { hasText: 'Quarterly Report (final).txt' }).locator('a[download]')).toHaveText('Save again');
  });

  test('risky file types carry the executable warning', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'setup.exe', 'MZ');
    await mustUpload(room, 'notes.txt', 'plain');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    const exe = page.locator('#file-list .file-row', { hasText: 'setup.exe' });
    await expect(exe.locator('.badge-exec-warning')).toHaveAttribute(
      'aria-label',
      'Potentially executable file. Only open or install files you trust.',
    );
    await expect(page.locator('#file-list .file-row', { hasText: 'notes.txt' }).locator('.badge-exec-warning')).toHaveCount(0);
  });
});
