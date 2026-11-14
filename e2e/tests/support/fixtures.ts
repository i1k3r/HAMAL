import { test as base, expect, type Page } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

/*
 * Shared test fixture. Every spec imports `test` and `expect` from here, which adds three
 * automatic guards to every test:
 *   1. No request may leave the HAMAL origins (fonts, scripts, images are self-hosted).
 *   2. No uncaught page error (`pageerror`).
 *   3. No console error, except the browser's own "Failed to load resource" network log line
 *      (tests that provoke HTTP errors or dropped connections assert on those explicitly).
 * A test that expects more console errors can widen the allow-list with
 * `test.use({ allowedConsoleErrors: [/pattern/] })`.
 */

const ALLOWED_ORIGINS = new Set(['http://127.0.0.1:7799', 'http://127.0.0.1:7800', 'http://127.0.0.1:7801']);
// Chromium logs every failed resource load (HTTP error status or net:: error) as a console error.
const NETWORK_LOG = /^Failed to load resource: /;

type Guards = {
  allowedConsoleErrors: RegExp[];
  guards: void;
};

export const test = base.extend<Guards>({
  allowedConsoleErrors: [[], { option: true }],
  guards: [
    async ({ context, allowedConsoleErrors }, use, testInfo) => {
      const external: string[] = [];
      const errors: string[] = [];

      await context.route(
        (url) => {
          if (url.protocol === 'data:' || url.protocol === 'blob:' || url.protocol === 'about:') return false;
          return !ALLOWED_ORIGINS.has(url.origin);
        },
        async (route) => {
          external.push(route.request().url());
          await route.abort('blockedbyclient');
        },
      );

      const watch = (page: Page) => {
        page.on('pageerror', (err) => errors.push(`pageerror: ${err.message}`));
        page.on('console', (msg) => {
          if (msg.type() !== 'error') return;
          const text = msg.text();
          if (NETWORK_LOG.test(text)) return;
          if (allowedConsoleErrors.some((re) => re.test(text))) return;
          errors.push(`console.error: ${text}`);
        });
      };
      context.pages().forEach(watch);
      context.on('page', watch);

      await use();

      if (testInfo.status === testInfo.expectedStatus) {
        expect(external, 'requests left the HAMAL origin').toEqual([]);
        expect(errors, 'unexpected page or console errors').toEqual([]);
      }
    },
    { auto: true },
  ],
});

export { expect };

/* ------------------------------------------------------------------------------------------ */
/* Known failures                                                                              */
/* ------------------------------------------------------------------------------------------ */

type KnownFailureEntry = {
  id: string;
  title: string;
  spec: string;
  expectedError: string;
  fixInPhase: number;
};

const registryPath = path.join(__dirname, '..', '..', 'known-failures.json');

function registry(): KnownFailureEntry[] {
  return JSON.parse(fs.readFileSync(registryPath, 'utf8'));
}

/**
 * Records a defect that exists on the baseline (see the handoff, Phase 0 "Test strategy").
 *
 * - Setup must happen before the call, as normal steps, so a broken setup is a regression.
 * - If `fn` throws and the message matches the entry's `expectedError`, the test passes with a
 *   `known-failure` annotation.
 * - If `fn` throws anything else, the error is rethrown: a new kind of failure is a regression.
 * - If `fn` does not throw, the test fails: the defect is gone and the entry must be removed.
 *
 * Put a stable marker in the assertion message (for example `expect(x, 'KF-001: …')`) so the
 * expectedError pattern can match it precisely.
 */
export async function knownFailure(id: string, fn: () => Promise<void>): Promise<void> {
  const entry = registry().find((e) => e.id === id);
  if (!entry) {
    throw new Error(`${id} is not registered in e2e/known-failures.json`);
  }
  let caught: unknown;
  try {
    await fn();
  } catch (err) {
    caught = err;
  }
  if (caught === undefined) {
    throw new Error(
      `${id} now passes: remove it from known-failures.json and turn it into a plain assertion.`,
    );
  }
  const message = caught instanceof Error ? caught.message : String(caught);
  if (!new RegExp(entry.expectedError).test(message)) {
    throw caught;
  }
  const observed = message.replace(/\u001b\[[0-9;]*m/g, '').split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 4).join(' | ');
  test.info().annotations.push({ type: 'known-failure', description: `${id}: ${entry.title} [observed: ${observed}]` });
}
