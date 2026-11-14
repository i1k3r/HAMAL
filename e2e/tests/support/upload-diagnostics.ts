import type { Page, Request, TestInfo } from '@playwright/test';

/*
 * Test-only diagnostics for the intermittent WebKit failure of
 * "upload › several files can be chosen with the picker" (CI run 43640379399: the list stayed at
 * 2 of 3 rows for the whole 12 s wait). It records, without changing what the page does:
 *   - every upload request: filename, start/end time, duration, HTTP status or failure;
 *   - every GET /files response: time, status, file count and filenames (polls and the refresh
 *     site.js does after each upload use the same URL, so both are captured);
 *   - document.hidden / visibilityState at start and every visibilitychange (site.js polls every
 *     3.5 s when visible and every 12 s when hidden);
 *   - the rendered row count over time and, on failure, what the server holds.
 * Nothing is printed when the test passes. On failure, report() prints a timeline and attaches it.
 */

type Event = { t: number; kind: string; detail: string };

const UPLOAD = /\/api\/v1\/rooms\/[^/]+\/files\?filename=/;
const FILES = /\/api\/v1\/rooms\/[^/]+\/files$/;

export class UploadDiagnostics {
  private readonly t0 = Date.now();
  private readonly events: Event[] = [];
  private readonly started = new Map<Request, number>();
  private sampler: ReturnType<typeof setInterval> | undefined;
  private lastRows = -1;

  constructor(private readonly page: Page) {}

  private log(kind: string, detail: string, at = Date.now()): void {
    this.events.push({ t: at - this.t0, kind, detail });
  }

  /** Call before page.goto(). */
  async attach(): Promise<void> {
    await this.page.addInitScript(() => {
      const w = window as any;
      w.__hamalVisibility = [
        { at: Date.now(), event: 'init', hidden: document.hidden, state: document.visibilityState },
      ];
      document.addEventListener('visibilitychange', () => {
        w.__hamalVisibility.push({
          at: Date.now(),
          event: 'visibilitychange',
          hidden: document.hidden,
          state: document.visibilityState,
        });
      });
    });

    this.page.on('request', (req) => {
      if (req.method() === 'POST' && UPLOAD.test(req.url())) {
        this.started.set(req, Date.now());
        this.log('upload:start', this.uploadName(req));
      } else if (req.method() === 'GET' && FILES.test(new URL(req.url()).pathname)) {
        this.log('files:request', 'GET /files sent');
      }
    });
    this.page.on('requestfinished', async (req) => {
      if (req.method() === 'POST' && UPLOAD.test(req.url())) {
        const end = Date.now();
        const begin = this.started.get(req) ?? end;
        const res = await req.response().catch(() => null);
        this.log('upload:end', `${this.uploadName(req)} status=${res?.status() ?? '?'} duration=${end - begin}ms`, end);
      }
    });
    this.page.on('requestfailed', (req) => {
      if (req.method() === 'POST' && UPLOAD.test(req.url())) {
        const end = Date.now();
        const begin = this.started.get(req) ?? end;
        this.log('upload:failed', `${this.uploadName(req)} error=${req.failure()?.errorText ?? '?'} duration=${end - begin}ms`, end);
      } else if (req.method() === 'GET' && FILES.test(new URL(req.url()).pathname)) {
        this.log('files:failed', `error=${req.failure()?.errorText ?? '?'}`);
      }
    });
    this.page.on('response', async (res) => {
      const req = res.request();
      if (req.method() !== 'GET' || !FILES.test(new URL(res.url()).pathname)) return;
      const at = Date.now();
      let summary = '';
      try {
        const body = await res.json();
        const names = (body.files ?? []).map((f: any) => f.filename);
        summary = `count=${names.length} [${names.join(', ')}]`;
      } catch (err) {
        summary = `body unreadable (${err instanceof Error ? err.message : String(err)})`;
      }
      this.log('files:response', `status=${res.status()} ${summary}`, at);
    });

    // Rendered row count, sampled every 250 ms; only changes are logged.
    this.sampler = setInterval(async () => {
      try {
        const n = await this.page.locator('#file-list .file-row').count();
        if (n !== this.lastRows) {
          this.lastRows = n;
          this.log('rows', `#file-list .file-row = ${n}`);
        }
      } catch {
        /* page closed */
      }
    }, 250);
  }

  /** Stops sampling. When `failed`, adds visibility history and server state, prints and attaches. */
  async report(testInfo: TestInfo, failed: boolean, serverFiles: () => Promise<string[]>): Promise<void> {
    if (this.sampler) clearInterval(this.sampler);
    if (!failed) return;

    try {
      const visibility: Array<{ at: number; event: string; hidden: boolean; state: string }> =
        await this.page.evaluate(() => (window as any).__hamalVisibility ?? []);
      for (const v of visibility) this.log('visibility', `${v.event} hidden=${v.hidden} state=${v.state}`, v.at);
      const now = await this.page.evaluate(() => ({ hidden: document.hidden, state: document.visibilityState }));
      this.log('visibility', `at report hidden=${now.hidden} state=${now.state}`);
    } catch (err) {
      this.log('visibility', `unavailable (${err instanceof Error ? err.message : String(err)})`);
    }
    try {
      this.log('server', `files on server at report: [${(await serverFiles()).join(', ')}]`);
    } catch (err) {
      this.log('server', `unavailable (${err instanceof Error ? err.message : String(err)})`);
    }

    const lines = this.events
      .sort((a, b) => a.t - b.t)
      .map((e) => `${String(e.t).padStart(6)} ms  ${e.kind.padEnd(15)} ${e.detail}`);
    const text = [`Upload diagnostics for "${testInfo.title}" [${testInfo.project.name}]`, ...lines].join('\n');
    console.log(text);
    await testInfo.attach('upload-diagnostics.txt', { body: text, contentType: 'text/plain' });
  }

  private uploadName(req: Request): string {
    return new URL(req.url()).searchParams.get('filename') ?? '?';
  }
}
