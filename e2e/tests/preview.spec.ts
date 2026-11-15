import { test, expect } from './support/fixtures';
import { createRoom, mustUpload, PNG_1PX, wav } from './support/api';
import { waitForPolls } from './support/page';
import fs from 'node:fs';
import path from 'node:path';

const clip = fs.readFileSync(path.join(__dirname, '..', 'fixtures', 'media', 'clip.webm'));

test.describe('media previews', () => {
  test('an image opens in the preview and Escape closes it', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'pixel.png', PNG_1PX, { type: 'image/png' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.getByRole('button', { name: 'View pixel.png' }).click();
    const overlay = page.locator('#file-preview-overlay');
    await expect(overlay).toBeVisible();
    await expect(page.locator('#preview-filename')).toHaveText('pixel.png');
    const img = page.locator('#preview-body img');
    await expect(img).toHaveAttribute('alt', 'pixel.png');
    await expect.poll(() => img.evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1);
    await page.keyboard.press('Escape');
    await expect(overlay).toBeHidden();
    await expect(page.locator('#preview-body')).toBeEmpty();
  });

  test('audio previews play and keep playing across polls', async ({ page, browserName }) => {
    test.skip(browserName === 'webkit', 'headless WebKit has no audio output device');
    const room = await createRoom();
    await mustUpload(room, 'tone.wav', wav(20), { type: 'audio/wav' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.getByRole('button', { name: 'View tone.wav' }).click();
    const audio = page.locator('#preview-body audio');
    await expect(audio).toHaveAttribute('controls', '');
    await audio.evaluate((a: HTMLAudioElement) => a.play());
    await expect.poll(() => audio.evaluate((a: HTMLAudioElement) => a.currentTime)).toBeGreaterThan(0.2);
    const t0 = await audio.evaluate((a: HTMLAudioElement) => a.currentTime);
    await waitForPolls(page, 2);
    const t1 = await audio.evaluate((a: HTMLAudioElement) => ({ t: a.currentTime, paused: a.paused }));
    expect(t1.paused).toBe(false);
    expect(t1.t, 'playback continued across polls instead of restarting').toBeGreaterThan(t0 + 3);
  });

  test('video previews load and keep their element across polls', async ({ page }) => {
    const room = await createRoom();
    await mustUpload(room, 'clip.webm', clip, { type: 'video/webm' });
    await page.goto(`${room.base}/r/${room.participant_token}`);
    await page.getByRole('button', { name: 'View clip.webm' }).click();
    const video = page.locator('#preview-body video');
    await expect(video).toHaveAttribute('controls', '');
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThanOrEqual(1);
    await video.evaluate((v: HTMLVideoElement) => {
      v.muted = true;
      v.loop = true;
      (v as any).__e2eMark = 'same-element';
      return v.play();
    });
    await waitForPolls(page, 2);
    const state = await page.locator('#preview-body video').evaluate((v: HTMLVideoElement) => ({
      mark: (v as any).__e2eMark,
      paused: v.paused,
      t: v.currentTime,
    }));
    expect(state.mark, 'the preview video element was not recreated by polling').toBe('same-element');
    expect(state.paused).toBe(false);
    expect(state.t).toBeGreaterThan(0);
  });
});
