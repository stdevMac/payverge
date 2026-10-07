// Tests for the shell helpers in lib.sh: node --test scripts/licenses/lib.test.mjs
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const LIB = path.join(HERE, "lib.sh");
const work = mkdtempSync(path.join(tmpdir(), "payverge-licenses-lib-"));
after(() => rmSync(work, { recursive: true, force: true }));

// Sources lib.sh and runs `script` with the given positional arguments.
function sh(script, ...args) {
  return spawnSync("bash", ["-c", `source "$0"; ${script}`, LIB, ...args], { encoding: "utf8" });
}

test("lic_notice_names matches whole module paths and package names only", () => {
  const notice = path.join(work, "NOTICE");
  writeFileSync(
    notice,
    [
      "  AWS SDK for Go (github.com/aws/aws-sdk-go-v2)",
      "  yaml.v3 (gopkg.in/yaml.v3)",
      "  procfs (github.com/prometheus/procfs)",
      "  Orchestrion-JS (@apm-js-collab/code-transformer)",
      "go-ethereum - https://github.com/ethereum/go-ethereum",
      "  Credited at the end of a sentence: github.com/example/tail.",
      "",
    ].join("\n"),
  );
  const names = (name) => sh('lic_notice_names "$1" "$2"', notice, name).status === 0;

  for (const name of [
    "github.com/aws/aws-sdk-go-v2",
    "gopkg.in/yaml.v3",
    "github.com/prometheus/procfs",
    "@apm-js-collab/code-transformer",
    "github.com/ethereum/go-ethereum",
    "github.com/example/tail",
  ]) {
    assert.equal(names(name), true, name);
  }
  for (const name of [
    "github.com/aws/aws-sdk-go", // prefix of a longer path
    "github.com/aws/aws-sdk-go-v2/service/s3", // submodule of a credited module
    "gopkg.in/yaml", // gopkg.in/yaml.v3 contains it
    "github.com/prometheus", // parent of a credited module
    "code-transformer",
    "github.com/example/ta",
    "github.com/ethereum/go", // the URL names go-ethereum, not this
    "gopkg.in/yamlXv3", // the dot is not a wildcard
  ]) {
    assert.equal(names(name), false, name);
  }
});

test("lic_go_overrides prints module, detected and license, and fails on a short row", () => {
  const good = path.join(work, "good.tsv");
  writeFileSync(good, "# comment\n\nexample.com/a\tGPL-3.0\tLGPL-3.0-or-later\twhy\n");
  const ok = sh('LIC_GO_OVERRIDES="$1"; lic_go_overrides', good);
  assert.equal(ok.status, 0, ok.stderr);
  assert.equal(ok.stdout, "example.com/a\tGPL-3.0\tLGPL-3.0-or-later\n");

  const bad = path.join(work, "bad.tsv");
  writeFileSync(bad, "example.com/a\tGPL-3.0\tLGPL-3.0-or-later\twhy\nexample.com/b\tUnknown\tFTL\n");
  const failed = sh('LIC_GO_OVERRIDES="$1"; rows="$(lic_go_overrides)" || exit 7; echo "$rows"', bad);
  assert.equal(failed.status, 7, "a malformed row must fail the caller's command substitution");
  assert.match(failed.stderr, /malformed row/);
});

test("lic_subpackage_licenses lists licences between a package and its module root", () => {
  const mod = path.join(work, "mod");
  for (const dir of ["crypto/keccak", "metrics/exp", "plain"]) {
    mkdirSync(path.join(mod, dir), { recursive: true });
  }
  writeFileSync(path.join(mod, "LICENSE"), "module licence\n");
  writeFileSync(path.join(mod, "crypto/keccak/LICENSE"), "keccak licence\n");
  writeFileSync(path.join(mod, "metrics/LICENSE.txt"), "metrics licence\n");
  const input = [
    `example.com/m|${mod}|${mod}`, // module root package: root licence is not a subdirectory one
    `example.com/m|${mod}|${mod}/crypto/keccak`,
    `example.com/m|${mod}|${mod}/metrics/exp`, // licence one level up, still below the root
    `example.com/m|${mod}|${mod}/metrics`, // same licence again: listed once
    `example.com/m|${mod}|${mod}/plain`,
    `|${mod}|${mod}/crypto/keccak`, // standard-library package: no module
    `example.com/m|${mod}|${work}/elsewhere`, // outside the module directory
    "",
  ].join("\n");
  const res = spawnSync("bash", ["-c", 'source "$0"; lic_subpackage_licenses', LIB], {
    encoding: "utf8",
    input,
  });
  assert.equal(res.status, 0, res.stderr);
  assert.equal(
    res.stdout,
    [
      `example.com/m/crypto/keccak\t${mod}/crypto/keccak/LICENSE`,
      `example.com/m/metrics\t${mod}/metrics/LICENSE.txt`,
      "",
    ].join("\n"),
  );
});

test("lic_licence_reproduced accepts reflowed whitespace and rejects edited text", () => {
  const text = path.join(work, "LICENSE.sub");
  writeFileSync(text, "Copyright 2009 The Go Authors.\n\nRedistribution and use\n  are permitted.\n");
  const agg = path.join(work, "agg.txt");
  const reproduced = (body) => {
    writeFileSync(agg, body);
    return sh('lic_licence_reproduced "$1" "$2"', text, agg).status === 0;
  };
  assert.equal(reproduced("header\n\nCopyright 2009 The Go Authors.\nRedistribution and use are\npermitted.\n"), true);
  assert.equal(reproduced("Copyright 2009 The Go Authors.\nRedistribution and use are allowed.\n"), false);
  assert.equal(reproduced("Redistribution and use are permitted.\n"), false);
});

// The committed NOTICE and LICENSES/go-subpackages.txt must agree: every
// directory licence reproduced in the texts file is named in NOTICE, so
// check.sh passes, and dropping one NOTICE entry makes the gate fail.
test("root NOTICE names every directory licence in LICENSES/go-subpackages.txt", () => {
  const ROOT = path.resolve(HERE, "..", "..");
  const texts = readFileSync(path.join(ROOT, "LICENSES", "go-subpackages.txt"), "utf8");
  const subs = [...texts.matchAll(/^(\S+) \((?:LICENSE|LICENCE|COPYING)[^)]*\)$/gm)].map((m) => m[1]);
  for (const required of [
    "github.com/ethereum/go-ethereum/crypto/keccak",
    "github.com/ethereum/go-ethereum/metrics",
  ]) {
    assert.ok(subs.includes(required), `${required} must be reproduced`);
  }
  const notice = path.join(ROOT, "NOTICE");
  const names = (file, name) => sh('lic_notice_names "$1" "$2"', file, name).status === 0;
  for (const sub of subs) {
    assert.equal(names(notice, sub), true, `NOTICE must name ${sub}`);
  }

  const trimmed = path.join(work, "NOTICE.trimmed");
  writeFileSync(
    trimmed,
    readFileSync(notice, "utf8").replace(/^ {2}github\.com\/ethereum\/go-ethereum\/metrics\n/m, ""),
  );
  assert.equal(names(trimmed, "github.com/ethereum/go-ethereum/metrics"), false);
});
