import type { Page, Response } from '@playwright/test';

const POLL_TAIL = /\/api\/v1\/rooms\/[^/]+\/texts$/;

/**
 * Waits for `count` complete poll cycles. A cycle fetches room, files, then texts (site.js
 * pollStatus); the texts GET is the last request of a cycle, so it marks completion.
 */
export async function waitForPolls(page: Page, count = 1, timeout = 15_000): Promise<void> {
  for (let i = 0; i < count; i++) {
    await page.waitForResponse(
      (r: Response) => r.request().method() === 'GET' && POLL_TAIL.test(new URL(r.url()).pathname),
      { timeout },
    );
  }
  // Let the render that follows the response run.
  await page.waitForTimeout(150);
}

/** Marks the element that actually scrolls `selector`'s content and returns its CSS selector. */
export async function markScroller(page: Page, selector: string): Promise<string> {
  const found = await page.evaluate((sel) => {
    let el = document.querySelector(sel) as HTMLElement | null;
    while (el) {
      const s = getComputedStyle(el);
      if ((s.overflowY === 'auto' || s.overflowY === 'scroll') && el.scrollHeight > el.clientHeight) {
        el.setAttribute('data-e2e-scroller', '');
        return true;
      }
      el = el.parentElement;
    }
    return false;
  }, selector);
  return found ? '[data-e2e-scroller]' : ':root';
}

export async function scrollTo(page: Page, scroller: string, top: number): Promise<void> {
  await page.evaluate(
    ([sel, y]) => {
      const el = sel === ':root' ? document.scrollingElement! : document.querySelector(sel as string)!;
      el.scrollTop = y as number;
    },
    [scroller, top] as const,
  );
}

/** The first element matching `rows` whose top edge is inside the viewport: its text and top. */
export async function firstVisible(
  page: Page,
  rows: string,
  label: string,
): Promise<{ label: string; top: number } | null> {
  return page.evaluate(
    ([rowsSel, labelSel]) => {
      for (const row of Array.from(document.querySelectorAll(rowsSel))) {
        const r = row.getBoundingClientRect();
        if (r.height > 0 && r.top >= 0 && r.top < window.innerHeight) {
          const l = row.querySelector(labelSel);
          return { label: (l ? l.textContent : row.textContent || '').trim(), top: Math.round(r.top) };
        }
      }
      return null;
    },
    [rows, label] as const,
  );
}

/** Creates a File inside the page and dispatches a paste event with it on `target`. */
export async function pasteImage(page: Page, targetSelector: string | null): Promise<void> {
  await page.evaluate((sel) => {
    const bytes = Uint8Array.from(
      atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=='),
      (c) => c.charCodeAt(0),
    );
    const dt = new DataTransfer();
    dt.items.add(new File([bytes], 'clip.png', { type: 'image/png' }));
    const ev = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    const target = sel ? document.querySelector(sel)! : document.body;
    target.dispatchEvent(ev);
  }, targetSelector);
}

/** Dispatches a drop of in-page Files on `selector`. */
export async function dropFiles(page: Page, selector: string, names: string[]): Promise<void> {
  await page.evaluate(
    ([sel, fileNames]) => {
      const dt = new DataTransfer();
      for (const n of fileNames as string[]) dt.items.add(new File([`content of ${n}`], n, { type: 'text/plain' }));
      const el = document.querySelector(sel as string)!;
      el.dispatchEvent(new DragEvent('dragover', { dataTransfer: dt, bubbles: true, cancelable: true }));
      el.dispatchEvent(new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }));
    },
    [selector, names] as const,
  );
}
