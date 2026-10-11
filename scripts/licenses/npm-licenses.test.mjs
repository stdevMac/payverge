import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  checkNotices,
  checkPackages,
  classifyId,
  collectPackages,
  evaluate,
  mentionsName,
  normaliseLicense,
  parseOverrides,
  renderReport,
} from "./npm-licenses.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, "..", "..");

function pkg(name, license, extra = {}) {
  return { key: `node_modules/${name}`, name, version: "1.0.0", license, source: license ? "lockfile" : null, ...extra };
}

test("classifies identifiers into permissive, review and forbidden", () => {
  for (const id of ["MIT", "Apache-2.0", "BSD-3-Clause", "ISC", "0BSD", "BlueOak-1.0.0", "CC0-1.0"]) {
    assert.equal(classifyId(id), "permissive", id);
  }
  for (const id of ["LGPL-3.0-or-later", "LGPL-2.1", "MPL-2.0", "CC-BY-4.0", "EPL-2.0", "FSL-1.1-MIT", "LicenseRef-see-file", "Totally-Made-Up"]) {
    assert.equal(classifyId(id), "review", id);
  }
  for (const id of ["GPL-3.0", "GPL-2.0-or-later", "AGPL-3.0-only", "SSPL-1.0", "BUSL-1.1", "CC-BY-NC-4.0", "CC-BY-NC-SA-4.0", "CC-BY-ND-4.0", "UNLICENSED"]) {
    assert.equal(classifyId(id), "forbidden", id);
  }
});

test("OR takes the best branch and AND the worst", () => {
  assert.equal(evaluate("(MPL-2.0 OR Apache-2.0)").category, "permissive");
  assert.deepEqual(evaluate("(MPL-2.0 OR Apache-2.0)").ids, ["Apache-2.0"]);
  assert.equal(evaluate("GPL-3.0 OR MIT").category, "permissive");
  assert.equal(evaluate("Apache-2.0 AND LGPL-3.0-or-later AND MIT").category, "review");
  assert.equal(evaluate("MIT AND GPL-3.0-only").category, "forbidden");
  assert.equal(evaluate("(MIT OR CC0-1.0)").category, "permissive");
  assert.equal(evaluate("mit or gpl-3.0").category, "permissive", "operators are case-insensitive");
  assert.equal(evaluate("GPL-2.0-only WITH Classpath-exception-2.0").category, "review");
  assert.equal(evaluate("GPL-2.0-only WITH Some-Other-Exception").category, "forbidden");
  assert.equal(evaluate("(MIT").category, "review", "unparseable expressions need review");
});

test("normalises legacy package.json licence shapes", () => {
  assert.equal(normaliseLicense("MIT OR SEE LICENSE IN FEEL-FREE.md"), "MIT OR LicenseRef-see-file");
  assert.equal(evaluate(normaliseLicense("MIT OR SEE LICENSE IN FEEL-FREE.md")).category, "permissive");
  assert.equal(normaliseLicense([{ type: "MIT", url: "x" }]), "MIT");
  assert.equal(normaliseLicense([{ type: "MIT" }, { type: "Apache-2.0" }]), "(MIT OR Apache-2.0)");
  assert.equal(normaliseLicense({ type: "ISC" }), "ISC");
  assert.equal(normaliseLicense("  "), null);
  assert.equal(normaliseLicense(undefined), null);
});

test("collects only production entries from a v3 lockfile", () => {
  const lock = {
    lockfileVersion: 3,
    packages: {
      "": { name: "app" },
      "node_modules/a": { version: "1.0.0", license: "MIT" },
      "node_modules/b": { version: "1.0.0", license: "GPL-3.0", dev: true },
      "node_modules/c": { version: "1.0.0", license: "GPL-3.0", devOptional: true },
      "node_modules/d/node_modules/e": { version: "2.0.0", license: "ISC", optional: true },
      "node_modules/f": { version: "1.0.0", link: true },
      "node_modules/alias": { name: "real-name", version: "3.0.0", license: "MIT" },
    },
  };
  const names = collectPackages(lock).map((p) => `${p.name}@${p.version}`);
  assert.deepEqual(names, ["a@1.0.0", "e@2.0.0", "real-name@3.0.0"]);
  assert.throws(() => collectPackages({ lockfileVersion: 1, dependencies: {} }), /lockfileVersion/);
});

test("fails closed on missing, forbidden and unreviewed licences", () => {
  const overrides = parseOverrides("");
  const { errors } = checkPackages(
    [pkg("ok", "MIT"), pkg("nolicense", null), pkg("copyleft", "AGPL-3.0-only"), pkg("weak", "MPL-2.0")],
    overrides,
  );
  assert.equal(errors.length, 3);
  assert.match(errors.join("\n"), /nolicense@1\.0\.0: no licence declared/);
  assert.match(errors.join("\n"), /copyleft@1\.0\.0: "AGPL-3\.0-only" is not allowed/);
  assert.match(errors.join("\n"), /weak@1\.0\.0: "MPL-2\.0" needs review/);
});

test("allow rows accept review licences only for the exact expression", () => {
  const overrides = parseOverrides(
    [
      "allow\tweak\tMPL-2.0\tfile-level copyleft, unmodified",
      "allow\tgpl-thing\tGPL-3.0\tthis must never work",
    ].join("\n"),
  );
  const ok = checkPackages([pkg("weak", "MPL-2.0")], overrides);
  assert.deepEqual(ok.errors, []);
  assert.equal(ok.results[0].allow.reason, "file-level copyleft, unmodified");

  const changed = checkPackages([pkg("weak", "EPL-2.0")], parseOverrides("allow\tweak\tMPL-2.0\tx"));
  assert.equal(changed.errors.length, 1, "a changed upstream licence fails again");

  const forbidden = checkPackages([pkg("gpl-thing", "GPL-3.0")], overrides);
  assert.equal(forbidden.errors.length, 1, "allow rows cannot accept forbidden licences");
});

