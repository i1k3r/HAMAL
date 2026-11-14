import { defineConfig, devices, type Project } from '@playwright/test';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

/*
 * HAMAL regression harness (Phase 0). Test-only: nothing here ships in the binary or image.
 *
 * Two servers are started from the repository root with `go run ./cmd/lan-drop`:
 *   - primary (127.0.0.1:7799): the configuration from the handoff, default product limits.
 *   - limited (127.0.0.1:7800): global sharing enabled and small quotas, used only by
 *     share.spec.ts and quota.spec.ts so the primary server keeps default behaviour.
 * Each server gets a fresh temporary data directory per run.
 */

const repoRoot = path.resolve(__dirname, '..');

function dataDir(envKey: string, prefix: string): string {
  // The config file is evaluated in the runner and in every worker; create the directory once.
  if (!process.env[envKey]) {
    process.env[envKey] = fs.mkdtempSync(path.join(os.tmpdir(), prefix));
  }
  return process.env[envKey] as string;
}

export const PRIMARY_URL = 'http://127.0.0.1:7799';
export const LIMITED_URL = 'http://127.0.0.1:7800';

const commonEnv = {
  LAN_DROP_MIN_TTL: '60s',
  LAN_DROP_CLEANUP_INTERVAL: '5s',
  LAN_DROP_SHARE_MANAGEMENT_RATE_LIMIT: '10000',
  LAN_DROP_LOG_LEVEL: 'warn',
};

// Quarantined tests (see quarantine.json) are excluded from the blocking run and executed in a
// separate, non-blocking CI step with HAMAL_ONLY_QUARANTINED=1.
type QuarantineEntry = { test: string; owner: string; issue: string; expires: string };
const quarantine: QuarantineEntry[] = JSON.parse(
  fs.readFileSync(path.join(__dirname, 'quarantine.json'), 'utf8'),
);
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const quarantineRe = quarantine.length
  ? new RegExp(quarantine.map((q) => escape(q.test)).join('|'))
  : undefined;
const onlyQuarantined = process.env.HAMAL_ONLY_QUARANTINED === '1';

function behaviour(name: string, use: Project['use'], responsiveOnly: boolean): Project {
  const p: Project = { name, use, testIgnore: ['visual/**'] };
  if (onlyQuarantined) {
    p.grep = quarantineRe ?? /^$a/; // nothing quarantined: run nothing
  } else {
    if (responsiveOnly) p.grep = /@responsive/;
    if (quarantineRe) p.grepInvert = quarantineRe;
  }
  return p;
}

const projects: Project[] = [
  behaviour('chromium-1440', { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } }, false),
  behaviour('chromium-1280', { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 720 } }, true),
  behaviour('webkit-iphone13', { ...devices['iPhone 13'] }, true),
  behaviour('chromium-pixel5', { ...devices['Pixel 5'] }, true),
  behaviour(
    'chromium-320',
    { ...devices['Desktop Chrome'], viewport: { width: 320, height: 640 }, hasTouch: true, isMobile: true },
    true,
  ),
];

// Visual specs only run where pixels are reproducible: inside the pinned Playwright container
// (CI job `visual`), which sets HAMAL_VISUAL=1. Locally, run them through that container too.
if (process.env.HAMAL_VISUAL === '1') {
  const visualUse = {
    locale: 'en-US',
    timezoneId: 'UTC',
    reducedMotion: 'reduce' as const,
    deviceScaleFactor: 1,
  };
  projects.push(
    {
      name: 'visual-desktop',
      testDir: './tests/visual',
      use: { ...devices['Desktop Chrome'], ...visualUse, viewport: { width: 1440, height: 900 } },
    },
    {
      name: 'visual-phone',
      testDir: './tests/visual',
      use: {
        ...devices['Desktop Chrome'],
        ...visualUse,
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
  );
}

export default defineConfig({
  testDir: './tests',
  timeout: 45_000,
  expect: {
    timeout: 8_000,
    toHaveScreenshot: { maxDiffPixelRatio: 0.001, animations: 'disabled', caret: 'hide' },
  },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 4 : undefined,
  reporter: [
    ['list'],
    ['json', { outputFile: 'test-results/results.json' }],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{testFilePath}/{arg}{ext}',
  use: {
    trace: 'retain-on-failure',
    video: 'off',
    screenshot: 'only-on-failure',
  },
  projects,
  webServer: [
    {
      command: 'go run ./cmd/lan-drop',
      cwd: repoRoot,
      url: `${PRIMARY_URL}/readyz`,
      timeout: 180_000,
      reuseExistingServer: !process.env.CI,
      stdout: 'ignore',
      stderr: 'pipe',
      env: {
        ...commonEnv,
        LAN_DROP_DATA_DIR: dataDir('HAMAL_E2E_PRIMARY_DIR', 'hamal-e2e-primary-'),
        LAN_DROP_LISTEN_ADDR: '127.0.0.1:7799',
        LAN_DROP_BASE_URL: PRIMARY_URL,
      },
    },
    {
      command: 'go run ./cmd/lan-drop',
      cwd: repoRoot,
      url: `${LIMITED_URL}/readyz`,
      timeout: 180_000,
      reuseExistingServer: !process.env.CI,
      stdout: 'ignore',
      stderr: 'pipe',
      env: {
        ...commonEnv,
        LAN_DROP_DATA_DIR: dataDir('HAMAL_E2E_LIMITED_DIR', 'hamal-e2e-limited-'),
        LAN_DROP_LISTEN_ADDR: '127.0.0.1:7800',
        LAN_DROP_BASE_URL: LIMITED_URL,
        LAN_DROP_GLOBAL_SHARE_ENABLED: 'true',
        LAN_DROP_MAX_FILE_SIZE: '1048576',
        LAN_DROP_MAX_ROOM_SIZE: '3145728',
        LAN_DROP_MAX_FILES_PER_ROOM: '5',
      },
    },
  ],
});
