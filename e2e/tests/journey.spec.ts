import { test, expect } from './support/fixtures';
import { PRIMARY_URL } from './support/api';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

/*
 * The whole journey in one test: open HAMAL, open a room, share a file and a note, join from a
 * second device with the PIN, and see the same table on both. Every page along the way uses the
 * Light Table design (body.lt, the same fonts and frame).
 */

const isLightTable = async (page: import('@playwright/test').Page) => {
  await expect(page.locator('body')).toHaveClass(/\blt\b/);
  expect(await page.evaluate(() => getComputedStyle(document.body).backgroundColor)).toBe('rgb(26, 13, 11)');
};

test('open HAMAL → open a room → share → join as a participant @responsive', async ({ page, browser }) => {
  test.setTimeout(90_000);
  // 1. Open HAMAL
  await page.goto(`${PRIMARY_URL}/`);
  await isLightTable(page);

  // 2. Open a room with a PIN
  await page.locator('#ttl-select').selectOption('1800');
  await page.locator('#pin-input').fill('2468');
  await page.locator('#create-submit-btn').click();
  await page.waitForURL(/\/c\/c_/);

  // 3. The creator lands on the empty table
  await isLightTable(page);
  await expect(page.locator('#table-empty')).toBeVisible();
  await expect(page.locator('#countdown')).toHaveText(/minutes/);

  // 4. Share a file and a note
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hamal-journey-'));
  const file = path.join(dir, 'agenda.txt');
  fs.writeFileSync(file, 'meeting at ten');
  await page.locator('#file-input').setInputFiles(file);
  await expect(page.locator('#file-list .file-row', { hasText: 'agenda.txt' })).toBeVisible({ timeout: 12_000 });
  await page.locator('#write-note-btn').click();
  await page.locator('#text-input').fill('wifi password is on the fridge');
  await page.locator('#send-text-btn').click();
  await expect(page.locator('#file-list .text-item-card', { hasText: 'wifi password' })).toBeVisible({ timeout: 12_000 });

  // 5. Invite: the QR projection carries the participant address
  if (await page.locator('#show-qr-btn').isVisible()) await page.locator('#show-qr-btn').click();
  else { await page.locator('#room-menu-btn').click(); await page.locator('#room-menu [data-menu="qr"]').click(); }
  const address = await page.locator('#participant-link-input').inputValue();
  expect(address).toMatch(/\/r\/p_/);
  await page.keyboard.press('Escape');

  // 6. A second device joins with the PIN
  const ctx = await browser.newContext();
  const phone = await ctx.newPage();
  await phone.goto(address.replace(/^https?:\/\/[^/]+/, PRIMARY_URL));
  await isLightTable(phone);
  await expect(phone.locator('#pin-barrier-card')).toBeVisible();
  await phone.locator('#participant-pin-input').fill('2468');
  await phone.locator('#unlock-btn').click();
  await expect(phone.locator('#room-active-card')).toBeVisible();

  // 7. Both devices see the same table; the phone shares back
  await expect(phone.locator('#file-list .file-row', { hasText: 'agenda.txt' })).toBeVisible({ timeout: 12_000 });
  await expect(phone.locator('#file-list .text-item-card', { hasText: 'wifi password' })).toBeVisible();
  await phone.locator('#write-note-btn').click();
  await phone.locator('#text-input').fill('got it, thanks');
  await phone.locator('#send-text-btn').click();
  await expect(page.locator('#file-list .text-item-card', { hasText: 'got it, thanks' })).toBeVisible({ timeout: 12_000 });
  await expect(page.locator('#participant-list .lt-seat')).not.toHaveCount(0, { timeout: 12_000 });
  await ctx.close();
});
