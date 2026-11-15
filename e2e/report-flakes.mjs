#!/usr/bin/env node
// Summarises a Playwright JSON report (test-only, non-blocking).
//
// Classification used by CI:
//   regression  - failed on every attempt (Playwright already exits non-zero for these)
//   flaky       - failed, then passed on retry; listed here so it can be tracked/quarantined
//   known       - passed with a `known-failure` annotation (registered baseline defect)
// Writes a Markdown summary to $GITHUB_STEP_SUMMARY when available. Always exits 0.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.dirname(fileURLToPath(import.meta.url));
const reportPath = process.argv[2] ?? path.join(root, 'test-results', 'results.json');
if (!fs.existsSync(reportPath)) {
  console.log(`No report at ${reportPath}; nothing to summarise.`);
  process.exit(0);
}
const report = JSON.parse(fs.readFileSync(reportPath, 'utf8'));

const rows = { regression: [], flaky: [], known: [] };
function visit(suite, trail) {
  const here = suite.title ? [...trail, suite.title] : trail;
  for (const spec of suite.specs ?? []) {
    for (const t of spec.tests ?? []) {
      const name = `${[...here, spec.title].join(' › ')} [${t.projectName}]`;
      if (t.status === 'unexpected') rows.regression.push(name);
      if (t.status === 'flaky') rows.flaky.push(name);
      for (const a of t.annotations ?? []) {
        if (a.type === 'known-failure') rows.known.push(`${name}: ${a.description}`);
      }
    }
  }
  for (const s of suite.suites ?? []) visit(s, here);
}
for (const s of report.suites ?? []) visit(s, []);

const stats = report.stats ?? {};
const lines = [
  '### E2E summary',
  '',
  `expected: ${stats.expected ?? '?'} · unexpected (regressions): ${stats.unexpected ?? '?'} · flaky: ${stats.flaky ?? '?'} · skipped: ${stats.skipped ?? '?'}`,
  '',
];
for (const [kind, list] of Object.entries(rows)) {
  lines.push(`**${kind}** (${list.length})`);
  for (const item of list) lines.push(`- ${item}`);
  lines.push('');
}
const md = lines.join('\n');
console.log(md);
if (process.env.GITHUB_STEP_SUMMARY) fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, md + '\n');
