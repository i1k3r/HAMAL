import { test, expect } from './support/fixtures';
import { createRoom, mustUpload, mustPostText, PNG_1PX, getJSON } from './support/api';
import { waitForPolls } from './support/page';

/*
 * Light Table room pages: behaviour that belongs to the design itself (views, toolbar, viewer,
 * long notes, expiry, layout limits). Paste, preview dismissal, upload states and the PIN screen
 * have their own specs.
 */

test.describe('Light Table', () => {
  test('files and notes lie on one table as prints, slips and index cards, newest first', async ({ page }) => {
    const room = await createRoom();
    // API timestamps have one-second resolution, so space the items out.
    await mustUpload(room, 'report.pdf', 'pdf', { type: 'application/pdf' });
    await new Promise((r) => setTimeout(r, 1100));
    await mustPostText(room, 'docker compose up -d');
    await new Promise((r) => setTimeout(r, 1100));
    await mustUpload(room, 'photo.png', PNG_1PX, { type: 'image/png' });
    await page.goto(`${room.base}/c/${room.creator_token}`);
    const items = page.locator('#file-list > .lt-item');
    await expect(items).toHaveCount(3);
    await expect(items.nth(0)).toContainText('photo.png');
    await expect(items.nth(0).locator('.lt-print img')).toHaveAttribute('alt', 'photo.png');
    await expect(items.nth(1).locator('.lt-card')).toContainText('docker compose up -d');
    await expect(items.nth(2).locator('.lt-slip .lt-slip-ext')).toHaveText('PDF');
    // The room's own messages (room created, file uploaded) go to the room log, not the table.
    await expect(page.locator('#activity-list')).toContainText('report.pdf laid on the table');
    await expect(page.locator('#count-all')).toHaveText('3');
  });

  test('Table and List show the same items; the choice survives a reload', async ({ page }) => {
    const room = await createRoom();
    for (let i = 0; i < 3; i++) await mustUpload(room, `doc-${i}.txt`, `d${i}`);
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('#view-table')).toHaveAttribute('aria-pressed', 'true');
    await page.locator('#view-list').click();
    await expect(page.locator('#file-list')).toHaveClass(/is-list/);
    await expect(page.locator('#file-list .file-row')).toHaveCount(3);
    await expect(page.locator('#list-bar')).toBeVisible();
    await page.reload();
    await expect(page.locator('#view-list')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#file-list .lt-row')).toHaveCount(3);
  });

  test('more than 24 items switches to List automatically and says so', async ({ page }) => {
    test.setTimeout(60_000);
    const room = await createRoom();
    for (let i = 0; i < 24; i++) await mustUpload(room, `f${String(i).padStart(2, '0')}.txt`, 'x');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('#view-table')).toHaveAttribute('aria-pressed', 'true');
    await mustUpload(room, 'f24.txt', 'x');
    await expect(page.locator('#view-list')).toHaveAttribute('aria-pressed', 'true', { timeout: 12_000 });
    await expect(page.locator('#live-polite')).toHaveText(/more than 24 items/);
  });

  test('search and the kind filter narrow the table', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'Invoice_November.pdf', 'p');
    await mustUpload(room, 'holiday.png', PNG_1PX, { type: 'image/png' });
    await mustPostText(room, 'invoice number 42');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('#file-list > .lt-item')).toHaveCount(3);
    await page.locator('#search-input').fill('invoice');
    await expect(page.locator('#file-list > .lt-item')).toHaveCount(2);
    await page.locator('#search-input').fill('');
    await page.locator('#kind-chips [data-kind="image"]').click();
    await expect(page.locator('#file-list > .lt-item')).toHaveCount(1);
    await expect(page.locator('#file-list > .lt-item')).toContainText('holiday.png');
    await page.locator('#kind-chips [data-kind="note"]').click();
    await expect(page.locator('#file-list > .lt-item')).toHaveCount(1);
    await expect(page.locator('#file-list .text-item-card')).toContainText('invoice number 42');
  });

  test('the viewer steps through the viewable items with the arrow keys and closes back on the item', async ({ page }) => {
    const room = await createRoom();
    for (const n of ['a.png', 'b.png', 'c.png']) {
      await mustUpload(room, n, PNG_1PX, { type: 'image/png' });
      await new Promise((r) => setTimeout(r, 1100));
    }
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.getByRole('button', { name: 'View c.png' }).click();
    await expect(page.locator('#viewer-count')).toHaveText('1 of 3');
    await expect(page.locator('#viewer-prev')).toBeDisabled();
    await page.keyboard.press('ArrowRight');
    await expect(page.locator('#preview-filename')).toHaveText('b.png');
    await expect(page.locator('#viewer-count')).toHaveText('2 of 3');
    await page.locator('#viewer-next').click();
    await expect(page.locator('#preview-filename')).toHaveText('a.png');
    await expect(page.locator('#viewer-next')).toBeDisabled();
    await page.keyboard.press('Escape');
    await expect(page.locator('#file-preview-overlay')).toBeHidden();
    await expect(page.getByRole('button', { name: 'View a.png' })).toBeFocused();
  });

  test('a note over the limit can be sent as a .txt file with identical bytes', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#write-note-btn').click();
    const text = 'çğış '.repeat(14_000); // about 98 KB of UTF-8
    await page.locator('#text-input').fill(text);
    await expect(page.locator('#text-overlimit')).toBeVisible();
    await expect(page.locator('#send-text-btn')).toBeDisabled();
    await page.locator('#send-as-file-btn').click();
    await expect(page.locator('#file-list .file-row')).toHaveCount(1, { timeout: 12_000 });
    const files = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/files`);
    const f = files.json.files[0];
    expect(f.filename).toMatch(/^note-\d{2}-\d{2}\.txt$/);
    expect(f.size_bytes).toBe(Buffer.byteLength(text, 'utf8'));
    const res = await fetch(`${room.base}/api/v1/rooms/${room.participant_token}/files/${f.file_id}`);
    expect(Buffer.from(await res.arrayBuffer()).toString('utf8')).toBe(text);
  });

  test('a note that is a single link renders as a safe link; other text stays text', async ({ page }) => {
    const room = await createRoom();
    await mustPostText(room, 'https://example.com/path');
    await mustPostText(room, 'javascript:alert(1)');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    const link = page.locator('#file-list .text-item-card a[href="https://example.com/path"]');
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    await expect(page.locator('#file-list .text-item-card', { hasText: 'javascript:alert(1)' }).locator('a')).toHaveCount(0);
  });

  test('the lamp is a meter with the time left in words', async ({ page }) => {
    const room = await createRoom({ ttl: 3600 });
    await page.goto(`${room.base}/c/${room.creator_token}`);
    const meter = page.locator('#lamp-meter');
    await expect(meter).toHaveAttribute('role', 'meter');
    await expect(meter).toHaveAttribute('aria-valuemax', '3600');
    await expect(meter).toHaveAttribute('aria-valuetext', /minutes left/);
    await expect(page.locator('#countdown')).toHaveText(/^\d+ minutes$/);
  });

  test('in the last two minutes a single warning appears', async ({ page }) => {
    test.setTimeout(60_000);
    const room = await createRoom({ ttl: 60 });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    const banner = page.locator('#expiry-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toHaveAttribute('role', 'alert');
    await expect(banner).toContainText('Save anything you need now.');
    await expect(page.locator('[role="alert"]:visible')).toHaveCount(1);
    await expect(page.locator('#lamp')).toHaveClass(/is-late/);
    await expect(page.locator('.lt-lamp-text')).toHaveText(/^Lamp off in \d\d:\d\d\s*$/);
  });

  test('no horizontal scroll at 320 px with a very long file name @responsive', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'a'.repeat(120) + '.txt', 'x');
    await mustPostText(room, 'b'.repeat(400));
    for (const url of [`${room.base}/c/${room.creator_token}`, `${room.base}/r/${room.participant_token}`]) {
      await page.goto(url);
      await waitForPolls(page, 1);
      const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      expect(over, url).toBeLessThanOrEqual(0);
    }
  });

  test('keyboard shortcuts: / finds, N writes a note, ? lists them', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('body').click({ position: { x: 5, y: 5 } });
    await page.keyboard.press('/');
    await expect(page.locator('#search-input')).toBeFocused();
    await page.locator('body').click({ position: { x: 5, y: 5 } });
    await page.keyboard.press('n');
    await expect(page.locator('#text-input')).toBeFocused();
    await page.keyboard.press('Escape');
    await page.keyboard.press('?');
    await expect(page.locator('#shortcuts-dialog')).toBeVisible();
    await expect(page.locator('#shortcuts-dialog')).toContainText('HAMAL v0.8.14');
  });

  test('a photo that cannot be loaded shows what it is instead of a broken image', async ({ page }) => {
    const room = await createRoom();
    const f = await mustUpload(room, 'gone.jpg', PNG_1PX, { type: 'image/jpeg' });
    await page.route(`**/files/${f.file_id}`, (route) => route.fulfill({ status: 404, body: 'not found' }));
    await page.goto(`${room.base}/c/${room.creator_token}`);
    const media = page.locator('#file-list .file-row', { hasText: 'gone.jpg' }).locator('.lt-print-media');
    await expect(media).toHaveClass(/is-missing/);
    await expect(media).toContainText('Preview unavailable');
    await expect(media.locator('img')).toHaveCount(0);
    // The file itself is still listed with its actions.
    await expect(page.getByRole('button', { name: 'View gone.jpg' })).toBeVisible();
  });

  test('text typed into a note is dark ink on the white card', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#write-note-btn').click();
    await page.locator('#text-input').fill('docker compose up -d');
    const color = await page.locator('#text-input').evaluate((n) => getComputedStyle(n).color);
    expect(color).toBe('rgb(31, 27, 25)');
  });
});
