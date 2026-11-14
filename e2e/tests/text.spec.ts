import { test, expect, knownFailure } from './support/fixtures';
import { createRoom, mustPostText, postText, getJSON } from './support/api';
import { TransferDiagnostics } from './support/transfer-diagnostics';

test.describe('text notes', () => {
  test('a note sent from one device appears on the other, newest first @responsive', async ({ browser }, testInfo) => {
    const room = await createRoom();
    const creatorCtx = await browser.newContext();
    const creator = await creatorCtx.newPage();
    const phone = await browser.newPage();
    // Diagnostics for an intermittent WebKit failure (CI run 43641965031: the first note was not
    // accepted, so the input was not cleared); printed only on failure.
    const diag = new TransferDiagnostics();
    await diag.watch(creator, 'creator', { 'text cards': '#text-list .text-item-card' });
    await diag.watch(phone, 'phone', { 'text cards': '#text-list .text-item-card' });
    let failed = true;
    try {
      await creator.goto(`${room.base}/c/${room.creator_token}`);
      await creator.locator('#tab-text').click();

      await phone.goto(`${room.base}/r/${room.participant_token}`);
      await phone.locator('#tab-text').click();
      await phone.locator('#text-input').fill('first from phone');
      await phone.locator('#send-text-btn').click();
      await expect(phone.locator('#text-input')).toHaveValue('');
      await phone.locator('#text-input').fill('second from phone');
      await phone.locator('#send-text-btn').click();

      const cards = creator.locator('#text-list .text-item-card');
      await expect(cards.first()).toContainText('second from phone', { timeout: 12_000 });
      await expect(cards.nth(1)).toContainText('first from phone');
      // Sender's own notes are marked as self; the other device sees them as a peer's.
      await expect(phone.locator('#text-list .text-item-card').first()).toHaveClass(/client-self/);
      await expect(cards.first()).toHaveClass(/client-peer/);
      failed = false;
    } finally {
      await diag.report(testInfo, failed, async () => {
        const r = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/texts`);
        const texts = (r.json.texts ?? []).map((t: any) => `${t.sender_type}:${JSON.stringify(String(t.content).slice(0, 40))}`);
        return `texts on server: [${texts.join(', ')}]`;
      });
      await creatorCtx.close();
      await phone.close();
    }
  });

  test('the size counter tracks bytes and blocks text over the limit', async ({ page }) => {
    const room = await createRoom();
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#tab-text').click();
    const counter = page.locator('#text-size-counter');
    await expect(counter).toHaveText('0 B / 64 KB');

    await page.locator('#text-input').fill('hello');
    await expect(counter).toHaveText('5 B / 64 KB');
    await expect(page.locator('#send-text-btn')).toBeEnabled();

    // 40,000 two-byte characters = 80,000 bytes: within maxlength (characters) but over 64 KB.
    await page.locator('#text-input').fill('é'.repeat(40_000));
    await expect(page.locator('#send-text-btn')).toBeDisabled();
  });

  test('the server rejects a note over 64 KB with 413', async () => {
    const room = await createRoom();
    const r = await postText(room, 'a'.repeat(64 * 1024 + 1));
    expect(r.status).toBe(413);
    const ok = await postText(room, 'a'.repeat(64 * 1024));
    expect(ok.status).toBe(201);
  });

  test('copy puts the note on the clipboard and says so', async ({ page, context, browserName }) => {
    test.skip(browserName !== 'chromium', 'clipboard permissions are Chromium-only in Playwright');
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    const room = await createRoom();
    await mustPostText(room, 'kubectl get pods -A');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#tab-text').click();
    const card = page.locator('#text-list .text-item-card', { hasText: 'kubectl get pods -A' });
    await card.locator('.btn-copy-text').click();
    await expect(card.locator('.copy-label')).toHaveText('Copied!');
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('kubectl get pods -A');
  });

  test('copy does not claim success when copying failed', async ({ page }) => {
    // Every copy method fails: the async clipboard rejects and execCommand reports failure.
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText: () => Promise.reject(new Error('denied')), readText: () => Promise.reject(new Error('denied')) },
      });
      document.execCommand = () => false;
    });
    const room = await createRoom();
    await mustPostText(room, 'secret-token-123');
    await page.goto(`${room.base}/c/${room.creator_token}`);
    await page.locator('#tab-text').click();
    const card = page.locator('#text-list .text-item-card', { hasText: 'secret-token-123' });
    await card.locator('.btn-copy-text').click();
    await page.waitForTimeout(300);

    await knownFailure('KF-004', async () => {
      await expect(
        card.locator('.copy-label'),
        'KF-004: Copy must not report success when every copy method failed',
      ).not.toHaveText('Copied!', { timeout: 1_000 });
    });
  });

  test('at 200 notes the welcome message is evicted and the next note is refused', async ({ page }) => {
    // Baseline behaviour (6765e5e, text.go makeRoomForText): only *system* messages are evicted to
    // make room. Once 200 client notes exist, the next one is rejected with HTTP 400. (The handoff's
    // feature table described this row as "the 201st note evicts the oldest"; that is not what the
    // baseline does, so this test pins the actual behaviour.)
    test.setTimeout(90_000);
    const room = await createRoom();
    for (let i = 0; i < 200; i++) await mustPostText(room, `bulk note ${String(i).padStart(3, '0')}`);
    const texts = await getJSON(`${room.base}/api/v1/rooms/${room.participant_token}/texts`);
    expect(texts.status).toBe(200);
    expect(texts.json.texts.length).toBe(200);
    expect(texts.json.texts.every((t: any) => t.sender_type === 'client'), 'the system message was evicted').toBe(true);

    const refused = await postText(room, 'bulk note 200');
    expect(refused.status).toBe(400);
    expect(refused.json.error).toBe('room text message limit reached');

    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.locator('#tab-text').click();
    await expect(page.locator('#text-list .text-item-card')).toHaveCount(200);
    await expect(page.locator('#text-list .text-item-card').first()).toContainText('bulk note 199');
    await expect(page.locator('#text-count')).toHaveText('200');

    await page.locator('#text-input').fill('one more from the phone');
    await page.locator('#send-text-btn').click();
    await expect(page.locator('#text-compose-error')).toHaveText('room text message limit reached');
    await expect(page.locator('#text-list .text-item-card')).toHaveCount(200);
  });
});