test("license rows fill gaps only, first match wins, and stale rows are reported", () => {
  const overrides = parseOverrides(
    [
      "license\t@img/sharp-libvips-*\tLGPL-3.0-or-later\tlibvips",
      "license\t@img/sharp-*\tApache-2.0\tbinding",
      "license\tunused-*\tMIT\tnothing matches",
      "allow\t@img/sharp-libvips-*\tLGPL-3.0-or-later\tdynamic linking",
    ].join("\n"),
  );
  const { errors, results, stale } = checkPackages(
    [pkg("@img/sharp-libvips-linux-x64", null), pkg("@img/sharp-linux-x64", null), pkg("@img/sharp-declared", "GPL-3.0")],
    overrides,
  );
  assert.equal(results[0].license, "LGPL-3.0-or-later");
  assert.equal(results[0].source, "override");
  assert.equal(results[1].license, "Apache-2.0");
  assert.equal(results[2].license, "GPL-3.0", "a declared licence is never replaced");
  assert.deepEqual(errors, ['@img/sharp-declared@1.0.0: "GPL-3.0" is not allowed in production dependencies (GPL-3.0)']);
  assert.deepEqual(stale, ["license\tunused-*\tMIT"]);
});

test("rejects malformed override rows", () => {
  assert.throws(() => parseOverrides("allow\tx\tMIT"), /line 1/);
  assert.throws(() => parseOverrides("permit\tx\tMIT\twhy"), /line 1/);
  assert.deepEqual(parseOverrides("# comment\n\n"), []);
});

test("report dedupes by name@version and lists reviewed licences", () => {
  const overrides = parseOverrides("allow\tweak\tMPL-2.0\tunmodified");
  const { results } = checkPackages(
    [pkg("b", "MIT"), pkg("a", "ISC"), pkg("b", "MIT", { key: "node_modules/x/node_modules/b" }), pkg("weak", "MPL-2.0")],
    overrides,
  );
  const { table, reviewed, count } = renderReport(results);
  assert.equal(count, 3);
  const body = table.split("\n").slice(2);
  assert.match(body[0], /^\| \[a\]/);
  assert.match(body[1], /^\| \[b\]/);
  assert.match(reviewed, /\| weak@1\.0\.0 \| MPL-2\.0 \| unmodified \|/);
});

test("every dependency NOTICE file must be named in the root NOTICE", () => {
  const packages = [
    pkg("plain", "MIT"),
    pkg("noticed", "Apache-2.0", { notice: "NOTICE" }),
    pkg("covered", "Apache-2.0", { notice: "NOTICE.txt" }),
  ];
  assert.deepEqual(checkNotices(packages, "Payverge\n  covered\n  Copyright 2020 Someone"), [
    "noticed: ships node_modules/noticed/NOTICE; reproduce its notice in the root NOTICE file",
  ]);
});

test("NOTICE names match whole package names, not substrings of longer ones", () => {
  const notice = [
    "sharp-libvips - https://github.com/lovell/sharp-libvips",
    "  Orchestrion-JS (@apm-js-collab/code-transformer)",
    "  yaml.v3 (gopkg.in/yaml.v3)",
    "  import-in-the-middle",
    "  Credited at the end of a sentence: left-pad.",
    "  Source: https://github.com/example/linked",
  ].join("\n");
  for (const name of ["sharp-libvips", "@apm-js-collab/code-transformer", "gopkg.in/yaml.v3", "import-in-the-middle", "left-pad", "github.com/example/linked"]) {
    assert.equal(mentionsName(notice, name), true, name);
  }
  for (const name of ["sharp", "lovell", "code-transformer", "@apm-js-collab/code", "yaml", "gopkg.in/yaml", "import-in-the", "middle", "pad", "linked", "github.com/example/link"]) {
    assert.equal(mentionsName(notice, name), false, name);
  }
  assert.equal(mentionsName("a+b (c)", "a+b"), true, "regex metacharacters are literal");
  assert.equal(mentionsName("aab", "a.b"), false, "a dot in a name is not a wildcard");

  const packages = [pkg("sharp", "Apache-2.0", { notice: "NOTICE" }), pkg("sharp-libvips", "LGPL-3.0-or-later", { notice: "NOTICE" })];
  assert.deepEqual(checkNotices(packages, notice), [
    "sharp: ships node_modules/sharp/NOTICE; reproduce its notice in the root NOTICE file",
  ]);
});

test("the real frontend production tree passes the policy", { skip: !existsSync(path.join(ROOT, "frontend", "node_modules")) }, () => {
  const frontendDir = path.join(ROOT, "frontend");
  const lock = JSON.parse(readFileSync(path.join(frontendDir, "package-lock.json"), "utf8"));
  const overrides = parseOverrides(readFileSync(path.join(HERE, "npm-overrides.tsv"), "utf8"));
  const packages = collectPackages(lock, { frontendDir });
  const { errors } = checkPackages(packages, overrides);
  assert.deepEqual(errors, []);
  assert.deepEqual(checkNotices(packages, readFileSync(path.join(ROOT, "NOTICE"), "utf8")), []);
});
