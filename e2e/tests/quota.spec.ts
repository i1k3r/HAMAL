import { test, expect } from './support/fixtures';
import { createRoom, mustUpload, LIMITED_URL } from './support/api';

// Runs against the "limited" server: max file 1 MiB, max room 3 MiB, max 5 files per room.

test.describe('quotas', () => {
  test('a file over the per-file limit is refused with a visible message', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#file-input').setInputFiles({
      name: 'too-big.bin',
      mimeType: 'application/octet-stream',
      buffer: Buffer.alloc(1024 * 1024 + 1),
    });
    await expect(page.locator('#upload-error')).toContainText('too-big.bin:', { timeout: 12_000 });
    await expect(page.locator('#file-list .file-row')).toHaveCount(0);
  });

  test('the room size limit refuses the file that would exceed it', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    await mustUpload(room, 'a.bin', Buffer.alloc(1000 * 1024));
    await mustUpload(room, 'b.bin', Buffer.alloc(1000 * 1024));
    await mustUpload(room, 'c.bin', Buffer.alloc(1000 * 1024));
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(3);
    await page.locator('#file-input').setInputFiles({
      name: 'overflow.bin',
      mimeType: 'application/octet-stream',
      buffer: Buffer.alloc(200 * 1024),
    });
    await expect(page.locator('#upload-error')).toContainText('overflow.bin:', { timeout: 12_000 });
    await expect(page.locator('#file-list .file-row')).toHaveCount(3);
  });

  test('the file count limit refuses the sixth file', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    for (let i = 0; i < 5; i++) await mustUpload(room, `f${i}.txt`, 'x');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(5);
    await page.locator('#file-input').setInputFiles({ name: 'sixth.txt', mimeType: 'text/plain', buffer: Buffer.from('6') });
    await expect(page.locator('#upload-error')).toContainText('sixth.txt:', { timeout: 12_000 });
    await expect(page.locator('#file-list .file-row')).toHaveCount(5);
  });
});
