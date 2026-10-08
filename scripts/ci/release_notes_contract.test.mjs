#!/usr/bin/env node
// Release-notes truth contract.
//
// docs/CHANGELOG.md and the community files under .github/ are what a
// self-hoster reads before deciding to run Payverge, so they must not promise
// more than the code on the same commit does. Each claim below pairs a phrase
// with the code fact that makes it true. The phrase may appear only while the
// fact holds. Text inside HTML comments is ignored, so draft wording for a
// future release can wait there.
//
//   node --test scripts/ci/release_notes_contract.test.mjs
//   RELEASE_VERSION=1.0.0 node --test scripts/ci/release_notes_contract.test.mjs
//
// Phrase matching catches over-claims. It cannot prove that a note is
// accurate. When a capability changes how it is switched on (for example, a
// new storage driver or email provider becomes the default), update the
// matching fact function in the same PR. See docs/governance/RELEASING.md,
// "Release-notes check".

import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const read = (relative) => readFileSync(path.join(ROOT, relative), "utf8");
const RELEASE_VERSION = (process.env.RELEASE_VERSION ?? "").trim().replace(/^v/, "");

const CHANGELOG = "docs/CHANGELOG.md";
// Files a self-hoster reads as statements about the current release.
// GOVERNANCE.md (principles) and RELEASING.md (the target release flow) state
// goals rather than current capabilities, so they are only link-checked.
const CLAIM_FILES = [
  CHANGELOG,
  ".github/CONTRIBUTING.md",
  ".github/SECURITY.md",
  ".github/SUPPORT.md",
  ".github/ISSUE_TEMPLATE/bug_report.yml",
];
const LINK_FILES = [
  ...CLAIM_FILES.filter((file) => file.endsWith(".md")),
  ".github/CODE_OF_CONDUCT.md",
  ".github/PULL_REQUEST_TEMPLATE.md",
  "docs/governance/GOVERNANCE.md",
  "docs/governance/RELEASING.md",
];

const stripHtmlComments = (text) => text.replace(/<!--[\s\S]*?-->/g, "");

