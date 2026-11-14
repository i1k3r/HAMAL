# HAMAL regression harness

Test-only. Nothing in this folder is built into the binary or the Docker image (`e2e` is listed
in `.dockerignore`). It implements Phase 0 of the Light Table implementation handoff.

## Run it

Requirements: Go (version from `go.mod`), Node 22, and Playwright browsers for the pinned version.

```sh
cd e2e
npm ci
npx playwright install chromium webkit     # once per machine
npx playwright test                        # all behaviour projects
npx playwright test --project=chromium-1440
node check-known-failures.mjs              # registry + quarantine contract
node report-flakes.mjs                     # summary of the last run
```

Playwright starts two servers from the repository root with `go run ./cmd/lan-drop`, each with a
fresh temporary data directory:

| Server | Address | Used by | Settings |
| --- | --- | --- | --- |
| primary | 127.0.0.1:7799 | every spec except the two below | `LAN_DROP_MIN_TTL=60s`, `LAN_DROP_CLEANUP_INTERVAL=5s`, `LAN_DROP_SHARE_MANAGEMENT_RATE_LIMIT=10000`, otherwise product defaults |
| limited | 127.0.0.1:7800 | `share.spec.ts`, `quota.spec.ts` | as primary, plus `LAN_DROP_GLOBAL_SHARE_ENABLED=true`, 1 MiB file limit, 3 MiB room limit, 5 files per room |

Locally an already running server on those ports is reused; stop your own HAMAL first.

## Projects

| Project | Device | Runs |
| --- | --- | --- |
| `chromium-1440` | Desktop Chrome, 1440×900 | every behaviour spec |
| `chromium-1280` | Desktop Chrome, 1280×720 | tests tagged `@responsive` |
| `webkit-iphone13` | iPhone 13 (390×844) | tests tagged `@responsive` |
| `chromium-pixel5` | Pixel 5 | tests tagged `@responsive` |
| `chromium-320` | Chrome, 320×640, touch | tests tagged `@responsive` |
| `visual-desktop`, `visual-phone` | 1440×900 and 390×844 | `tests/visual/` only, and only when `HAMAL_VISUAL=1` |

Every test gets three automatic guards (`tests/support/fixtures.ts`): no request may leave the
HAMAL origins, no uncaught page error, and no console error other than the browser's own
"Failed to load resource" network log line.

## Known failures

A defect that exists on the baseline is recorded only through
`await knownFailure('KF-00N', async () => { …assertions… })`, never with `test.fail()`.

- Setup runs outside the block, so a broken setup fails as a regression.
- If the block throws an error matching the entry's `expectedError`, the test passes with a
  `known-failure` annotation that records what was observed.
- Any other error is rethrown: a new kind of failure is a regression.
- If the block does not throw, the test fails with "KF-00N now passes": remove the entry from
  `known-failures.json` and turn the block into a plain assertion in the same PR.

`check-known-failures.mjs` fails CI when an entry's `fixInPhase` is less than or equal to
`phase.txt`, when an entry is unused, or when a spec uses an unregistered id. Each phase PR bumps
`phase.txt`. An entry is added only after the defect reproduced in three consecutive runs on the
baseline.

## Flaky tests and quarantine

CI retries a failed test once. Failing both attempts is a regression and blocks the merge.
Passing only on retry is flaky: it does not block that PR but is listed in the job summary. New
or changed specs in a PR also run with `--repeat-each=5`, where any failure blocks.

A test that is flaky twice in any 10 consecutive runs on `main` goes into `quarantine.json`:

```json
[{ "test": "<exact test title>", "owner": "<name>", "issue": "<link>", "expires": "YYYY-MM-DD" }]
```

Quarantined tests are excluded from the blocking run and executed in a separate non-blocking CI
step. Entries may last at most 14 days. Every phase gate requires an empty quarantine
(`node check-known-failures.mjs --gate`).

## Visual baselines

Visual specs answer every room, files and texts poll from `fixtures/visual/` and mask
token- and clock-dependent elements (QR image, address field, countdowns, share links). They run
with locale `en-US`, timezone UTC, reduced motion and device scale factor 1.

Baselines are generated and compared only inside the official Playwright image at the pinned
version (`mcr.microsoft.com/playwright:v1.63.0-noble`), never on a developer machine. To create or
update them, run the **Test** workflow manually with `update_snapshots` enabled, download the
`visual-baselines` artifact into `tests/visual/__screenshots__/`, and commit it in a PR labelled
`baseline-update`. Until baselines are committed, the CI visual job reports that none exist.

## Files

| Path | Purpose |
| --- | --- |
| `playwright.config.ts` | servers, projects, reporters, quarantine filtering |
| `tests/support/fixtures.ts` | guards and `knownFailure` |
| `tests/support/api.ts` | arranging state through the public API |
| `tests/support/page.ts` | poll waiting, scroll helpers, paste and drop helpers |
| `tests/*.spec.ts` | one spec per feature area |
| `tests/visual/` | screenshot specs |
| `fixtures/media/clip.webm` | 4-second VP8 test video (generated with Playwright's recorder) |
| `fixtures/visual/` | mocked API responses for screenshots |
| `known-failures.json`, `phase.txt`, `quarantine.json` | registry, current phase, quarantine |
| `check-known-failures.mjs`, `report-flakes.mjs` | CI checks and summary |
