import { test, expect } from './support/fixtures';
import { createRoom, getJSON, DIAG_URL } from './support/api';
import { dropFiles, pasteImage } from './support/page';
import { TransferDiagnostics, serverUploads } from './support/transfer-diagnostics';

async function serverFiles(room: { base: string; participant_token: string }): Promise<string[]> {
  const r = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
  return (r.json.files ?? []).map((f: any) => f.filename).sort();
}

/*
 * Upload instrumentation experiment for the intermittent WebKit "uploaded file is empty" failure
 * of the multi-file picker test. Two independent switches, read from the environment:
 *   HAMAL_UPLOAD_PROBE=on|off        in-page File probe (reads each File with arrayBuffer()).
 *                                    "off" installs nothing in the page.
 *   HAMAL_UPLOAD_DIAG_SERVER=on|off  "on" creates the room on the diagnostic server
 *                                    (e2e/diagserver, 127.0.0.1:7801), which records what it
 *                                    receives; "off" uses the normal primary server (7799).
 * Variants: A = off/off (the original scenario, and the default), B = probe off / server on,
 * C = probe on / server off, D = on/on. Results from different variants must not be combined.
 */
function onOff(name: string): boolean {
  const v = process.env[name] ?? 'off';
  if (v !== 'on' && v !== 'off') throw new Error(`${name} must be "on" or "off", got "${v}"`);
  return v === 'on';
}
const UPLOAD_PROBE = onOff('HAMAL_UPLOAD_PROBE');
const UPLOAD_DIAG_SERVER = onOff('HAMAL_UPLOAD_DIAG_SERVER');
const UPLOAD_VARIANT = ({ 'off/off': 'A', 'off/on': 'B', 'on/off': 'C', 'on/on': 'D' } as const)[
  `${UPLOAD_PROBE ? 'on' : 'off'}/${UPLOAD_DIAG_SERVER ? 'on' : 'off'}` as 'off/off'
];
const UPLOAD_VARIANT_LABEL = `upload-variant=${UPLOAD_VARIANT} probe=${UPLOAD_PROBE ? 'on' : 'off'} diag-server=${UPLOAD_DIAG_SERVER ? 'on' : 'off'}`;

test.describe('upload', () => {
  test('several files can be chosen with the picker @responsive', async ({ page }, testInfo) => {
    // Diagnostics for an intermittent WebKit failure (CI runs 43640379399, 43641965031,
    // 43641969624, 43643272986, 43643277150: one of the three uploads gets HTTP 400 "uploaded
    // file is empty"); printed only on failure. Instrumentation is chosen by the experiment
    // switches above.
    testInfo.annotations.push({ type: 'upload-variant', description: UPLOAD_VARIANT_LABEL });
    const diag = new TransferDiagnostics(UPLOAD_VARIANT_LABEL);
    await diag.watch(page, 'page', { 'file rows': '#file-list .file-row' }, { probeFiles: UPLOAD_PROBE });
    const room = UPLOAD_DIAG_SERVER ? await createRoom({ base: DIAG_URL }) : await createRoom();
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
        async () =>
          `files on server: [${(await serverFiles(room)).join(', ')}]\n` +
          (UPLOAD_DIAG_SERVER ? serverUploads(room.participant_token) : 'diagnostic server not used in this variant'),
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
