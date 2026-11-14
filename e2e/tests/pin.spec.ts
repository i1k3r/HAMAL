import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('PIN join', () => {
  test('the right PIN opens the room @responsive', async ({ page }) => {
    const room = await createRoom({ pin: '2468' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#pin-form')).toBeVisible();
    await expect(page.locator('#participant-pin-input')).toBeFocused();
    await page.locator('#participant-pin-input').fill('2468');
    await page.locator('#unlock-btn').click();
    await expect(page.locator('#pin-barrier-card')).toBeHidden();
    await expect(page.locator('#room-active-card')).toBeVisible();
    await expect(page.locator('#dropzone')).toBeVisible();

    // The session survives a reload.
    await page.reload();
    await expect(page.locator('body')).toHaveAttribute('data-pin-authenticated', 'true');
    await expect(page.locator('#room-active-card')).toBeVisible();
  });

  test('a wrong PIN shows the remaining attempts', async ({ page }) => {
    const room = await createRoom({ pin: '2468' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#participant-pin-input').fill('1111');
    await page.locator('#unlock-btn').click();
    await expect(page.locator('#pin-error')).toBeVisible();
    await expect(page.locator('#pin-error-text')).toContainText(/attempts remaining/);
    await expect(page.locator('#participant-pin-input')).toHaveValue('');
    await expect(page.locator('#room-active-card')).toBeHidden();
  });

  test('a malformed PIN is rejected in the page', async ({ page }) => {
    const room = await createRoom({ pin: '2468' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#participant-pin-input').fill('12');
    await page.locator('#unlock-btn').click();
    await expect(page.locator('#pin-error-text')).toHaveText('PIN must be 4 to 8 digits.');
  });

  test('repeated wrong PINs lock the form with a countdown', async ({ page }) => {
    const room = await createRoom({ pin: '2468' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    const input = page.locator('#participant-pin-input');
    for (let i = 0; i < 12; i++) {
      if (await page.locator('#pin-cooldown').isVisible()) break;
      await input.fill('0000');
      const resp = page.waitForResponse((r) => r.url().endsWith('/auth/pin'));
      await page.locator('#unlock-btn').click();
      await resp;
      // site.js updates the form after parsing the response; wait until it shows either the
      // wrong-PIN message or the cooldown before the next attempt.
      await expect(page.locator('#pin-cooldown:visible, #pin-error:visible').first()).toBeVisible();
    }
    await expect(page.locator('#pin-cooldown')).toBeVisible();
    await expect(page.locator('#pin-cooldown-text')).toContainText(/Too many failed attempts. Cooldown active \(\d+s\)/);
    await expect(input).toBeDisabled();
    await expect(page.locator('#unlock-btn')).toBeDisabled();
  });
});
