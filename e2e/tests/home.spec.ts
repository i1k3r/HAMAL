import { test, expect } from './support/fixtures';
import { PRIMARY_URL } from './support/api';

/*
 * The landing screen (open a room). Room creation behaviour is pinned by create-room.spec.ts; this
 * spec covers the screen's states and layout in the Light Table design.
 */

test.describe('home: open a room', () => {
  test('a clean session shows the room form on the lit card, in the Light Table design @responsive', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await expect(page.locator('body')).toHaveClass(/\blt\b/);
    await expect(page.getByRole('heading', { level: 1 })).toContainText('Open a room.');
    await expect(page.locator('#ttl-select')).toHaveValue('3600');
    await expect(page.locator('#pin-input')).toHaveValue('');
    await expect(page.locator('#form-error')).toBeHidden();
    const cta = page.locator('#create-submit-btn');
    await expect(cta).toHaveText('Open the room');
    // The primary action is in the first screen on every device, and nothing scrolls sideways.
    const box = (await cta.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize()!.height);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
    // The same fonts as the room pages, and no leftovers of the old dashboard.
    expect(await page.locator('h1').evaluate((n) => getComputedStyle(n).fontFamily)).toContain('Schibsted Grotesk');
    await expect(page.locator('.sidebar, #nav-btn-files, #nav-btn-room')).toHaveCount(0);
  });

  test('the PIN can be shown and hidden, and only digits are kept', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    const pin = page.locator('#pin-input');
    await pin.pressSequentially('12a34');
    await expect(pin).toHaveValue('1234');
    await expect(pin).toHaveAttribute('type', 'password');
    await page.locator('#pin-toggle-btn').click();
    await expect(pin).toHaveAttribute('type', 'text');
    await page.locator('#pin-toggle-btn').click();
    await expect(pin).toHaveAttribute('type', 'password');
  });

  test('while the room is being opened the button is busy, and a server error is shown on the card', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    await page.route('**/api/v1/rooms', async (route) => {
      await held;
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'database is unavailable' }) });
    });
    const cta = page.locator('#create-submit-btn');
    await cta.click();
    await expect(cta).toBeDisabled();
    await expect(cta).toHaveAttribute('aria-busy', 'true');
    await expect(cta).toHaveText('Opening the room…');
    release();
    const err = page.locator('#form-error');
    await expect(err).toBeVisible();
    await expect(err).toHaveAttribute('role', 'alert');
    await expect(err).toHaveText('database is unavailable');
    await expect(cta).toBeEnabled();
    await expect(cta).not.toHaveAttribute('aria-busy', /.*/);
    await expect(cta).toHaveText('Open the room');
    expect(new URL(page.url()).pathname).toBe('/');
  });

  test('a lost connection says so in plain words', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.route('**/api/v1/rooms', (route) => route.abort('connectionreset'));
    await page.locator('#create-submit-btn').click();
    await expect(page.locator('#form-error')).toHaveText("Couldn't reach HAMAL. Check your connection and try again.");
    await expect(page.locator('#create-submit-btn')).toHaveText('Open the room');
  });

  test('fixing an invalid PIN and trying again opens the room', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.locator('#pin-input').fill('12');
    await page.locator('#create-submit-btn').click();
    await expect(page.locator('#form-error')).toHaveText('PIN must be 4 to 8 digits');
    await page.locator('#pin-input').fill('2468');
    await page.locator('#create-submit-btn').click();
    await page.waitForURL(/\/c\/c_/);
    await expect(page.locator('body')).toHaveAttribute('data-pin-required', 'true');
  });

  test('every entry screen has exactly one primary action, in the same safelight colour', async ({ page }) => {
    const { createRoom } = await import('./support/api');
    const room = await createRoom({ pin: '2468' });
    const SAFELIGHT = 'rgb(255, 106, 85)';
    const screens: [string, string][] = [
      [`${PRIMARY_URL}/`, '#create-submit-btn'],
      [`${room.base}/r/${room.participant_token}`, '#unlock-btn'],
    ];
    for (const [url, id] of screens) {
      await page.goto(url);
      const primaries = page.locator('.lt-btn-primary:visible');
      await expect(primaries, url).toHaveCount(1);
      await expect(primaries.first()).toHaveAttribute('id', id.slice(1));
      expect(await page.locator(id).evaluate((n) => getComputedStyle(n).backgroundColor)).toBe(SAFELIGHT);
    }
  });
});
