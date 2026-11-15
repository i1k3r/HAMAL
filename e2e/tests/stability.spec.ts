import { test, expect } from './support/fixtures';
import { createRoom, mustUpload, mustPostText } from './support/api';
import { waitForPolls, markScroller, scrollTo, firstVisible } from './support/page';

/*
 * Poll stability. The Light Table room pages render items by key, so a poll never rebuilds the
 * list: focus, scroll position and the newest notes stay put. (Formerly KF-001 to KF-003.)
 */

test.describe('poll stability', () => {
  test('focus stays on a Download link across a poll', async ({ page }) => {
    const room = await createRoom();
    for (let i = 0; i < 5; i++) await mustUpload(room, `report-${i}.txt`, `body ${i}`);
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(5);

    const link = page.locator('#file-list .file-row', { hasText: 'report-2.txt' }).locator('a[download]');
    await link.focus();
    await expect(link).toBeFocused();
    await waitForPolls(page, 1);

    {
      const focusedFile = await page.evaluate(() => {
        const a = document.activeElement as HTMLElement | null;
        const row = a ? a.closest('.file-row') as HTMLElement | null : null;
        return row ? row.dataset.fileId ?? null : a ? a.tagName : null;
      });
      const expectedFile = await page
        .locator('#file-list .file-row', { hasText: 'report-2.txt' })
        .getAttribute('data-file-id');
      expect(focusedFile, 'focus stays on the same Download link after a poll').toBe(expectedFile);
    }
  });

  test('visible file rows stay in place when a poll adds a file', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const room = await createRoom();
    for (let i = 0; i < 30; i++) await mustUpload(room, `file-${String(i).padStart(2, '0')}.txt`, 'x');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .file-row')).toHaveCount(30);

    const scroller = await markScroller(page, '#file-list');
    await scrollTo(page, scroller, 900);
    await page.waitForTimeout(200);
    const before = await firstVisible(page, '#file-list .file-row', '.lt-item-name');
    expect(before, 'setup: a file row is visible after scrolling').not.toBeNull();

    await mustUpload(room, 'file-new.txt', 'x');
    await expect(page.locator('#file-list .file-row')).toHaveCount(31, { timeout: 12_000 });
    await page.waitForTimeout(200);

    {
      const after = await firstVisible(page, '#file-list .file-row', '.lt-item-name');
      expect(
        after,
        'the row at the top of the viewport stays in place after a poll adds a file',
      ).toEqual(before);
    }
  });

  test('the text list keeps the newest notes in view across a poll and after sending', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const room = await createRoom();
    for (let i = 0; i < 30; i++) await mustPostText(room, `note ${String(i).padStart(2, '0')}\nsecond line\nthird line`);
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#file-list .text-item-card')).toHaveCount(30); // the room's own messages go to the room log, not the table

    const scroller = await markScroller(page, '#file-list');
    await scrollTo(page, scroller, 0);
    await page.waitForTimeout(200);

    await mustPostText(room, 'note 30 from another device');
    await expect(page.locator('#file-list .text-item-card')).toHaveCount(31, { timeout: 12_000 });
    await waitForPolls(page, 1);

    // Plain assertion: after a poll the newest notes stay in view. (The handoff candidate said the
    // list "scrolls to the oldest note after a poll"; that part does not reproduce on 6765e5e.)
    const afterPoll = await firstVisible(page, '#file-list .text-item-card', '.text-item-content');
    expect(afterPoll?.label, 'the newest note is at the top of the viewport after a poll').toContain(
      'note 30 from another device',
    );

    await page.locator('#write-note-btn').click();
    await page.locator('#text-input').fill('note 31 from this phone');
    await page.locator('#send-text-btn').click();
    await expect(page.locator('#file-list .text-item-card').first()).toContainText('note 31 from this phone');
    await page.waitForTimeout(200);

    // On a phone the composer sheet closes by itself after sending.
    await expect(page.locator('#composer')).toBeHidden();
    {
      const afterSend = await firstVisible(page, '#file-list .text-item-card', '.text-item-content');
      expect(afterSend?.label, 'the note just sent is visible after sending').toContain(
        'note 31 from this phone',
      );
    }
  });
});
