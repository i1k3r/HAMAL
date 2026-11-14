#!/usr/bin/env node
// Contract check for the known-failure registry and the quarantine list (test-only).
//
// Fails (exit 1) when:
//   - an entry is malformed or its id is duplicated;
//   - an entry's fixInPhase is <= the current phase in phase.txt (it should have been removed);
//   - an entry is not used by any spec, or a spec uses an id that is not registered;
//   - a quarantine entry is malformed or past its expiry date;
//   - `--gate` is passed and the quarantine is not empty (every phase gate requires that).

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.dirname(fileURLToPath(import.meta.url));
const gate = process.argv.includes('--gate');
const problems = [];

const phase = Number(fs.readFileSync(path.join(root, 'phase.txt'), 'utf8').trim());
if (!Number.isInteger(phase) || phase < 0) problems.push(`phase.txt must hold a non-negative integer, got "${phase}"`);

const registry = JSON.parse(fs.readFileSync(path.join(root, 'known-failures.json'), 'utf8'));
const ids = new Set();
for (const e of registry) {
  for (const key of ['id', 'title', 'spec', 'expectedError', 'fixInPhase']) {
    if (e[key] === undefined || e[key] === '') problems.push(`${e.id ?? '(no id)'}: missing "${key}"`);
  }
  if (ids.has(e.id)) problems.push(`${e.id}: duplicate id`);
  ids.add(e.id);
  if (typeof e.fixInPhase === 'number' && e.fixInPhase <= phase) {
    problems.push(`${e.id}: fixInPhase ${e.fixInPhase} <= current phase ${phase}; fix it and remove the entry`);
  }
  try {
    new RegExp(e.expectedError);
  } catch (err) {
    problems.push(`${e.id}: expectedError is not a valid regex (${err.message})`);
  }
  if (e.spec && !fs.existsSync(path.join(root, e.spec))) problems.push(`${e.id}: spec ${e.spec} does not exist`);
}

function walk(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((d) =>
    d.isDirectory() ? walk(path.join(dir, d.name)) : d.name.endsWith('.ts') ? [path.join(dir, d.name)] : [],
  );
}
const used = new Map();
for (const file of walk(path.join(root, 'tests'))) {
  const src = fs.readFileSync(file, 'utf8');
  for (const m of src.matchAll(/knownFailure\(\s*['"`](KF-\d+)['"`]/g)) {
    // known-failures.json uses forward slashes; path.relative() uses backslashes on Windows.
    used.set(m[1], path.relative(root, file).replace(/\\/g, '/'));
  }
}
for (const id of ids) if (!used.has(id)) problems.push(`${id}: registered but not used by any spec`);
for (const [id, file] of used) if (!ids.has(id)) problems.push(`${id}: used in ${file} but not registered`);
for (const e of registry) {
  if (used.has(e.id) && used.get(e.id) !== e.spec) {
    problems.push(`${e.id}: registered for ${e.spec} but used in ${used.get(e.id)}`);
  }
}

const quarantine = JSON.parse(fs.readFileSync(path.join(root, 'quarantine.json'), 'utf8'));
const today = new Date().toISOString().slice(0, 10);
for (const q of quarantine) {
  for (const key of ['test', 'owner', 'issue', 'expires']) {
    if (!q[key]) problems.push(`quarantine "${q.test ?? '?'}": missing "${key}"`);
  }
  if (q.expires && q.expires < today) problems.push(`quarantine "${q.test}": expired on ${q.expires}`);
}
if (gate && quarantine.length > 0) {
  problems.push(`phase gate requires an empty quarantine; ${quarantine.length} test(s) quarantined`);
}

console.log(`phase ${phase}: ${registry.length} known failure(s), ${quarantine.length} quarantined test(s)`);
for (const e of registry) console.log(`  ${e.id} (fix in phase ${e.fixInPhase}): ${e.title}`);
if (problems.length) {
  console.error('\nKnown-failure / quarantine check failed:');
  for (const p of problems) console.error(`  - ${p}`);
  process.exit(1);
}
console.log('OK');
