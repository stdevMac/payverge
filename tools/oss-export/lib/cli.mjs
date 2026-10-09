#!/usr/bin/env node
// Node half of tools/oss-export/export.sh. Subcommands:
//   validate --forbidden F [--drop F] [--scrub F] [--secrets-allow F]
//   drop     --root DIR [--drop F] --report FILE
//   scrub    --root DIR [--scrub F] --report FILE
//   gates    --root DIR --forbidden F [--copyright-holder NAME] [...] --report FILE
//   stats    --root DIR --report FILE
// Exit codes: 0 ok, 1 gate failure, 2 configuration or usage error.

import fs from "node:fs";
import path from "node:path";
import { parseArgs } from "node:util";

import {
  ConfigError,
  parseDropList,
  parseForbidden,
  parseScrubRules,
  parseSecretsAllow,
  readConfigFile,
} from "./config.mjs";
import { DEFAULT_SIZE_ALLOW, runGates } from "./gates.mjs";
import { applyDrop, applyScrub, treeStats } from "./steps.mjs";
import { formatBytes } from "./tree.mjs";

const MAX_LISTED_HITS = 60;

const optionSpec = {
  root: { type: "string" },
  report: { type: "string" },
  forbidden: { type: "string" },
  drop: { type: "string" },
  scrub: { type: "string" },
  "secrets-allow": { type: "string" },
  "tmp-dir": { type: "string" },
  "max-file-mb": { type: "string", default: "10" },
  "size-allow": { type: "string", multiple: true },
  "allow-env-file": { type: "string", multiple: true },
  "allow-opaque": { type: "string", multiple: true },
  "allow-missing-scanner": { type: "boolean", default: false },
  "trufflehog-verify": { type: "boolean", default: false },
  author: { type: "string" },
  message: { type: "string" },
  "copyright-holder": { type: "string" },
  candidates: { type: "string" },
};

function loadConfigs(values) {
  return {
    forbidden: values.forbidden ? readConfigFile(values.forbidden, parseForbidden) : null,
    drop: values.drop ? readConfigFile(values.drop, parseDropList) : [],
    scrub: values.scrub ? readConfigFile(values.scrub, parseScrubRules) : [],
    secretsAllow: values["secrets-allow"] ? readConfigFile(values["secrets-allow"], parseSecretsAllow) : new Set(),
  };
}

function requireOption(values, name) {
  if (!values[name]) throw new UsageError(`--${name} is required`);
  return values[name];
}

class UsageError extends Error {}

function writeReport(file, data) {
  if (!file) return;
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, `${JSON.stringify(data, null, 2)}\n`);
}

function cmdValidate(values) {
  const configs = loadConfigs(values);
  if (!configs.forbidden) throw new UsageError("--forbidden is required");
  console.log(
    `config ok: forbidden=${configs.forbidden.length} rules, drop=${configs.drop.length} entries, scrub=${configs.scrub.length} rules, secrets-allow=${configs.secretsAllow.size} fingerprints`,
  );
  return 0;
}

function cmdDrop(values) {
  const root = requireOption(values, "root");
  const { drop } = loadConfigs(values);
  const result = applyDrop(root, drop);
  for (const entry of result.entries) {
    if (entry.files === 0) continue;
    const origin = entry.builtin ? "builtin" : `drop.txt:${entry.lineNo}`;
    console.log(`  dropped ${String(entry.files).padStart(5)} files ${formatBytes(entry.bytes).padStart(9)}  ${entry.pattern}  (${origin})`);
  }
  console.log(`drop: ${result.droppedFiles} files, ${formatBytes(result.droppedBytes)} removed`);
  if (result.unmatched.length > 0) {
    console.log(`drop: ${result.unmatched.length} entries matched nothing (already pruned upstream?):`);
    for (const pattern of result.unmatched) console.log(`  - ${pattern}`);
  }
  writeReport(values.report, result);
  return 0;
}

function cmdScrub(values) {
  const root = requireOption(values, "root");
  const { scrub } = loadConfigs(values);
  const result = applyScrub(root, scrub);
  for (const rule of result.rules) {
    console.log(`  scrub ${rule.name}: ${rule.replacements} replacements in ${rule.files.length} files`);
  }
  console.log(`scrub: ${scrub.length} rules, ${result.filesChanged} files changed`);
  if (result.skippedNonUtf8.length > 0) {
    console.log(`scrub: skipped ${result.skippedNonUtf8.length} non-UTF-8 text files`);
  }
  writeReport(values.report, result);
  return 0;
}

function printGate(gateResult) {
  const label = `(${gateResult.letter}) ${gateResult.id}`.padEnd(24);
  const status = gateResult.status.toUpperCase().padEnd(5);
  const extra = [];
  if (gateResult.hits.length) extra.push(`${gateResult.hits.length} hits`);
  if (gateResult.allowlisted) extra.push(`${gateResult.allowlisted} allowlisted`);
  if (gateResult.error) extra.push(gateResult.error);
  console.log(`  ${label} ${status} ${gateResult.title}${extra.length ? ` — ${extra.join(", ")}` : ""}`);
}

