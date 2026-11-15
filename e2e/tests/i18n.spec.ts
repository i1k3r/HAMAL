import { test, expect } from './support/fixtures';
import { createRoom, mustPostText } from './support/api';
import { waitForPolls } from './support/page';

test.describe('languages', () => {
  for (const locale of ['tr-TR', 'ar', 'ja-JP']) {
    test(`the participant page is translated for ${locale}`, async ({ browser }) => {
      const context = await browser.newContext({ locale });
      const page = await context.newPage();
      const room = await createRoom();
      await mustPostText(room, 'translated page check');
      await page.goto(`${room.base}/r/${room.participant_token}`);
      // The first render comes from the Go template in English; site.js re-renders the list with
      // translated labels on the first poll (baseline behaviour).
      await waitForPolls(page, 1);
      await page.locator('#tab-text').click();
      const card = page.locator('#text-list .text-item-card', { hasText: 'translated page check' });
      const copyLabel = (await card.locator('.copy-label').textContent())?.trim();
      expect(copyLabel).toBeTruthy();
      expect(copyLabel).not.toBe('Copy');
      // Note: the sender role label is "CLIENT" in every locale at baseline, so it is not checked.
      const text = await page.locator('body').innerText();
      expect(text).not.toContain('undefined');
      await context.close();
    });
  }

  test('English is used when no supported language matches', async ({ browser }) => {
    const context = await browser.newContext({ locale: 'sw-KE' });
    const page = await context.newPage();
    const room = await createRoom();
    await mustPostText(room, 'fallback check');
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await waitForPolls(page, 1);
    await page.locator('#tab-text').click();
    const card = page.locator('#text-list .text-item-card', { hasText: 'fallback check' });
    await expect(card.locator('.copy-label')).toHaveText('Copy');
    await context.close();
  });
});
