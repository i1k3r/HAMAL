import type { Page, Request, Response, TestInfo } from '@playwright/test';

/*
 * Test-only failure diagnostics for intermittent WebKit transfer failures.
 *
 * Background: CI runs 43641965031 and 43641969624 showed the WebKit multi-file picker test losing
 * exactly one of three uploads to an HTTP 400 (a different file each time), and the WebKit
 * text-send test failing because a note was not accepted. These diagnostics record, without
 * changing what the page does:
 *   - every upload (POST …/files?filename=…) and text send (POST …/texts): time, duration, HTTP
 *     status, request Content-Type and Content-Length, and the size of the request body the
 *     browser reports; for any response >= 400 (or a failed request) also the response body;
 *   - every GET …/files and GET …/texts response: time, status and the returned item count/names;
 *   - document.hidden / visibilityState at load and on every visibilitychange;
 *   - optional sampled DOM counts (for example the rendered file rows);
 *   - on failure, the server-side state supplied by the test.
 * Nothing is printed when the test passes. On failure, report() prints one timeline per watched
 * page and attaches it to the test as `transfer-diagnostics.txt`.
 */

type Event = { t: number; page: string; kind: string; detail: string };

const UPLOAD = /\/api\/v1\/rooms\/[^/]+\/files\?filename=/;
const FILES = /\/api\/v1\/rooms\/[^/]+\/files$/;
const TEXTS = /\/api\/v1\/rooms\/[^/]+\/texts$/;

type Watched = { label: string; page: Page; samplers: Array<{ name: string; selector: string; last: number }> };

export class TransferDiagnostics {
  private readonly t0 = Date.now();
  private readonly events: Event[] = [];
  private readonly started = new Map<Request, number>();
  private readonly watched: Watched[] = [];
  private timer: ReturnType<typeof setInterval> | undefined;

  private log(page: string, kind: string, detail: string, at = Date.now()): void {
    this.events.push({ t: at - this.t0, page, kind, detail });
  }

  /**
   * Starts recording for `page`. Call before page.goto(). `count` maps a name to a selector whose
   * match count is sampled every 250 ms (only changes are logged).
   */
  async watch(page: Page, label: string, count: Record<string, string> = {}): Promise<void> {
    const w: Watched = {
      label,
      page,
      samplers: Object.entries(count).map(([name, selector]) => ({ name, selector, last: -1 })),
    };
    this.watched.push(w);

    await page.addInitScript(() => {
      const win = window as any;
      win.__hamalVisibility = [
        { at: Date.now(), event: 'init', hidden: document.hidden, state: document.visibilityState },
      ];
      document.addEventListener('visibilitychange', () => {
        win.__hamalVisibility.push({
          at: Date.now(),
          event: 'visibilitychange',
          hidden: document.hidden,
          state: document.visibilityState,
        });
      });
    });

    page.on('request', (req) => {
      const kind = this.writeKind(req);
      if (kind) {
        this.started.set(req, Date.now());
        this.log(label, `${kind}:start`, this.describe(req));
      } else if (this.readKind(req)) {
        this.log(label, `${this.readKind(req)}:request`, `GET sent`);
      }
    });
    page.on('requestfinished', async (req) => {
      const kind = this.writeKind(req);
      if (!kind) return;
      const end = Date.now();
      const res = await req.response().catch(() => null);
      const status = res?.status() ?? 0;
      const headers = await this.requestHeaders(req);
      let line = `${this.describe(req)} status=${status || '?'} duration=${end - (this.started.get(req) ?? end)}ms ${headers}`;
      if (!res || status >= 400) line += ` response-body=${await this.body(res)}`;
      this.log(label, `${kind}:end`, line, end);
    });
    page.on('requestfailed', async (req) => {
      const kind = this.writeKind(req) ?? this.readKind(req);
      if (!kind) return;
      const end = Date.now();
      const headers = this.writeKind(req) ? ` ${await this.requestHeaders(req)}` : '';
      this.log(
        label,
        `${kind}:failed`,
        `${this.describe(req)} error=${req.failure()?.errorText ?? '?'} duration=${end - (this.started.get(req) ?? end)}ms${headers}`,
        end,
      );
    });
    page.on('response', async (res) => {
      const kind = this.readKind(res.request());
      if (!kind) return;
      const at = Date.now();
      let summary: string;
      try {
        const body = await res.json();
        if (kind === 'files') {
          const names = (body.files ?? []).map((f: any) => f.filename);
          summary = `count=${names.length} [${names.join(', ')}]`;
        } else {
          const texts = body.texts ?? [];
          summary = `count=${texts.length} newest=${JSON.stringify(String(texts.at(-1)?.content ?? '').slice(0, 40))}`;
        }
      } catch (err) {
        summary = `body unreadable (${err instanceof Error ? err.message : String(err)})`;
      }
      this.log(label, `${kind}:response`, `status=${res.status()} ${summary}`, at);
    });

    if (!this.timer) {
      this.timer = setInterval(() => void this.sample(), 250);
    }
  }

