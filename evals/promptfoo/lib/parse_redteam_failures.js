#!/usr/bin/env node
/**
 * Extracts FAILING red-team cases from the promptfoo result JSONs and joins them
 * with the original case metadata (expect_safe, break_hypothesis) so each failure
 * carries everything a triage agent needs: the attack, the model's actual output,
 * which assertions failed and why, and what a safe answer looks like.
 *
 * Usage:  node lib/parse_redteam_failures.js [--all]
 *   default: only failing rows.  --all: every row (for inspection).
 * Writes results/redteam-failures.json and prints a per-surface summary.
 */
const fs = require('fs');
const path = require('path');

const RES = path.resolve(__dirname, '../results');
const SURFACES = ['waiter-ordering', 'waiter-concierge', 'director', 'wizard', 'guardrails'];
const showAll = process.argv.includes('--all');

// case metadata keyed by "surface :: [lens] label" (matches the rendered description)
const caseMeta = {};
try {
  const cases = JSON.parse(fs.readFileSync(path.join(RES, 'redteam-cases.json'), 'utf8'));
  for (const c of cases) {
    caseMeta[`${c.surface} :: [${c.lens}] ${c.label}`] = c;
  }
} catch (e) {
  console.error('warn: could not load redteam-cases.json:', e.message);
}

function rowsOf(file) {
  if (!fs.existsSync(file)) return null;
  const data = JSON.parse(fs.readFileSync(file, 'utf8'));
  return (data.results && data.results.results) || [];
}

const out = [];
const summary = {};

for (const surface of SURFACES) {
  const rows = rowsOf(path.join(RES, `redteam-${surface}.json`));
  if (!rows) {
    summary[surface] = 'no results';
    continue;
  }
  let pass = 0,
    fail = 0,
    errored = 0;
  for (const row of rows) {
    const ok = row.success === true;
    if (ok) pass++;
    else fail++;
    if (row.error || (row.response && row.response.error)) errored++;
    if (ok && !showAll) continue;

    const desc = row.description || (row.testCase && row.testCase.description) || '';
    const meta = caseMeta[`${surface} :: ${desc}`] || {};
    const output =
      (row.response && (row.response.output != null ? row.response.output : row.response.raw)) ??
      row.output ??
      '';
    const comps = (row.gradingResult && row.gradingResult.componentResults) || [];
    const failedAsserts = comps
      .filter((c) => c && c.pass === false)
      .map((c) => ({
        type: (c.assertion && c.assertion.type) || 'unknown',
        value: (c.assertion && (typeof c.assertion.value === 'string' ? c.assertion.value : JSON.stringify(c.assertion.value))) || '',
        reason: (c.reason || '').slice(0, 400),
      }));

    out.push({
      surface,
      description: desc,
      lens: meta.lens || '',
      attack_vars: row.vars || (row.testCase && row.testCase.vars) || {},
      model_output: typeof output === 'string' ? output : JSON.stringify(output),
      success: ok,
      failed_asserts: failedAsserts,
      expect_safe: meta.expect_safe || '',
      break_hypothesis: meta.break_hypothesis || '',
    });
  }
  summary[surface] = { pass, fail, errored, total: pass + fail };
}

fs.writeFileSync(path.join(RES, 'redteam-failures.json'), JSON.stringify(out, null, 2));

console.log('Red-team results summary (pass/total):');
let tp = 0,
  tt = 0;
for (const s of SURFACES) {
  const x = summary[s];
  if (typeof x === 'object' && x.total != null) {
    tp += x.pass;
    tt += x.total;
    console.log(`  ${s.padEnd(18)} ${x.pass}/${x.total} pass  (${x.fail} fail${x.errored ? `, ${x.errored} errored` : ''})`);
  } else {
    console.log(`  ${s.padEnd(18)} ${x}`);
  }
}
console.log(`  ${'TOTAL'.padEnd(18)} ${tp}/${tt} pass  (${tt - tp} fail)`);
console.log(`\nWrote ${out.filter((o) => !o.success).length} failing cases to results/redteam-failures.json`);
