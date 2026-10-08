#!/usr/bin/env node
/**
 * Aggregates the per-surface promptfoo result JSONs (written by runner.sh to
 * results/<surface>.json) into a single model × surface scoreboard.
 *
 * Each surface compares the production "gemini" model for that surface against
 * gpt-4o-mini. Guardrails' gemini contender is gemini-2.5-flash-lite (its
 * production model); all other surfaces use gemini-2.5-flash. We roll both up
 * under a single "gemini" column so the cross-surface pick is readable.
 */
const fs = require('fs');
const path = require('path');

const RESULTS_DIR = path.resolve(__dirname, '../results');
const SURFACES = ['waiter-ordering', 'waiter-concierge', 'director', 'wizard', 'guardrails', 'assistant-v2'];

function family(label) {
  const l = String(label || '').toLowerCase();
  if (l.includes('gemini')) return 'gemini';
  if (l.includes('gpt')) return 'gpt-4o-mini';
  return label || 'unknown';
}

function loadSurface(name) {
  const file = path.join(RESULTS_DIR, `${name}.json`);
  if (!fs.existsSync(file)) return null;
  let data;
  try {
    data = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (e) {
    return null;
  }
  const rows = (data.results && data.results.results) || [];
  const agg = {};
  for (const row of rows) {
    const fam = family(row.provider && (row.provider.label || row.provider.id));
    const a = (agg[fam] = agg[fam] || { pass: 0, total: 0, latency: 0, latencyN: 0 });
    a.total += 1;
    if (row.success) a.pass += 1;
    if (typeof row.latencyMs === 'number') {
      a.latency += row.latencyMs;
      a.latencyN += 1;
    }
  }
  return agg;
}

function pct(p, t) {
  return t === 0 ? '  -  ' : `${((100 * p) / t).toFixed(0).padStart(3)}%`;
}
function cell(a) {
  if (!a) return '   -    ';
  return `${pct(a.pass, a.total)} ${String(`(${a.pass}/${a.total})`).padEnd(8)}`;
}

const overall = { gemini: { pass: 0, total: 0 }, 'gpt-4o-mini': { pass: 0, total: 0 } };
const lat = { gemini: { latency: 0, n: 0 }, 'gpt-4o-mini': { latency: 0, n: 0 } };

const headSurface = 'Surface'.padEnd(18);
console.log('');
console.log('╔══════════════════════════════════════════════════════════════════╗');
console.log('║              PAYVERGE MODEL BENCHMARK — SCOREBOARD                  ║');
console.log('╚══════════════════════════════════════════════════════════════════╝');
console.log(`${headSurface}  ${'gemini'.padEnd(16)}  ${'gpt-4o-mini'.padEnd(16)}  winner`);
console.log('─'.repeat(70));

let anyMissing = false;
for (const s of SURFACES) {
  const agg = loadSurface(s);
  if (!agg) {
    anyMissing = true;
    console.log(`${s.padEnd(18)}  ${'(no results)'.padEnd(36)}`);
    continue;
  }
  const g = agg.gemini;
  const o = agg['gpt-4o-mini'];
  for (const [fam, a] of Object.entries(agg)) {
    if (overall[fam]) {
      overall[fam].pass += a.pass;
      overall[fam].total += a.total;
      lat[fam].latency += a.latency;
      lat[fam].n += a.latencyN;
    }
  }
  const gr = g ? g.pass / g.total : 0;
  const or = o ? o.pass / o.total : 0;
  const win = !g || !o ? '' : gr > or ? 'gemini' : or > gr ? 'gpt-4o-mini' : 'tie';
  console.log(`${s.padEnd(18)}  ${cell(g).padEnd(16)}  ${cell(o).padEnd(16)}  ${win}`);
}

console.log('─'.repeat(70));
const G = overall.gemini;
const O = overall['gpt-4o-mini'];
console.log(`${'TOTAL'.padEnd(18)}  ${cell(G).padEnd(16)}  ${cell(O).padEnd(16)}`);
const avg = (f) => (lat[f].n ? `${(lat[f].latency / lat[f].n / 1000).toFixed(1)}s` : '-');
console.log(`${'avg latency'.padEnd(18)}  ${avg('gemini').padEnd(16)}  ${avg('gpt-4o-mini').padEnd(16)}`);
console.log('');
if (anyMissing) {
  console.log('Some surfaces have no results yet — run ./evals/promptfoo/runner.sh first.');
}
