#!/usr/bin/env node
// Counts executions and failures of the multi-file picker test in a Playwright JSON report, per
// project and per upload-variant annotation (test-only; used by the upload-experiment CI job).
// Usage: node experiment-summary.mjs [path/to/results.json]
// Prints one line per (variant, project) and appends a Markdown table to $GITHUB_STEP_SUMMARY.
// Always exits 0: the experiment job reports counts, it does not gate.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.dirname(fileURLToPath(import.meta.url));
const reportPath = process.argv[2] ?? path.join(root, 'test-results', 'results.json');
const TITLE = 'several files can be chosen with the picker';

if (!fs.existsSync(reportPath)) {
  console.log(`No report at ${reportPath}`);
  process.exit(0);
}
const report = JSON.parse(fs.readFileSync(reportPath, 'utf8'));
const counts = new Map();

function visit(suite) {
  for (const spec of suite.specs ?? []) {
    if (!spec.title.includes(TITLE)) continue;
    for (const t of spec.tests ?? []) {
      const variant = (t.annotations ?? []).find((a) => a.type === 'upload-variant')?.description ?? 'upload-variant=?';
      const key = `${variant} project=${t.projectName}`;
      const c = counts.get(key) ?? { executions: 0, failures: 0 };
      for (const r of t.results ?? []) {
        if (r.status === 'skipped') continue;
        c.executions++;
        if (r.status !== 'passed') c.failures++;
      }
      counts.set(key, c);
    }
  }
  for (const s of suite.suites ?? []) visit(s);
}
for (const s of report.suites ?? []) visit(s);

const lines = [...counts.entries()].map(([k, c]) => `${k} executions=${c.executions} failures=${c.failures}`);
console.log(lines.length ? lines.join('\n') : 'No executions of the picker test found.');
if (process.env.GITHUB_STEP_SUMMARY) {
  const md = ['### Upload experiment', '', '| Variant / project | Executions | Failures |', '| --- | --- | --- |']
    .concat([...counts.entries()].map(([k, c]) => `| ${k} | ${c.executions} | ${c.failures} |`))
    .join('\n');
  fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, md + '\n');
}
