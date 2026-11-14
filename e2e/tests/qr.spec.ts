import { test, expect, knownFailure } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('QR invitation', () => {
  test('creator sees a QR code and the participant link @responsive', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);

    const qr = page.locator('#qr-image');
    await expect(qr).toBeVisible();
    await expect(qr).toHaveAttribute('src', `/api/v1/rooms/${room.participant_token}/qr.svg`);
    const loaded = await qr.evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0);
    expect(loaded, 'the QR image decodes').toBe(true);

    const svg = await page.request.get(`${room.base}/api/v1/rooms/${room.participant_token}/qr.svg`);
    expect(svg.status()).toBe(200);
    expect(svg.headers()['content-type']).toContain('image/svg+xml');

    await expect(page.locator('#participant-link-input')).toHaveValue(
      `${room.base}/r/${room.participant_token}`,
    );
  });

  test('copy link writes the participant URL to the clipboard', async ({ page, context, browserName }) => {
    test.skip(browserName !== 'chromium', 'clipboard permissions are Chromium-only in Playwright');
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#copy-link-btn').click();
    await expect(page.locator('#copy-toast')).toHaveClass(/show/);
    const clip = await page.evaluate(() => navigator.clipboard.readText());
    expect(clip).toBe(`${room.base}/r/${room.participant_token}`);
  });

  test('the QR lightbox opens from the QR code and closes with its close button', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#qr-box').click();
    const dialog = page.locator('#modal-qr');
    await expect(dialog).toBeVisible();
    await expect(page.locator('#qr-lightbox-box img')).toHaveAttribute(
      'src',
      `${room.base}/api/v1/rooms/${room.participant_token}/qr.svg`,
    );
    await page.locator('#modal-qr-close').click();
    await expect(dialog).toBeHidden();
  });

  test('the QR lightbox takes focus and closes with Escape', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#qr-box').click();
    const dialog = page.locator('#modal-qr');
    await expect(dialog).toBeVisible();

    await knownFailure('KF-005', async () => {
      const focusInside = await dialog.evaluate((d) => d.contains(document.activeElement));
      await page.keyboard.press('Escape');
      await page.waitForTimeout(300);
      const closedByEscape = await dialog.isHidden();
      expect(
        { focusInside, closedByEscape },
        'KF-005: the QR lightbox takes focus when it opens and closes with Escape',
      ).toEqual({ focusInside: true, closedByEscape: true });
    });
  });
});