// The changelog preamble plus its newest version section. Older sections
// describe older releases and are not re-checked against today's code.
function currentChangelog() {
  const text = stripHtmlComments(read(CHANGELOG));
  const headings = [...text.matchAll(/^## \[/gm)];
  return headings.length > 1 ? text.slice(0, headings[1].index) : text;
}

const claimText = (file) => (file === CHANGELOG ? currentChangelog() : stripHtmlComments(read(file)));

// --- code facts ------------------------------------------------------------

const PREFLIGHT_SOURCE = "backend/internal/config/production_preflight.go";
const STORAGE_CONFIG_SOURCE = "backend/internal/s3/init.go";
const STORAGE_BOOT_SOURCE = "backend/cmd/app/oss_storage.go";
const EMAIL_CONFIG_SOURCE = "backend/internal/config/email.go";

// The body of one arm of a Go switch: the lines after `case <label>:` up to
// the next line indented no deeper than the case (the next arm, `default:` or
// the closing brace). Returns null when the arm is missing.
function switchArm(source, label) {
  const lines = source.split("\n");
  const at = lines.findIndex((line) => line.trim().startsWith(`case ${label}:`));
  if (at === -1) return null;
  const indent = lines[at].match(/^\s*/)[0].length;
  const body = [];
  for (const line of lines.slice(at + 1)) {
    if (line.trim() && line.match(/^\s*/)[0].length <= indent) break;
    body.push(line);
  }
  return body.join("\n");
}

// A preflight arm that only warns (r.addWarning) and never fails (r.add).
const preflightArmOnlyWarns = (label) => {
  const arm = switchArm(read(PREFLIGHT_SOURCE), label);
  return arm !== null && !/\br\.add\(/.test(arm);
};

// Object storage is optional only while STORAGE_DRIVER defaults to the local
// volume, the production preflight accepts the local driver without S3
// settings, and startup aborts on a storage failure only for the s3 driver.
// The "s3.public.missing" error still exists, but only in the s3 arm.
function objectStorageRequired() {
  if (read("backend/cmd/app/main.go").includes('Fatalf("Failed to initialize S3')) return true;
  const defaultsToLocal = /if driver == "" \{\s*driver = DriverLocal\s*\}/.test(read(STORAGE_CONFIG_SOURCE));
  const bootFatalOnlyForS3 = /if cfg\.Driver == s3\.DriverS3 \{\s*logger\.Logger\.Fatalf\("Failed to initialize S3/.test(
    read(STORAGE_BOOT_SOURCE),
  );
  return !(defaultsToLocal && preflightArmOnlyWarns('"", "local"') && bootFatalOnlyForS3);
}

// An email API key is required in production unless EMAIL_PROVIDER falls back
// to the log provider when nothing is configured and the preflight only warns
// about it. The "email.api_key.missing" error still exists, but only for
// providers that need a key (resend, postmark).
function emailKeyRequiredInProduction() {
  const defaultsToLog = /func EmailProvider\(\) string \{[\s\S]*?\n\treturn EmailProviderLog\n\}/.test(
    read(EMAIL_CONFIG_SOURCE),
  );
  return !(defaultsToLog && preflightArmOnlyWarns('"", "log"'));
}

// RPC_URL falls back to the public Base RPC with a preflight warning, so no
// RPC provider account is needed.
const rpcUrlOptional = () => /r\.addWarning\("rpc_url\.default"/.test(read(PREFLIGHT_SOURCE));

// The manager-PIN step-up applies only to staff sessions while owner sessions
// skip it, so "every refund needs a PIN" is false.
const MANAGER_PIN_SOURCE = "backend/internal/server/manager_pin.go";
const ownersSkipManagerPin = () => /if ts != "staff"/.test(read(MANAGER_PIN_SOURCE));

// The Idempotency middleware passes a request without the header straight
// through, so "every refund needs an idempotency key" is false.
const IDEMPOTENCY_SOURCE = "backend/internal/middleware/idempotency.go";
const idempotencyKeyOptional = () => /if key == "" \{\s*c\.Next\(\)/.test(read(IDEMPOTENCY_SOURCE));

const INSTALLER_PATHS = ["deploy/install.sh", "scripts/install.sh", "install.sh"];
const installerExists = () => INSTALLER_PATHS.some((relative) => existsSync(path.join(ROOT, relative)));

// --- claims ----------------------------------------------------------------

const CLAIMS = [
  {
    claim: "zero third-party accounts",
    pattern: /zero third-party accounts/i,
    holds: () =>
      !objectStorageRequired() && !emailKeyRequiredInProduction() && rpcUrlOptional(),
    fact: "object storage optional, no email API key required in production, and RPC_URL optional",
  },
  {
    claim: "object storage is optional",
    pattern: /\b(?:object storage|S3)\b[^.]*\boptional\b|\boptional\b[^.]*\b(?:object storage|S3)\b/i,
    holds: () => !objectStorageRequired(),
    fact: `STORAGE_DRIVER defaults to local (${STORAGE_CONFIG_SOURCE}), the local preflight arm only warns, and ${STORAGE_BOOT_SOURCE} aborts only for the s3 driver`,
  },
  {
    claim: "email delivery is optional",
    pattern: /\be-?mail\b[^.]*\boptional\b|\boptional\b[^.]*\be-?mail\b/i,
    holds: () => !emailKeyRequiredInProduction(),
    fact: `EmailProvider() falls back to log (${EMAIL_CONFIG_SOURCE}) and the log preflight arm only warns`,
  },
  {
    // A scoped claim ("staff refunds require a manager PIN") is fine.
    claim: "every refund requires a PIN",
    pattern: /(?<!\b(?:staff|delegated)\s+)\brefunds?\s+(?:always\s+)?requires?\b[^.]*\bPIN\b/i,
    holds: () => !ownersSkipManagerPin(),
    fact: `RequireManagerPIN in ${MANAGER_PIN_SOURCE} applies to owner sessions too`,
  },
  {
    claim: "every refund requires an idempotency key",
    pattern: /\brefunds?\s+(?:always\s+)?requires?\b[^.]*\bidempotency[\s-]+key\b/i,
    holds: () => !idempotencyKeyOptional(),
    fact: `the Idempotency middleware in ${IDEMPOTENCY_SOURCE} rejects a request without the header`,
  },
  {
    claim: "an installer ships",
    pattern: /\binstall\.sh\b|\binstaller\b/i,
    holds: installerExists,
    fact: `one of ${INSTALLER_PATHS.join(", ")} exists`,
  },
];

test("the code facts this contract depends on can be read", () => {
  for (const relative of [
    "backend/cmd/app/main.go",
    PREFLIGHT_SOURCE,
    STORAGE_CONFIG_SOURCE,
    STORAGE_BOOT_SOURCE,
    EMAIL_CONFIG_SOURCE,
    MANAGER_PIN_SOURCE,
    IDEMPOTENCY_SOURCE,
  ]) {
    assert.ok(existsSync(path.join(ROOT, relative)), `${relative} moved; update this contract`);
  }
  for (const label of ['"", "local"', '"", "log"']) {
    assert.notEqual(
      switchArm(read(PREFLIGHT_SOURCE), label),
      null,
      `case ${label}: not found in ${PREFLIGHT_SOURCE}; update this contract`,
    );
  }
});

test("release notes and community files claim only what the code supports", () => {
  const overClaims = [];
  for (const file of CLAIM_FILES) {
    const text = claimText(file);
    for (const { claim, pattern, holds, fact } of CLAIMS) {
      const match = text.match(pattern);
      if (match && !holds()) {
        overClaims.push(`${file}: claims "${claim}" ("${match[0].trim()}") but the code lacks: ${fact}`);
      }
    }
  }
  assert.deepEqual(overClaims, []);
});

// --- release readiness (only with RELEASE_VERSION) -------------------------

const releaseOnly = RELEASE_VERSION ? {} : { skip: "set RELEASE_VERSION=X.Y.Z to run release-readiness checks" };

test("the changelog has a dated section for the release", releaseOnly, () => {
  const text = stripHtmlComments(read(CHANGELOG));
  const escaped = RELEASE_VERSION.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const heading = new RegExp(`^## \\[${escaped}\\][^\\n]*$`, "m").exec(text);
  assert.ok(heading, `${CHANGELOG} has no "## [${RELEASE_VERSION}]" section`);
  const rest = text.slice(heading.index + heading[0].length);
  const next = rest.search(/^## \[/m);
  const section = heading[0] + (next === -1 ? rest : rest.slice(0, next));
  assert.match(section, /\b\d{4}-\d{2}-\d{2}\b/, `the ${RELEASE_VERSION} section needs a release date (YYYY-MM-DD)`);
  assert.ok(!/Release date: set when/i.test(section), "the release-date placeholder is still present");
});

test("no integrator notes remain in the changelog", releaseOnly, () => {
  assert.ok(!/Integrator:/.test(read(CHANGELOG)), `${CHANGELOG} still carries an "Integrator:" note`);
});

test("repository paths named in the newest changelog section exist", releaseOnly, () => {
  const pathLike = /`([A-Za-z0-9_.-]+\.(?:md|sh|ya?ml|json)|[A-Za-z0-9_.-]+\/[A-Za-z0-9_./-]+)`/g;
  const missing = [...currentChangelog().matchAll(pathLike)]
    .map(([, relative]) => relative)
    .filter((relative) => !existsSync(path.join(ROOT, relative)));
  assert.deepEqual(missing, [], `${CHANGELOG} names paths that are not in the tree`);
});

test("relative links in community and governance docs resolve", releaseOnly, () => {
  const broken = [];
  for (const file of LINK_FILES) {
    const text = stripHtmlComments(read(file));
    for (const [, target] of text.matchAll(/\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g)) {
      if (/^(?:[a-z][a-z0-9+.-]*:|#)/i.test(target)) continue;
      const relative = decodeURI(target.split("#")[0]);
      if (!relative) continue;
      if (!existsSync(path.resolve(path.dirname(path.join(ROOT, file)), relative))) {
        broken.push(`${file} -> ${target}`);
      }
    }
  }
  assert.deepEqual(broken, []);
});