  /**
   * Stops sampling. When `failed`, adds visibility history and the server state from `server`,
   * prints the timeline and attaches it. Call before closing the watched pages.
   */
  async report(testInfo: TestInfo, failed: boolean, server?: () => Promise<string>): Promise<void> {
    if (this.timer) clearInterval(this.timer);
    if (!failed) return;

    for (const w of this.watched) {
      try {
        const visibility: Array<{ at: number; event: string; hidden: boolean; state: string }> =
          await w.page.evaluate(() => (window as any).__hamalVisibility ?? []);
        for (const v of visibility) this.log(w.label, 'visibility', `${v.event} hidden=${v.hidden} state=${v.state}`, v.at);
        const now = await w.page.evaluate(() => ({ hidden: document.hidden, state: document.visibilityState }));
        this.log(w.label, 'visibility', `at report hidden=${now.hidden} state=${now.state}`);
      } catch (err) {
        this.log(w.label, 'visibility', `unavailable (${err instanceof Error ? err.message : String(err)})`);
      }
    }
    if (server) {
      try {
        this.log('server', 'server', await server());
      } catch (err) {
        this.log('server', 'server', `unavailable (${err instanceof Error ? err.message : String(err)})`);
      }
    }

    const lines = this.events
      .sort((a, b) => a.t - b.t)
      .map((e) => `${String(e.t).padStart(6)} ms  ${e.page.padEnd(8)} ${e.kind.padEnd(16)} ${e.detail}`);
    const text = [`Transfer diagnostics for "${testInfo.title}" [${testInfo.project.name}]`, ...lines].join('\n');
    console.log(text);
    await testInfo.attach('transfer-diagnostics.txt', { body: text, contentType: 'text/plain' });
  }

  private async sample(): Promise<void> {
    for (const w of this.watched) {
      for (const s of w.samplers) {
        try {
          const n = await w.page.locator(s.selector).count();
          if (n !== s.last) {
            s.last = n;
            this.log(w.label, 'dom', `${s.name} = ${n}`);
          }
        } catch {
          /* page closed */
        }
      }
    }
  }

  private writeKind(req: Request): 'upload' | 'text-send' | undefined {
    if (req.method() !== 'POST') return undefined;
    if (UPLOAD.test(req.url())) return 'upload';
    if (TEXTS.test(new URL(req.url()).pathname)) return 'text-send';
    return undefined;
  }

  private readKind(req: Request): 'files' | 'texts' | undefined {
    if (req.method() !== 'GET') return undefined;
    const p = new URL(req.url()).pathname;
    if (FILES.test(p)) return 'files';
    if (TEXTS.test(p)) return 'texts';
    return undefined;
  }

  private describe(req: Request): string {
    const name = new URL(req.url()).searchParams.get('filename');
    return name ? name : new URL(req.url()).pathname.replace(/^\/api\/v1\/rooms\/[^/]+/, '');
  }

  /** Content-Type and Content-Length as sent (allHeaders includes headers added by the network stack). */
  private async requestHeaders(req: Request): Promise<string> {
    let headers: Record<string, string> = {};
    try {
      headers = await req.allHeaders();
    } catch {
      headers = req.headers();
    }
    let bodyBytes = 'unknown';
    try {
      const buf = req.postDataBuffer();
      bodyBytes = buf ? String(buf.length) : 'none';
    } catch {
      /* not available */
    }
    return `content-type=${JSON.stringify(headers['content-type'] ?? '(absent)')} content-length=${headers['content-length'] ?? '(absent)'} request-body-bytes=${bodyBytes}`;
  }

  private async body(res: Response | null): Promise<string> {
    if (!res) return '(no response)';
    try {
      return JSON.stringify((await res.text()).slice(0, 500));
    } catch (err) {
      return `(unreadable: ${err instanceof Error ? err.message : String(err)})`;
    }
  }
}
