import * as fs from 'fs';
import { test, expect } from './support/fixtures';
import { createRoom, mustUpload, LIMITED_URL } from './support/api';
import { waitForPolls } from './support/page';

// Runs against the "limited" server, which has LAN_DROP_GLOBAL_SHARE_ENABLED=true.

test.describe('share links', () => {
  test('the creator creates a share link and its page downloads the file', async ({ page, browser }) => {
    const room = await createRoom({ base: LIMITED_URL });
    await mustUpload(room, 'handout.txt', 'shared content');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    // Rows rendered by the Go template have no click handlers until the first poll re-renders
    // them (baseline behaviour, reported in Phase 0), so wait for that poll.
    await waitForPolls(page, 1);
    const row = page.locator('#file-list .file-row', { hasText: 'handout.txt' });
    await row.getByRole('button', { name: 'Share Link' }).click();
    await expect(page.locator('#share-modal')).toBeVisible();
    await expect(page.locator('#modal-file-subtitle')).toContainText('handout.txt');
    await page.locator('#generate-share-btn').click();
    await expect(page.locator('#share-result-box')).toBeVisible();
    const shareUrl = await page.locator('#generated-share-input').inputValue();
    expect(shareUrl).toMatch(new RegExp(`^${LIMITED_URL}/s/[A-Za-z0-9_-]+$`));

    const visitor = await browser.newPage();
    const res = await visitor.goto(shareUrl);
    expect(res?.status()).toBe(200);
    await expect(visitor.locator('body')).toHaveAttribute('data-page', 'share');
    await expect(visitor.locator('body')).toContainText('handout.txt');
    const file = await visitor.request.get(`${shareUrl}/download`);
    expect(file.status()).toBe(200);
    expect(await file.text()).toBe('shared content');
    await visitor.close();
  });

  test('participants do not get share buttons', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    await mustUpload(room, 'private.txt', 'x');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(1);
    await expect(page.locator('.btn-share-link')).toHaveCount(0);
  });

  test('a revoked share link stops working', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'revoke-me.txt', 'x');
    const fileId = file.file_id ?? file.file?.file_id;
    const created = await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${fileId}/share`, {
      data: { ttl_seconds: 3600 },
    });
    expect(created.status()).toBe(201);
    const share = await created.json();
    const revoke = await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/shares/${share.share_id}/revoke`);
    expect(revoke.ok()).toBe(true);
    const dl = await page.request.get(`${share.share_url}/download`);
    expect(dl.ok()).toBe(false);
    // A revoked share answers 410 until the cleanup job purges it; after that it is unknown (404).
    // Either way the page explains it and offers no download.
    const res = await page.goto(share.share_url);
    expect([404, 410]).toContain(res?.status());
    if (res?.status() === 410) {
      await expect(page.locator('h1')).toHaveText('This link no longer works');
      await expect(page.locator('body')).toContainText('It expired, was revoked, or the room it came from was closed.');
    } else {
      await expect(page.locator('h1')).toHaveText("This link isn't available");
      await expect(page.locator('body')).toContainText("It doesn't exist or was removed.");
    }
    await expect(page.locator('#share-download-btn')).toHaveCount(0);
  });

  test('the share page is Light Table: file, kind and size, one primary Download, live expiry @responsive', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const longName = 'Quarterly_infrastructure_review_and_migration_plan_for_the_storage_cluster_final_v7.pdf';
    const file = await mustUpload(room, longName, 'fourteen bytes', { type: 'application/pdf' });
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    await page.goto(share.share_url);
    await expect(page.locator('body')).toHaveClass(/\blt\b/);
    await expect(page.locator('h1#share-title')).toHaveText(longName);
    await expect(page.locator('#share-ext')).toHaveText('PDF');
    await expect(page.locator('#share-meta')).toHaveText('Document, 14 B');
    await expect(page.locator('.lt-btn-primary')).toHaveCount(1);
    await expect(page.locator('#share-download-btn')).toBeVisible();
    await expect(page.locator('#countdown')).toHaveText(/^(59|60) minutes$/);
    await expect(page.locator('#share-expiry')).not.toContainText('Calculating');
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
    const box = (await page.locator('#share-download-btn').boundingBox())!;
    expect(box.height).toBeGreaterThanOrEqual(44);
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  });

  test('right after the page loads, Download costs one request (no extra check)', async ({ page }) => {
    // The page load has just validated the link, so the download goes straight to the browser:
    // a visit costs two rate-limited requests (page + file), as before the redesign.
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'handout.txt', 'shared content');
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    await page.goto(share.share_url);
    const methods: string[] = [];
    page.on('request', (r) => { if (r.url().endsWith('/download')) methods.push(r.method()); });
    const [download] = await Promise.all([page.waitForEvent('download'), page.locator('#share-download-btn').click()]);
    expect(download.suggestedFilename()).toBe('handout.txt');
    expect(fs.readFileSync(await download.path(), 'utf8')).toBe('shared content');
    // (Chromium doesn't report the download navigation itself as a page request; the check is a fetch and would be.)
    expect(methods).not.toContain('HEAD');
    await expect(page.locator('#share-download-error')).toBeHidden();
  });

  test('when the page has been open a while, Download checks the link first, then downloads', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'handout.txt', 'shared content');
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    await page.clock.install();
    await page.goto(share.share_url);
    await page.clock.fastForward(31_000);
    const methods: string[] = [];
    page.on('request', (r) => { if (r.url().endsWith('/download')) methods.push(r.method()); });
    const [download] = await Promise.all([page.waitForEvent('download'), page.locator('#share-download-btn').click()]);
    expect(download.suggestedFilename()).toBe('handout.txt');
    expect(methods[0]).toBe('HEAD');
  });

  test('a file that is gone, or a link that died, says so on the page instead of showing an error document', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'gone.txt', 'x');
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    // The check runs once the page is no longer fresh (open for more than 30 s).
    await page.clock.install();
    await page.goto(share.share_url);
    await page.clock.fastForward(31_000);
    await page.route('**/download', (r) => r.fulfill({ status: 404, contentType: 'application/json', body: '{"error":"file not found"}' }));
    await page.locator('#share-download-btn').click();
    await expect(page.locator('#share-download-error')).toHaveText(/This file is no longer available/);
    await expect(page.locator('#share-download-error')).toHaveAttribute('role', 'alert');
    expect(new URL(page.url()).pathname).toBe(new URL(share.share_url).pathname);
    await page.unroute('**/download');
    await page.route('**/download', (r) => r.fulfill({ status: 410, contentType: 'application/json', body: '{"error":"share has expired or is inactive"}' }));
    await page.locator('#share-download-btn').click();
    await expect(page.locator('#share-expired-card')).toBeVisible();
    await expect(page.locator('#share-card')).toBeHidden();
    await expect(page.locator('#share-expired-card h1')).toBeFocused();
  });

  test('the page notices when the link expires while it is open', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'soon.txt', 'x');
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    await page.clock.install();
    await page.goto(share.share_url);
    await expect(page.locator('#share-card')).toBeVisible();
    await page.clock.runFor(3601 * 1000);
    await expect(page.locator('#share-expired-card')).toBeVisible();
    await expect(page.locator('#share-expired-card')).toContainText('It expired while this page was open.');
  });

  test('an unknown link answers 404 with an explanation, not a bare error', async ({ page }) => {
    const res = await page.goto(`${LIMITED_URL}/s/gsh_${'0'.repeat(64)}`);
    expect(res?.status()).toBe(404);
    await expect(page.locator('body')).toHaveClass(/\blt\b/);
    await expect(page.locator('h1')).toHaveText("This link isn't available");
    await expect(page.getByRole('link', { name: 'Open a room of your own' })).toHaveAttribute('href', '/');
    // The token from the address is not echoed back into the page.
    await expect(page.locator('body')).toHaveAttribute('data-token', '');
  });

  test('Download is reachable by keyboard with a visible focus ring', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'keys.txt', 'x');
    const share = await (await page.request.post(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, { data: { ttl_seconds: 3600 } })).json();
    await page.goto(share.share_url);
    for (let i = 0; i < 6 && !(await page.locator('#share-download-btn').evaluate((n) => n === document.activeElement)); i++) await page.keyboard.press('Tab');
    await expect(page.locator('#share-download-btn')).toBeFocused();
    expect(await page.locator('#share-download-btn').evaluate((n) => getComputedStyle(n).outlineStyle)).toBe('solid');
  });
});
