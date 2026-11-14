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
    await page.goto(share.share_url);
    await expect(page.locator('body')).toContainText('This share link has expired, been revoked, or is no longer accessible.');
  });
});
