import { type Page } from '@playwright/test';
import { test, expect } from '../support/fixtures';
import fs from 'node:fs';
import path from 'node:path';
import { createRoom, mustUpload, LIMITED_URL, PRIMARY_URL } from '../support/api';

/*
 * Visual baselines (test-only). These run only in the pinned Playwright container
 * (HAMAL_VISUAL=1, see playwright.config.ts) so pixels are reproducible.
 *
 * Determinism:
 *  - Page HTML comes from the real server, but every room/files/texts poll is answered from the
 *    fixtures in e2e/fixtures/visual/, so names, sizes, times, counts and participants are fixed.
 *  - Elements that depend on the random room token or the wall clock are masked: the QR image,
 *    the participant address field, the countdowns and the share page's expiry.
 *  - locale en-US, timezone UTC, reduced motion, device scale factor 1 (project settings);
 *    animations disabled and caret hidden (expect.toHaveScreenshot defaults in the config).
 *  - Fonts are self-hosted; every capture waits for document.fonts.ready.
 */

const fx = (name: string) => fs.readFileSync(path.join(__dirname, '..', '..', 'fixtures', 'visual', name), 'utf8');
const ROOM = fx('room.json');
const FILES = fx('files.json');
const TEXTS = fx('texts.json');
const QR = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 41 41" shape-rendering="crispEdges"><rect width="41" height="41" fill="#fff"/><rect x="4" y="4" width="7" height="7"/><rect x="30" y="4" width="7" height="7"/><rect x="4" y="30" width="7" height="7"/></svg>';

async function mockRoomApi(page: Page, opts: { creator: boolean }) {
  await page.route(/\/api\/v1\/rooms\/[^/]+(\/files|\/texts|\/qr\.svg)?$/, async (route) => {
    if (route.request().method() !== 'GET') return route.continue();
    const p = new URL(route.request().url()).pathname;
    if (p.endsWith('/files')) return route.fulfill({ contentType: 'application/json', body: FILES });
    if (p.endsWith('/texts')) return route.fulfill({ contentType: 'application/json', body: TEXTS });
    if (p.endsWith('/qr.svg')) return route.fulfill({ contentType: 'image/svg+xml', body: QR });
    const room = JSON.parse(ROOM);
    room.is_creator = opts.creator;
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(room) });
  });
}

async function settle(page: Page) {
  // First fixture poll fires 2 s after load; wait for its last request (texts) to render.
  await page.waitForResponse((r) => /\/texts$/.test(new URL(r.url()).pathname), { timeout: 15_000 });
  await page.waitForTimeout(300);
  await page.evaluate(() => document.fonts.ready);
  // Give countdown a deterministic, fixed-size bounding box independent of timer state
  await page.evaluate(() => {
    const el = document.getElementById('countdown');
    if (el) {
      el.style.display = 'inline-block';
      el.style.boxSizing = 'border-box';
      el.style.width = '6ch';
      el.style.textAlign = 'center';
      el.style.overflow = 'hidden';
    }
  });
}

const masks = (page: Page) => [
  page.locator('#qr-image'),
  page.locator('#participant-link-input'),
  page.locator('#countdown'),
  page.locator('#closing-countdown'),
];

test.describe('visual baselines', () => {
  test('home', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.evaluate(() => document.fonts.ready);
    await expect(page).toHaveScreenshot('home.png', { fullPage: true });
  });

  test('home, invalid PIN', async ({ page }) => {
    await page.goto(`${PRIMARY_URL}/`);
    await page.evaluate(() => document.fonts.ready);
    await page.locator('#pin-input').fill('12');
    await page.locator('#create-submit-btn').click();
    await expect(page.locator('#form-error')).toBeVisible();
    await page.locator('#pin-input').blur();
    await expect(page).toHaveScreenshot('home-invalid-pin.png', { fullPage: true });
  });

  test('creator, files tab', async ({ page }) => {
    const room = await createRoom();
    await mockRoomApi(page, { creator: true });
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await settle(page);
    await expect(page).toHaveScreenshot('creator-files.png', { fullPage: true, mask: masks(page) });
  });

  test('creator, writing a note', async ({ page }) => {
    // The Light Table pages have no separate Text tab: notes lie on the table with the files, and
    // "Write a note" opens the composer (inline on desktop, a bottom sheet on phones).
    const room = await createRoom();
    await mockRoomApi(page, { creator: true });
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await settle(page);
    await page.locator('#write-note-btn').click();
    await expect(page.locator('#composer')).toBeVisible();
    await page.locator('#text-input').blur();
    await expect(page).toHaveScreenshot('creator-note.png', { fullPage: true, mask: masks(page) });
  });

  test('participant, files tab', async ({ page }) => {
    const room = await createRoom();
    await mockRoomApi(page, { creator: false });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await settle(page);
    await expect(page).toHaveScreenshot('participant-files.png', { fullPage: true, mask: masks(page) });
  });

  test('participant, PIN screen', async ({ page }) => {
    const room = await createRoom({ pin: '2468' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.evaluate(() => document.fonts.ready);
    await page.locator('#participant-pin-input').blur();
    await expect(page).toHaveScreenshot('participant-pin.png', { fullPage: true, mask: masks(page) });
  });

  test('share page', async ({ page }) => {
    const room = await createRoom({ base: LIMITED_URL });
    const file = await mustUpload(room, 'handout.pdf', 'x'.repeat(2048));
    const res = await fetch(`${room.base}/api/v1/rooms/${room.creator_token}/files/${file.file_id}/share`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ttl_seconds: 3600 }),
    });
    const share = await res.json();
    await page.goto(share.share_url);
    await page.evaluate(() => document.fonts.ready);
    await expect(page).toHaveScreenshot('share.png', {
      fullPage: true,
      mask: [page.locator('#countdown'), page.locator('a[href*="/s/"]')],
    });
  });
});
