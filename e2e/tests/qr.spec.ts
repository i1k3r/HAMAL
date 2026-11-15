import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('QR invitation', () => {
  test('creator projects a QR code and the participant link @responsive', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#show-qr-btn:visible, #room-menu-btn:visible').first().click();
    if (await page.locator('#room-menu').isVisible()) await page.locator('#room-menu [data-menu="qr"]').click();

    const dialog = page.locator('#qr-dialog');
    await expect(dialog).toBeVisible();
    const qr = page.locator('#qr-image');
    await expect(qr).toBeVisible();
    await expect(qr).toHaveAttribute('src', `/api/v1/rooms/${room.participant_token}/qr.svg`);
    await expect.poll(() => qr.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0)).toBe(true);
    // Large enough to scan across a room (the design asks for at least 600 px on a 1440×900 screen).
    const box = await qr.boundingBox();
    const vp = page.viewportSize()!;
    if (vp.width >= 1280 && vp.height >= 900) expect(box!.width).toBeGreaterThanOrEqual(600);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(vp.width);

    const svg = await page.request.get(`${room.base}/api/v1/rooms/${room.participant_token}/qr.svg`);
    expect(svg.status()).toBe(200);
    expect(svg.headers()['content-type']).toContain('image/svg+xml');

    await expect(page.locator('#participant-link-input')).toHaveValue(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#qr-address')).toHaveText(`${room.base}/r/${room.participant_token}`);
  });

  test('copy address writes the participant URL to the clipboard', async ({ page, context, browserName }) => {
    test.skip(browserName !== 'chromium', 'clipboard permissions are Chromium-only in Playwright');
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#show-qr-btn').click();
    await page.locator('#copy-link-btn').click();
    await expect(page.locator('#copy-toast')).toHaveClass(/show/);
    await expect(page.locator('#copy-toast')).toHaveText('Address copied');
    const clip = await page.evaluate(() => navigator.clipboard.readText());
    expect(clip).toBe(`${room.base}/r/${room.participant_token}`);
  });

  test('Done closes the projection and returns focus to Show QR', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#show-qr-btn').click();
    await expect(page.locator('#qr-dialog')).toBeVisible();
    await expect(page.locator('#qr-done-btn')).toBeFocused();
    // The table is hidden from assistive technology while the code is projected.
    expect(await page.locator('#room-active-card').evaluate((n) => (n as HTMLElement).inert || n.getAttribute('aria-hidden') === 'true')).toBe(true);
    await page.locator('#qr-done-btn').click();
    await expect(page.locator('#qr-dialog')).toBeHidden();
    await expect(page.locator('#show-qr-btn')).toBeFocused();
  });

  test('Q opens the projection; it takes focus and closes with Escape', async ({ page }) => {
    // Formerly KF-005 (the old lightbox neither took focus nor closed with Escape).
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('body').click({ position: { x: 5, y: 5 } });
    await page.keyboard.press('q');
    const dialog = page.locator('#qr-dialog');
    await expect(dialog).toBeVisible();
    expect(await dialog.evaluate((d) => d.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
  });
});
