import { test, expect } from './support/fixtures';
import { createRoom, getJSON } from './support/api';
import { dropFiles, pasteImage } from './support/page';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

async function serverFiles(room: { base: string; participant_token: string }): Promise<string[]> {
  const r = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
  return (r.json.files ?? []).map((f: any) => f.filename).sort();
}

test.describe('upload', () => {
  test('several files can be chosen with the picker @responsive', async ({ page }) => {
    // Real files on disk, not Playwright's in-memory buffers: with in-memory files, WebKit
    // intermittently sent an empty file part (see e2e/README.md, "WebKit upload investigation").
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hamal-picker-'));
    try {
      const files = { 'alpha.txt': 'alpha', 'beta.txt': 'beta', 'gamma.txt': 'gamma' };
      for (const [name, content] of Object.entries(files)) fs.writeFileSync(path.join(dir, name), content);

      const room = await createRoom();
      await page.goto(`${room.base}/r/${room.participant_token}`);
      await page.locator('#file-input').setInputFiles(Object.keys(files).map((name) => path.join(dir, name)));
      await expect(page.locator('#file-list .file-row')).toHaveCount(3, { timeout: 12_000 });
      expect(await serverFiles(room)).toEqual(['alpha.txt', 'beta.txt', 'gamma.txt']);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });

  test('files dropped on the upload area are uploaded', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await dropFiles(page, '#dropzone', ['dropped-1.txt', 'dropped-2.txt']);
    await expect(page.locator('#file-list .file-row')).toHaveCount(2, { timeout: 12_000 });
    expect(await serverFiles(room)).toEqual(['dropped-1.txt', 'dropped-2.txt']);
  });

  test('an image pasted on the page becomes an upload', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await pasteImage(page, null);
    await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 12_000 });
    const names = await serverFiles(room);
    expect(names).toHaveLength(1);
    expect(names[0]).toMatch(/^pasted-image-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.png$/);
  });

  test('an image pasted into a text field is not uploaded (baseline behaviour)', async ({ page }) => {
    // Pins 6765e5e: site.js ignores paste events whose target is an INPUT or TEXTAREA.
    // The handoff's feature table says the opposite; see the Phase 0 report.
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#tab-text').click();
    await pasteImage(page, '#text-input');
    await page.waitForTimeout(1_500);
    expect(await serverFiles(room)).toEqual([]);
  });

  test('progress is shown while a file uploads', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    // Hold the upload so the progress panel can be observed.
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    await page.route('**/api/v1/rooms/*/files?filename=*', async (route) => {
      await held;
      await route.continue();
    });
    await page.locator('#file-input').setInputFiles({ name: 'slow.bin', mimeType: 'application/octet-stream', buffer: Buffer.alloc(64 * 1024) });
    await expect(page.locator('#upload-progress-container')).toBeVisible();
    await expect(page.locator('#upload-filename')).toHaveText('slow.bin');
    release();
    await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 12_000 });
    await expect(page.locator('#upload-progress-container')).toBeHidden();
  });
});
