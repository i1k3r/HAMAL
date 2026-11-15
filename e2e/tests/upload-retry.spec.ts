import { test, expect } from './support/fixtures';
import { createRoom, getJSON } from './support/api';

const UPLOAD = '**/api/v1/rooms/*/files?filename=*';

test.describe('upload queue retries and failures', () => {
  test('a 429 is retried after Retry-After and the file then uploads', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    let calls = 0;
    await page.route(UPLOAD, async (route) => {
      calls++;
      if (calls <= 2) {
        await route.fulfill({
          status: 429,
          headers: { 'Retry-After': '1', 'Content-Type': 'application/json' },
          body: JSON.stringify({ error: 'too many requests' }),
        });
        return;
      }
      await route.continue();
    });
    await page.locator('#file-input').setInputFiles({ name: 'busy.txt', mimeType: 'text/plain', buffer: Buffer.from('busy') });
    await expect(page.locator('#tray-list .lt-up', { hasText: 'busy.txt' }).locator('.lt-up-status')).toContainText('server busy, retrying in 1s');
    await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 15_000 });
    expect(calls).toBe(3);
    await expect(page.locator('#tray-list .lt-up', { hasText: 'busy.txt' }).locator('.lt-up-status')).toHaveText('Sent');
    await expect(page.locator('#tray-summary')).toHaveText('All 1 sent');
  });

  test('after 5 retries a 429 becomes a visible failure', async ({ page }) => {
    test.setTimeout(60_000);
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    let calls = 0;
    await page.route(UPLOAD, (route) => {
      calls++;
      return route.fulfill({
        status: 429,
        headers: { 'Retry-After': '1', 'Content-Type': 'application/json' },
        body: JSON.stringify({ error: 'too many requests' }),
      });
    });
    await page.locator('#file-input').setInputFiles({ name: 'never.txt', mimeType: 'text/plain', buffer: Buffer.from('x') });
    await expect(page.locator('#tray-list .lt-up', { hasText: 'never.txt' }).locator('.lt-up-status')).toHaveText('Not sent: the server is busy (too many requests)', { timeout: 20_000 });
    expect(calls).toBe(6); // first attempt + MAX_UPLOAD_RETRIES (5)
    await expect(page.locator('#tray-summary')).toHaveText('0 of 1 sent · 1 not sent');
    // Only a failure that can still succeed offers Retry.
    await expect(page.locator('#tray-list .lt-up', { hasText: 'never.txt' }).getByRole('button', { name: 'Retry never.txt' })).toBeVisible();
  });

  test('a transport failure is shown and the queue continues', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.route(UPLOAD, (route) => {
      const name = new URL(route.request().url()).searchParams.get('filename');
      return name === 'broken.txt' ? route.abort('connectionreset') : route.continue();
    });
    await page.locator('#file-input').setInputFiles([
      { name: 'broken.txt', mimeType: 'text/plain', buffer: Buffer.from('b') },
      { name: 'fine.txt', mimeType: 'text/plain', buffer: Buffer.from('f') },
    ]);
    await expect(page.locator('#tray-list .lt-up', { hasText: 'broken.txt' }).locator('.lt-up-status')).toHaveText('Not sent: connection lost');
    await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 12_000 });
    const files = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
    expect(files.json.files.map((f: any) => f.filename)).toEqual(['fine.txt']);
  });

  for (const status of [401, 404, 409, 410]) {
    test(`HTTP ${status} on upload stops the rest of the queue`, async ({ page }) => {
      const room = await createRoom();
      await page.goto(`${room.base}/r/${room.participant_token}`);
      let calls = 0;
      await page.route(UPLOAD, (route) => {
        calls++;
        return route.fulfill({
          status,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ error: `refused with ${status}` }),
        });
      });
      await page.locator('#file-input').setInputFiles([
        { name: 'one.txt', mimeType: 'text/plain', buffer: Buffer.from('1') },
        { name: 'two.txt', mimeType: 'text/plain', buffer: Buffer.from('2') },
        { name: 'three.txt', mimeType: 'text/plain', buffer: Buffer.from('3') },
      ]);
      const one = page.locator('#tray-list .lt-up', { hasText: 'one.txt' }).locator('.lt-up-status');
      if (status === 401) {
        await expect(one).toHaveText('Not sent: this device is no longer signed in to the room. Reload to enter the PIN again.');
      } else {
        await expect(one).toHaveText(`The room ended before this finished (refused with ${status})`);
      }
      for (const name of ['two.txt', 'three.txt']) {
        await expect(page.locator('#tray-list .lt-up', { hasText: name }).locator('.lt-up-status')).toContainText(
          status === 401 ? 'Not sent: this device is no longer signed in' : 'The room ended before this finished',
        );
      }
      // A stop that cannot succeed by retrying never offers Retry.
      await expect(page.locator('#tray-list').getByRole('button', { name: /^Retry / })).toHaveCount(0);
      expect(calls).toBe(1);
    });
  }
});
