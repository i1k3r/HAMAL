import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

/** Presses Tab up to `max` times and returns a description of every element that got focus. */
async function tabStops(page: import('@playwright/test').Page, max = 80): Promise<string[]> {
  const stops: string[] = [];
  await page.locator('body').focus();
  for (let i = 0; i < max; i++) {
    await page.keyboard.press('Tab');
    const d = await page.evaluate(() => {
      const a = document.activeElement as HTMLElement | null;
      if (!a || a === document.body) return 'BODY';
      const inDropzone = !!a.closest('#dropzone');
      return `${a.tagName}#${a.id || ''}${inDropzone ? '[in-dropzone]' : ''}`;
    });
    if (stops.length && d === stops[0]) break; // wrapped around
    stops.push(d);
  }
  return stops;
}

test.describe('keyboard access', () => {
  test("participant's upload area is reachable with Tab", async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await expect(page.locator('#dropzone')).toBeVisible();
    const stops = await tabStops(page);
    expect(stops.some((s) => s.includes('#dropzone') || s.includes('[in-dropzone]'))).toBe(true);
  });

  test("creator's upload area is reachable with Tab", async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await expect(page.locator('#dropzone')).toBeVisible();
    const stops = await tabStops(page);
    expect(stops.length, 'setup: Tab moves focus through the page').toBeGreaterThan(5);

    // Formerly KF-006.
    const reachable = stops.some(
      (s) => s.includes('#dropzone') || s.includes('[in-dropzone]') || s.includes('#file-input'),
    );
    expect(reachable, "the creator's upload area is reachable with Tab").toBe(true);
  });
});
