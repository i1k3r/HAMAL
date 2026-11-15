import { test, expect } from './support/fixtures';
import { createRoom, getJSON, mustUpload, PNG_1PX, wav, type Room } from './support/api';
import { pasteImage } from './support/page';
import fs from 'node:fs';
import path from 'node:path';

/*
 * Clipboard image paste and media previews, in both the creator and the participant view
 * (feature from 6765e5e). The participant paste and preview basics live in upload.spec.ts and
 * preview.spec.ts; this spec covers the creator view, every way of dismissing the preview, and
 * that ordinary text pasting is unaffected.
 */

const clip = fs.readFileSync(path.join(__dirname, '..', 'fixtures', 'media', 'clip.webm'));

const VIEWS = [
  { name: 'creator', url: (r: Room) => `${r.base}/c/${r.creator_token}` },
  { name: 'participant', url: (r: Room) => `${r.base}/r/${r.participant_token}` },
] as const;

async function serverFiles(room: Room): Promise<any[]> {
  const r = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
  return r.json.files ?? [];
}

for (const view of VIEWS) {
  test.describe(`paste and preview (${view.name})`, () => {
    test(`a pasted image goes through the normal upload flow and downloads intact (${view.name})`, async ({ page }) => {
      const room = await createRoom();
      await page.goto(view.url(room));
      await pasteImage(page, null);
      await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 12_000 });
      const files = await serverFiles(room);
      expect(files).toHaveLength(1);
      expect(files[0].filename).toMatch(/^pasted-image-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.png$/);
      // Download it through the page's own download link.
      const row = page.locator('#file-list .file-row', { hasText: files[0].filename });
      const [download] = await Promise.all([page.waitForEvent('download'), row.locator('a[download]').click()]);
      expect(download.suggestedFilename()).toBe(files[0].filename);
      expect(fs.readFileSync(await download.path()).equals(PNG_1PX), 'downloaded bytes equal the pasted image').toBe(true);
      // The uploaded image is previewable like any other image.
      await page.getByRole('button', { name: `Preview ${files[0].filename}` }).click();
      await expect(page.locator('#preview-body img')).toBeVisible();
    });

    test(`pasting text into the note field inserts text and uploads nothing (${view.name})`, async ({ page, context, browserName }) => {
      test.skip(browserName !== 'chromium', 'clipboard permissions are Chromium-only in Playwright');
      await context.grantPermissions(['clipboard-read', 'clipboard-write']);
      const room = await createRoom();
      await page.goto(view.url(room));
      await page.locator('#tab-text').click();
      const input = page.locator('#text-input');
      await input.click();
      await page.evaluate(() => navigator.clipboard.writeText('pasted line 1\npasted line 2'));
      await page.keyboard.press('ControlOrMeta+V');
      await expect(input).toHaveValue('pasted line 1\npasted line 2');
      await page.locator('#send-text-btn').click();
      await expect(page.locator('#text-list .text-item-card').first()).toContainText('pasted line 2', { timeout: 12_000 });
      await page.waitForTimeout(500);
      expect(await serverFiles(room)).toEqual([]);
    });

    test(`a text-only paste outside a field uploads nothing (${view.name})`, async ({ page }) => {
      const room = await createRoom();
      await page.goto(view.url(room));
      await page.evaluate(() => {
        const dt = new DataTransfer();
        dt.setData('text/plain', 'just words');
        document.body.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
      });
      await page.waitForTimeout(1_000);
      expect(await serverFiles(room)).toEqual([]);
      await expect(page.locator('#file-list .file-row')).toHaveCount(0);
    });

    test(`image, video and audio previews open (${view.name})`, async ({ page }) => {
      const room = await createRoom();
      await mustUpload(room, 'pixel.png', PNG_1PX, { type: 'image/png' });
      await mustUpload(room, 'clip.webm', clip, { type: 'video/webm' });
      await mustUpload(room, 'tone.wav', wav(2), { type: 'audio/wav' });
      await page.goto(view.url(room));
      const overlay = page.locator('#file-preview-overlay');

      await page.getByRole('button', { name: 'Preview pixel.png' }).click();
      await expect(overlay).toBeVisible();
      await expect(page.locator('#preview-filename')).toHaveText('pixel.png');
      const img = page.locator('#preview-body img');
      await expect.poll(() => img.evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1);
      await page.keyboard.press('Escape');
      await expect(overlay).toBeHidden();

      await page.getByRole('button', { name: 'Preview clip.webm' }).click();
      const video = page.locator('#preview-body video');
      await expect(video).toHaveAttribute('controls', '');
      await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThanOrEqual(1);
      await page.keyboard.press('Escape');
      await expect(overlay).toBeHidden();

      await page.getByRole('button', { name: 'Preview tone.wav' }).click();
      const audio = page.locator('#preview-body audio');
      await expect(audio).toHaveAttribute('controls', '');
      await expect(page.locator('#preview-filename')).toHaveText('tone.wav');
      await page.keyboard.press('Escape');
      await expect(overlay).toBeHidden();
    });

    test(`the preview closes with Escape, the close button and the backdrop (${view.name})`, async ({ page }) => {
      const room = await createRoom();
      await mustUpload(room, 'clip.webm', clip, { type: 'video/webm' });
      await page.goto(view.url(room));
      const overlay = page.locator('#file-preview-overlay');
      const open = async () => {
        await page.getByRole('button', { name: 'Preview clip.webm' }).click();
        await expect(overlay).toBeVisible();
        await expect(page.locator('#preview-body video')).toHaveCount(1);
        expect(await page.evaluate(() => document.body.style.overflow)).toBe('hidden');
      };
      const expectClosed = async () => {
        await expect(overlay).toBeHidden();
        await expect(page.locator('#preview-body')).toBeEmpty();
        expect(await page.evaluate(() => document.body.style.overflow)).toBe('');
      };

      await open();
      await page.keyboard.press('Escape');
      await expectClosed();

      await open();
      await page.locator('#preview-close-btn').click();
      await expectClosed();

      await open();
      // A click inside the dialog content does not close it.
      await page.locator('#preview-filename').click();
      await expect(overlay).toBeVisible();
      // A click on the backdrop (the overlay itself, outside the dialog) does.
      await overlay.click({ position: { x: 5, y: 5 } });
      await expectClosed();
    });
  });
}