function printGateDetails(gateResult) {
  if (gateResult.perRule && Object.keys(gateResult.perRule).length) {
    for (const [name, counts] of Object.entries(gateResult.perRule)) {
      console.log(`      rule ${name}: ${counts.lines} lines in ${counts.files} files`);
    }
  }
  for (const hit of gateResult.hits.slice(0, MAX_LISTED_HITS)) {
    const where = hit.line !== undefined && hit.line !== null ? `${hit.file}:${hit.line}` : hit.file;
    console.log(`      ${where}  [${hit.rule}]${hit.detail ? ` ${hit.detail}` : ""}`);
  }
  if (gateResult.hits.length > MAX_LISTED_HITS) {
    console.log(`      … ${gateResult.hits.length - MAX_LISTED_HITS} more in the report`);
  }
  for (const note of gateResult.notes) console.log(`      note: ${note}`);
}

async function cmdGates(values) {
  const root = requireOption(values, "root");
  const tmpDir = requireOption(values, "tmp-dir");
  const configs = loadConfigs(values);
  if (!configs.forbidden) throw new UsageError("--forbidden is required: the export never runs without the forbidden-strings gate");
  const maxMb = Number(values["max-file-mb"]);
  if (!Number.isFinite(maxMb) || maxMb <= 0) throw new UsageError("--max-file-mb must be a positive number");
  const result = await runGates({
    root,
    tmpDir,
    forbiddenRules: configs.forbidden,
    dropEntries: configs.drop,
    secretsAllow: configs.secretsAllow,
    maxBytes: Math.floor(maxMb * 1024 * 1024),
    sizeAllow: values["size-allow"] && values["size-allow"].length ? values["size-allow"] : DEFAULT_SIZE_ALLOW,
    envAllow: values["allow-env-file"] || [],
    opaqueAllow: values["allow-opaque"] || [],
    allowMissingScanner: values["allow-missing-scanner"],
    trufflehogVerify: values["trufflehog-verify"],
    author: values.author || null,
    message: values.message || null,
    copyrightHolder: values["copyright-holder"] || null,
  });
  console.log(`gates over ${result.entries} paths:`);
  for (const gateResult of result.gates) printGate(gateResult);
  for (const gateResult of result.gates) {
    if (gateResult.status === "pass" && gateResult.notes.length === 0) continue;
    console.log(`  details (${gateResult.letter}) ${gateResult.id}:`);
    printGateDetails(gateResult);
  }
  if (values.candidates && result.candidates.length) {
    fs.mkdirSync(path.dirname(values.candidates), { recursive: true });
    fs.writeFileSync(
      values.candidates,
      [
        "# Unreviewed secret-scanner findings, formatted for secrets-allow.txt.",
        "# Add a line ONLY after confirming the value is a synthetic fixture.",
        ...[...new Set(result.candidates)].sort(),
        "",
      ].join("\n"),
    );
  }
  writeReport(values.report, { ok: result.ok, gates: result.gates });
  const failed = result.gates.filter((g) => g.status === "fail").map((g) => g.id);
  const warned = result.gates.filter((g) => g.status === "warn").map((g) => g.id);
  if (warned.length) console.log(`gates: WARN ${warned.join(", ")}`);
  if (failed.length) {
    console.log(`gates: FAIL ${failed.join(", ")}`);
    return 1;
  }
  console.log("gates: all fatal gates passed");
  return 0;
}

function cmdStats(values) {
  const root = requireOption(values, "root");
  const stats = treeStats(root);
  console.log(`tree: ${stats.files} files, ${stats.symlinks} symlinks, ${formatBytes(stats.bytes)}`);
  console.log("  by top-level entry (largest first):");
  for (const bucket of stats.topLevel.slice(0, 15)) {
    console.log(`    ${formatBytes(bucket.bytes).padStart(9)}  ${String(bucket.files).padStart(5)} files  ${bucket.path}`);
  }
  if (stats.topLevel.length > 15) console.log(`    … ${stats.topLevel.length - 15} more`);
  console.log("  largest files:");
  for (const file of stats.largest) console.log(`    ${formatBytes(file.bytes).padStart(9)}  ${file.path}`);
  writeReport(values.report, stats);
  return 0;
}

const commands = { validate: cmdValidate, drop: cmdDrop, scrub: cmdScrub, gates: cmdGates, stats: cmdStats };

async function main(argv) {
  const [command, ...rest] = argv;
  const handler = commands[command];
  if (!handler) {
    console.error(`usage: cli.mjs <${Object.keys(commands).join("|")}> [options]`);
    return 2;
  }
  const { values } = parseArgs({ args: rest, options: optionSpec, allowPositionals: false });
  return handler(values);
}

main(process.argv.slice(2)).then(
  (code) => {
    process.exitCode = code;
  },
  (error) => {
    if (error instanceof ConfigError || error instanceof UsageError || error?.code === "ERR_PARSE_ARGS_UNKNOWN_OPTION") {
      console.error(`oss-export: ${error.message}`);
      process.exitCode = 2;
      return;
    }
    if (error?.code === "ENOENT") {
      console.error(`oss-export: ${error.message}`);
      process.exitCode = 2;
      return;
    }
    console.error(error?.stack || String(error));
    process.exitCode = 3;
  },
);
