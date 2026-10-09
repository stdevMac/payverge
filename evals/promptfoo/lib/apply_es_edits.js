#!/usr/bin/env node
/**
 * Applies the structured Spanish-mirror edits (from the prompt-hardening-es-mirror
 * workflow) to the production prompt files, with strict safety checks:
 *  - old_string must occur EXACTLY once in the file (else skip + report)
 *  - reports every applied / skipped edit so mismatches can be fixed by hand
 *
 * Usage: node lib/apply_es_edits.js <edits.json>
 *   edits.json = { files: [ { file, edits: [ { old_string, new_string, note } ] } ] }
 */
const fs = require('fs');

const EDITS_FILE = process.argv[2];
if (!EDITS_FILE) {
  console.error('usage: node lib/apply_es_edits.js <edits.json>');
  process.exit(1);
}
const raw = JSON.parse(fs.readFileSync(EDITS_FILE, 'utf8'));
const files = raw.files || raw;

let applied = 0,
  skipped = 0;
for (const f of files) {
  if (!f || !f.file) continue;
  let content;
  try {
    content = fs.readFileSync(f.file, 'utf8');
  } catch (e) {
    console.log(`MISSING FILE: ${f.file}`);
    skipped += (f.edits || []).length;
    continue;
  }
  console.log(`\n### ${f.file}`);
  for (const e of f.edits || []) {
    const idx = content.indexOf(e.old_string);
    if (idx === -1) {
      console.log(`  SKIP (no match): ${e.note} | "${e.old_string.slice(0, 50).replace(/\n/g, '⏎')}..."`);
      skipped++;
      continue;
    }
    const last = content.lastIndexOf(e.old_string);
    if (idx !== last) {
      console.log(`  SKIP (ambiguous, ${content.split(e.old_string).length - 1}x): ${e.note}`);
      skipped++;
      continue;
    }
    content = content.slice(0, idx) + e.new_string + content.slice(idx + e.old_string.length);
    console.log(`  OK: ${e.note}`);
    applied++;
  }
  fs.writeFileSync(f.file, content);
}
console.log(`\nApplied ${applied} edits, skipped ${skipped}.`);
if (skipped) console.log('Fix skipped edits by hand (old_string did not match uniquely).');
