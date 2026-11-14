import { test, expect } from './support/fixtures';
import { createRoom, getJSON, DIAG_URL } from './support/api';
import { dropFiles, pasteImage } from './support/page';
import { TransferDiagnostics, serverUploads } from './support/transfer-diagnostics';

async function serverFiles(room: { base: string; participant_token: string }): Promise<string[]> {
  const r = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
  return (r.json.files ?? []).map((f: any) => f.filename).sort();
}

test.describe('upload', () => {
  test('several files can be chosen with the picker @responsive', async ({ page }, testInfo) => {
    // Diagnostics for an intermittent WebKit failure (CI runs 43640379399, 43641965031,
    // 43641969624, 43643272986, 43643277150: one of the three uploads gets HTTP 400 "uploaded
    // file is empty"); printed only on failure. The room lives on the diagnostic server, which
    // runs the same handler and records what it receives for each upload.
    const diag = new TransferDiagnostics();
    await diag.watch(page, 'page', { 'file rows': '#file-list .file-row' }, { probeFiles: true });
    const room = await createRoom({ base: DIAG_URL });
    let failed = true;
    try {
      await page.goto(`${room.base}/r/${room.participant_token}`);
      await page.locator('#file-input').setInputFiles([
        { name: 'alpha.txt', mimeType: 'text/plain', buffer: Buffer.from('alpha') },
        { name: 'beta.txt', mimeType: 'text/plain', buffer: Buffer.from('beta') },
        { name: 'gamma.txt', mimeType: 'text/plain', buffer: Buffer.from('gamma') },
      ]);
      await expect(page.locator('#file-list .file-row')).toHaveCount(3, { timeout: 12_000 });
      expect(await serverFiles(room)).toEqual(['alpha.txt', 'beta.txt', 'gamma.txt']);
      failed = false;
    } finally {
      await diag.report(
        testInfo,
        failed,
        async () => `files on server: [${(await serverFiles(room)).join(', ')}]\n${serverUploads(room.participant_token)}`,
      );
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
