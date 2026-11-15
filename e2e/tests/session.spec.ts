import { test, expect } from './support/fixtures';
import { createRoom } from './support/api';

test.describe('session identity', () => {
  test('polls carry the client session id and own notes stay "self" after reload', async ({ page }) => {
    const room = await createRoom();
    const headers: string[] = [];
    page.on('request', (r) => {
      if (r.method() === 'GET' && /\/texts$/.test(new URL(r.url()).pathname)) {
        headers.push(r.headers()['x-client-session-id'] ?? '');
      }
    });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#write-note-btn').click();
    await page.locator('#text-input').fill('mine');
    await page.locator('#send-text-btn').click();
    const mine = page.locator('#file-list .text-item-card', { hasText: 'mine' });
    await expect(mine).toHaveClass(/client-self/);

    const stored = await page.evaluate((t) => sessionStorage.getItem(`hamal_client_session_${t}`), room.participant_token);
    expect(stored).toMatch(/^cs_[0-9a-f]+$/);
    await expect.poll(() => headers.length, { timeout: 12_000 }).toBeGreaterThan(0);
    expect(new Set(headers)).toEqual(new Set([stored]));

    await page.reload();
    await page.locator('#write-note-btn').click();
    await expect(page.locator('#file-list .text-item-card', { hasText: 'mine' })).toHaveClass(/client-self/);
  });
});
