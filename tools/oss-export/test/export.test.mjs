// End-to-end tests for export.sh against throwaway fixture repositories.
// Every case gets its own temporary root, so the ../oss-private lookup and
// the stub-scanner log never leak between cases.

import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { after, describe, it } from "node:test";
import { gzipSync } from "node:zlib";

import { walk } from "../lib/tree.mjs";
import {
  APACHE_LICENSE,
  MARKERS,
  baseFiles,
  fakeSecret,
  gitIn,
  installStubScanners,
  makeRepo,
  makeTempRoot,
  runExport,
  sha256,
} from "./helpers.mjs";

const EMAIL = "dev@example.com";
const parent = makeTempRoot("e2e");
after(() => fs.rmSync(parent, { recursive: true, force: true }));

// fixture() builds a repo plus stub scanners in a fresh root and returns a
// runner that exports HEAD (or a given ref) into <root>/out.
function fixture(options = {}) {
  const root = fs.mkdtempSync(path.join(parent, "case-"));
  const repo = makeRepo(root, options);
  const scanners = installStubScanners(root);
  const stubLog = path.join(root, "stub.log");
  const run = (args = ["HEAD", "$OUT", "--author-email", EMAIL], runOptions = {}) =>
    runExport(repo, args, {
      scanners,
      ...runOptions,
      env: { STUB_LOG: stubLog, ...(runOptions.env || {}) },
    });
  return { ...repo, scanners, stubLog, run };
}

function exported(result) {
  return walk(result.out).map((entry) => entry.rel);
}

function assertGateFailed(result, id) {
  assert.equal(result.status, 1, `expected exit 1\n${result.output}`);
  assert.equal(result.gate(id)?.status, "fail", `gate ${id} should fail\n${result.output}`);
  assert.ok(!fs.existsSync(path.join(result.out, ".git")), "nothing may be committed when a gate fails");
  assert.match(fs.readFileSync(path.join(result.reportDir, "summary.txt"), "utf8"), /FAILED/);
}

function assertExported(result) {
  assert.equal(result.status, 0, `expected exit 0\n${result.output}`);
  assert.equal(gitIn(result.out, ["rev-list", "--count", "HEAD"]), "1");
}

describe("export.sh", { concurrency: 4 }, () => {
  it("exports a clean tree as one fresh commit and never pushes", () => {
    const fx = fixture({ files: { "keep.log": "kept by a negated pattern\n" } });
    const sourceHead = gitIn(fx.repo, ["rev-parse", "HEAD"]);
    const before = Math.floor(Date.now() / 1000);
    const result = fx.run();
    assertExported(result);

    const out = result.out;
    assert.equal(
      gitIn(out, ["log", "-1", "--format=%an <%ae>|%cn <%ce>|%s"]),
      `Marcos Maceo <${EMAIL}>|Marcos Maceo <${EMAIL}>|Initial public release`,
    );
    const committed = Number(gitIn(out, ["log", "-1", "--format=%ct"]));
    assert.ok(committed >= before - 5 && committed <= Math.floor(Date.now() / 1000) + 5, "committer date is now");
    assert.equal(gitIn(out, ["symbolic-ref", "--short", "HEAD"]), "main");
    assert.equal(gitIn(out, ["remote"]), "");
    assert.equal(gitIn(out, ["status", "--porcelain", "--ignored", "--untracked-files=all"]), "");
    assert.throws(() => gitIn(out, ["cat-file", "-e", `${sourceHead}^{commit}`]), "no private history");

    const files = exported(result);
    assert.deepEqual(gitIn(out, ["ls-files"]).split("\n").sort(), [...files].sort());
    assert.ok(files.includes("LICENSE") && files.includes(".env.example") && files.includes("keep.log"));
    assert.ok(!files.some((f) => f.startsWith("docs/superpowers")), "docs/superpowers is dropped");
    assert.ok(!files.some((f) => f.startsWith("private/")), "drop.txt is applied");
    assert.equal(fs.readFileSync(path.join(out, "src/paths.txt"), "utf8"), "built on /path/to/project/src\n");

    for (const gate of result.gates.gates) assert.equal(gate.status, "pass", gate.id);
    assert.equal(result.gates.gates.length, 10);
    assert.match(result.stdout, /tree: \d+ files/);
    assert.match(fs.readFileSync(path.join(result.reportDir, "summary.txt"), "utf8"), /result: exported/);

    const log = fs.readFileSync(fx.stubLog, "utf8");
    const truffle = JSON.parse(log.split("\n").find((l) => l.startsWith("trufflehog ")).slice("trufflehog ".length));
    assert.ok(truffle.includes("--no-only-verified"), "unverified findings must be reported");
    assert.ok(!truffle.some((a) => a.startsWith("--only-verified")), "--only-verified[=false] hides findings");
    assert.ok(truffle.includes("--no-verification") && truffle.includes("--no-update"));
    const leaksLine = log.split("\n").find((l) => l.startsWith("gitleaks ") && !l.includes('"--help"'));
    const leaks = JSON.parse(leaksLine.slice("gitleaks ".length));
    assert.equal(leaks[0], "dir");
    assert.ok(leaks.includes("--config") && leaks.includes("--ignore-gitleaks-allow"));
  });

  it("exports the ref, never the working tree, and defaults the ref to oss/main", () => {
    const fx = fixture();
    gitIn(fx.repo, ["branch", "oss/main"]);
    fs.writeFileSync(path.join(fx.repo, "src/app.js"), "// acme-private-corp working-tree edit\n");
    fs.writeFileSync(path.join(fx.repo, "untracked.txt"), "acme-private-corp\n");
    const result = fx.run(["$OUT", "--author-email", EMAIL]);
    assertExported(result);
    assert.match(result.stdout, /ref\s+oss\/main/);
    assert.equal(fs.readFileSync(path.join(result.out, "src/app.js"), "utf8"), baseFiles()["src/app.js"]);
    assert.ok(!fs.existsSync(path.join(result.out, "untracked.txt")));
  });

  it("--no-commit runs every gate but creates no repository", () => {
    const result = fixture().run(["HEAD", "$OUT", "--author-email", EMAIL, "--no-commit"]);
    assert.equal(result.status, 0, result.output);
    assert.ok(!fs.existsSync(path.join(result.out, ".git")));
    assert.ok(result.gates.ok);
    assert.match(fs.readFileSync(path.join(result.reportDir, "summary.txt"), "utf8"), /--no-commit/);
  });

  // (a) gitleaks -------------------------------------------------------------

  it("(a) fails on a gitleaks finding without printing the value", () => {
    const secret = fakeSecret(MARKERS.gitleaks);
    const files = { "src/config.js": `const token = "${secret}";\nconst again = "${secret}";\n` };
    const result = fixture({ files }).run();
    assertGateFailed(result, "gitleaks");
    assert.deepEqual(result.gate("gitleaks").hits, [
      { file: "src/config.js", line: 1, rule: "stub-rule" },
      { file: "src/config.js", line: 2, rule: "stub-rule" },
    ]);
    assert.ok(!result.output.includes(secret), "the secret must never be printed");
    assert.ok(!fs.readFileSync(path.join(result.reportDir, "gates.json"), "utf8").includes(secret));
    const candidates = fs.readFileSync(path.join(result.reportDir, "secrets-allow.candidates.txt"), "utf8");
    const lines = candidates.split("\n").filter((l) => l && !l.startsWith("#"));
    assert.deepEqual(lines, [`gitleaks stub-rule src/config.js sha256:${sha256(secret)}`], "one line per fingerprint");
  });

  it("(a) passes when the finding is pinned in secrets-allow.txt, and fails again if the value changes", () => {
    const secret = fakeSecret(MARKERS.gitleaks);
    const allow = `gitleaks stub-rule src/config.js sha256:${sha256(secret)}\n`;
    const ok = fixture({
      files: {
        "src/config.js": `const token = "${secret}";\n`,
        "docs/superpowers/oss-export/secrets-allow.txt": allow,
      },
    }).run();
    assertExported(ok);
    assert.equal(ok.gate("gitleaks").allowlisted, 1);

    const changed = fixture({
      files: {
        "src/config.js": `const token = "${fakeSecret(MARKERS.gitleaks)}";\n`,
        "docs/superpowers/oss-export/secrets-allow.txt": allow,
      },
    }).run();
    assertGateFailed(changed, "gitleaks");
  });

  it("(a) fails closed on a root .gitleaksignore", () => {
    const result = fixture({ files: { ".gitleaksignore": "src/config.js:stub-rule:1\n" } }).run();
    assertGateFailed(result, "gitleaks");
    assert.equal(result.gate("gitleaks").hits[0].rule, "scanner-suppression-file");
  });

  // (b) trufflehog -----------------------------------------------------------

  it("(b) fails on an unverified trufflehog finding and passes once pinned", () => {
    const secret = fakeSecret(MARKERS.trufflehog);
    const files = { "src/db.js": `const url = "${secret}";\n` };
    const failed = fixture({ files }).run();
    assertGateFailed(failed, "trufflehog");
    assert.deepEqual(failed.gate("trufflehog").hits, [{ file: "src/db.js", line: 1, rule: "StubDetector" }]);
    assert.ok(!failed.output.includes(secret));

    const allow = `trufflehog StubDetector src/db.js sha256:${sha256(secret)}\n`;
    const ok = fixture({ files: { ...files, "docs/superpowers/oss-export/secrets-allow.txt": allow } }).run();
    assertExported(ok);
    assert.equal(ok.gate("trufflehog").allowlisted, 1);
  });

  it("(b) a finding that verifies as live is fatal even when allowlisted", () => {
    const secret = fakeSecret(MARKERS.trufflehogLive);
    const files = {
      "src/live.js": `const key = "${secret}";\n`,
      "docs/superpowers/oss-export/secrets-allow.txt": `trufflehog StubDetector src/live.js sha256:${sha256(secret)}\n`,
    };
    const fx = fixture({ files });
    const verified = fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--trufflehog-verify"]);
    assertGateFailed(verified, "trufflehog");
    assert.equal(verified.gate("trufflehog").hits[0].rule, "StubDetector (VERIFIED LIVE)");
    const log = fs.readFileSync(fx.stubLog, "utf8");
    assert.ok(!log.split("\n").find((l) => l.startsWith("trufflehog ")).includes("--no-verification"));

    // Without --trufflehog-verify nothing is verified, so the pin applies.
    const unverified = fixture({ files }).run();
    assertExported(unverified);
  });

  // Missing scanners ---------------------------------------------------------

  it("a missing scanner fails unless --allow-missing-scanner is given", () => {
    const env = { GITLEAKS_BIN: "/nonexistent/gitleaks", TRUFFLEHOG_BIN: "/nonexistent/trufflehog" };
    const failed = fixture().run(undefined, { env });
    assertGateFailed(failed, "gitleaks");
    assert.equal(failed.gate("trufflehog").status, "fail");

    const warned = fixture().run(["HEAD", "$OUT", "--author-email", EMAIL, "--allow-missing-scanner"], { env });
    assertExported(warned);
    assert.equal(warned.gate("gitleaks").status, "warn");
    assert.equal(warned.gate("trufflehog").status, "warn");
    assert.match(warned.output, /NOT installed/);
  });

  // (c) forbidden strings ----------------------------------------------------

  it("(c) fails on a forbidden string in content, in a path and in the author", () => {
    const content = fixture({ files: { "src/a.js": "// built for ACME-Private-Corp\n" } }).run();
    assertGateFailed(content, "forbidden-strings");
    assert.deepEqual(content.gate("forbidden-strings").hits, [{ file: "src/a.js", line: 1, rule: "private-org" }]);
    assert.ok(!content.output.includes("ACME-Private-Corp"));

    const pathHit = fixture({ files: { "clients/acme-private-corp.md": "x\n" } }).run();
    assertGateFailed(pathHit, "forbidden-strings");
    assert.equal(pathHit.gate("forbidden-strings").hits[0].line, "path");

    const author = fixture().run(["HEAD", "$OUT", "--author-email", "me@acme-private-corp.example"]);
    assertGateFailed(author, "forbidden-strings");
    assert.equal(author.gate("forbidden-strings").hits[0].file, "<commit author>");
  });

  it("(c) fails on a forbidden string in the commit message", () => {
    const leaky = fixture().run(["HEAD", "$OUT", "--author-email", EMAIL, "--message", "Public release\n\nThanks acme-private-corp"]);
    assertGateFailed(leaky, "forbidden-strings");
    assert.deepEqual(leaky.gate("forbidden-strings").hits, [{ file: "<commit message>", line: 3, rule: "private-org" }]);
    assert.ok(!leaky.output.includes("Thanks acme"));

    // A message that starts with a dash is still read as the message.
    const dashed = fixture().run(["HEAD", "$OUT", "--author-email", EMAIL, "--message", "--from acme-private-corp"]);
    assertGateFailed(dashed, "forbidden-strings");
    assert.equal(dashed.gate("forbidden-strings").hits[0].file, "<commit message>");

    const clean = fixture().run(["HEAD", "$OUT", "--author-email", EMAIL, "--message", "First public cut"]);
    assertExported(clean);
    assert.equal(gitIn(clean.out, ["log", "-1", "--format=%s"]), "First public cut");
  });

  it("(c) a sha256: rule fails a pinned blob under any name", () => {
    const pixels = Buffer.concat([Buffer.from("RIFF\0\0\0\0WEBP"), randomBytes(64)]);
    const forbidden = `acme-private-corp ;; name=private-org\nsha256:${sha256(pixels)} ;; name=leaky-screenshot\n`;
    const pinned = fixture({
      files: { "assets/renamed.webp": pixels, "docs/superpowers/oss-export/forbidden.txt": forbidden },
    }).run();
    assertGateFailed(pinned, "forbidden-strings");
    assert.deepEqual(pinned.gate("forbidden-strings").hits, [
      { file: "assets/renamed.webp", line: "blob", rule: "leaky-screenshot" },
    ]);

    const regenerated = Buffer.concat([pixels, Buffer.from([0])]);
    const ok = fixture({
      files: { "assets/renamed.webp": regenerated, "docs/superpowers/oss-export/forbidden.txt": forbidden },
    }).run();
    assertExported(ok);
  });

  it("(c) a gzip-wrapped leak and an unreadable archive fail; --allow-opaque exempts the archive", () => {
    const leak = fixture({ files: { "logs/run.txt.gz": gzipSync("start\nacme-private-corp\n") } }).run();
    assertGateFailed(leak, "forbidden-strings");
    assert.deepEqual(leak.gate("forbidden-strings").hits, [{ file: "logs/run.txt.gz", line: "gzip:2", rule: "private-org" }]);

    const files = { "docs/deck.pptx": Buffer.concat([Buffer.from("PK\x03\x04", "latin1"), randomBytes(64)]) };
    const opaque = fixture({ files }).run();
    assertGateFailed(opaque, "forbidden-strings");
    assert.deepEqual(opaque.gate("forbidden-strings").hits, [{ file: "docs/deck.pptx", line: "zip", rule: "opaque-archive" }]);
    const allowed = fixture({ files }).run(["HEAD", "$OUT", "--author-email", EMAIL, "--allow-opaque", "docs/*.pptx"]);
    assertExported(allowed);
    assert.ok(exported(allowed).includes("docs/deck.pptx"));
  });

  it("the archive ignores the user's global and system git config and attributes", () => {
    const fx = fixture();
    const hostile = path.join(fx.root, "hostile");
    fs.mkdirSync(path.join(hostile, "xdg", "git"), { recursive: true });
    fs.writeFileSync(path.join(hostile, "attributes"), "src/app.js export-ignore\n");
    fs.writeFileSync(path.join(hostile, "xdg", "git", "attributes"), "README.md export-ignore\n");
    fs.writeFileSync(
      path.join(hostile, "global.gitconfig"),
      `[core]\n\tattributesFile = ${path.join(hostile, "attributes")}\n`,
    );
    fs.writeFileSync(path.join(hostile, "system.gitconfig"), "[core]\n\tautocrlf = true\n");
    const result = fx.run(undefined, {
      env: {
        GIT_CONFIG_GLOBAL: path.join(hostile, "global.gitconfig"),
        GIT_CONFIG_SYSTEM: path.join(hostile, "system.gitconfig"),
        GIT_CONFIG_NOSYSTEM: "0",
        XDG_CONFIG_HOME: path.join(hostile, "xdg"),
      },
    });
    assertExported(result);
    const files = exported(result);
    assert.ok(files.includes("src/app.js"), "a global export-ignore must not hide a file");
    assert.ok(files.includes("README.md"), "the XDG attributes file must not hide a file");
    assert.equal(fs.readFileSync(path.join(result.out, "src/app.js"), "utf8"), "export const answer = 42;\n", "no CRLF conversion");
  });

  it("(c) the gates run after the scrub", () => {
    const files = {
      "docs/superpowers/oss-export/forbidden.txt": "acme-private-corp ;; name=private-org\nlit:/home/alice ;; name=home\n",
    };
    assertExported(fixture({ files }).run());
    const noScrub = fixture({ files: { ...files, "docs/superpowers/oss-export/scrub.rules": null } }).run();
    assertGateFailed(noScrub, "forbidden-strings");
    assert.deepEqual(noScrub.gate("forbidden-strings").hits, [{ file: "src/paths.txt", line: 1, rule: "home" }]);
  });

  it("(c) a forbidden.txt in ../oss-private takes precedence over the ref's copy", () => {
    const word = `zeta${randomBytes(4).toString("hex")}`;
    const files = { "src/word.txt": `${word}\n` };
    assertExported(fixture({ files }).run());

    const fx = fixture({ files });
    fs.mkdirSync(path.join(fx.root, "oss-private"));
    fs.writeFileSync(path.join(fx.root, "oss-private/forbidden.txt"), `${word} ;; name=private-word\n`);
    const result = fx.run();
    assertGateFailed(result, "forbidden-strings");
    assert.match(result.stdout, /forbidden\.txt <- private dir/);
    assert.equal(result.gate("forbidden-strings").hits[0].rule, "private-word");
  });

  it("(c) --config-dir is exclusive and explicit flags win", () => {
    const fx = fixture();
    const dir = path.join(fx.root, "cfg");
    fs.mkdirSync(dir);
    fs.writeFileSync(path.join(dir, "forbidden.txt"), "answer = 42 ;; name=answer\n");
    const viaDir = fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--config-dir", dir]);
    assertGateFailed(viaDir, "forbidden-strings");
    // drop.txt was not in --config-dir, so private/ survives (no fallback).
    assert.ok(exported(viaDir).includes("private/notes.md"));

    const flag = path.join(fx.root, "flag-forbidden.txt");
    fs.writeFileSync(flag, "nothing-matches-this ;; name=none\n");
    const viaFlag = fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--config-dir", dir, "--forbidden", flag], {
      outName: "out2",
    });
    assertExported(viaFlag);
  });

  // (d) file size ------------------------------------------------------------

  it("(d) fails on a large file unless allowlisted; the CJK fonts are allowlisted by default", () => {
    // The limit (about 20 KB) sits above the fixture's real LICENSE (11 KB).
    const big = { "assets/big.bin": Buffer.alloc(32768, 1) };
    const args = ["HEAD", "$OUT", "--author-email", EMAIL, "--max-file-mb", "0.02"];
    const failed = fixture({ files: big }).run(args);
    assertGateFailed(failed, "file-size");
    assert.equal(failed.gate("file-size").hits[0].file, "assets/big.bin");

    assertExported(fixture({ files: big }).run([...args, "--size-allow", "assets/*.bin"]));

    const font = { "frontend/public/fonts/print/noto-sans-cjk-sc/Regular.otf": Buffer.alloc(32768, 1) };
    const fonts = fixture({ files: font }).run(args);
    assertExported(fonts);
    assert.equal(fonts.gate("file-size").notes.length, 1);
  });

  // (e) symlinks -------------------------------------------------------------

  it("(e) fails on a symlink that leaves the tree and accepts in-tree links", () => {
    const failed = fixture({ symlinks: { "src/escape": "../../outside" } }).run();
    assertGateFailed(failed, "symlinks");
    assert.equal(failed.gate("symlinks").hits[0].rule, "escapes-tree");

    const chained = fixture({ symlinks: { "sub/up": "..", "sub/esc": "up/../.." } }).run();
    assertGateFailed(chained, "symlinks");
    assert.deepEqual(chained.gate("symlinks").hits, [{ file: "sub/esc", rule: "escapes-tree" }]);

    const absolute = fixture({ symlinks: { "src/abs": "/etc/hosts" } }).run();
    assertGateFailed(absolute, "symlinks");

    const ok = fixture({ symlinks: { "docs/readme-link.md": "../README.md" } }).run();
    assertExported(ok);
    assert.ok(fs.lstatSync(path.join(ok.out, "docs/readme-link.md")).isSymbolicLink());
  });

  // (f) env files ------------------------------------------------------------

  it("(f) fails on a tracked env file unless explicitly allowed", () => {
    const files = { "ci/.env.ci": "MODE=test\n" };
    const failed = fixture({ files }).run();
    assertGateFailed(failed, "env-files");
    assert.equal(failed.gate("env-files").hits[0].file, "ci/.env.ci");
    assertExported(fixture({ files }).run(["HEAD", "$OUT", "--author-email", EMAIL, "--allow-env-file", "ci/.env.ci"]));
  });

  // (g) gitignore ------------------------------------------------------------

  it("(g) fails when a tracked file is ignored by the exported .gitignore", () => {
    const result = fixture({ forced: { "debug.log": "x\n" } }).run();
    assertGateFailed(result, "gitignore-clean");
    assert.deepEqual(
      result.gate("gitignore-clean").hits.map((h) => h.file),
      ["debug.log"],
    );
  });

  // (h) license --------------------------------------------------------------

  it("(h) fails when LICENSE is missing or not Apache-2.0", () => {
    const missing = fixture({ files: { LICENSE: null } }).run();
    assertGateFailed(missing, "license");
    assert.equal(missing.gate("license").hits[0].rule, "missing");

    const mit = fixture({ files: { LICENSE: "MIT License\n\nCopyright (c) 2026\n" } }).run();
    assertGateFailed(mit, "license");
    assert.equal(mit.gate("license").hits[0].rule, "not-apache-2.0");
  });

  it("(h) a filled-in copyright line must name --copyright-holder, which defaults to the author name", () => {
    const fill = (line) => APACHE_LICENSE.replace("[yyyy] [name of copyright owner]", line);
    const owner = fixture({ files: { LICENSE: fill("2026 Marcos Maceo") } }).run();
    assertExported(owner);

    const rider = fixture({ files: { LICENSE: fill("2026 Marcos Maceo. Commercial use is not permitted (Commons Clause)") } });
    const riderResult = rider.run();
    assertGateFailed(riderResult, "license");
    assert.equal(riderResult.gate("license").hits[0].rule, "copyright-holder-mismatch");

    const other = fixture({ files: { LICENSE: fill("2024-2026 Acme, Inc.") } });
    assert.equal(other.run().gate("license").hits[0].rule, "copyright-holder-mismatch");
    assertExported(other.run(["HEAD", "$OUT", "--author-email", EMAIL, "--copyright-holder", "Acme, Inc."], { outName: "out2" }));
  });

  // (i) private paths --------------------------------------------------------

  it("(i) docs/superpowers never ships, even without a drop.txt", () => {
    const result = fixture({ files: { "docs/superpowers/oss-export/drop.txt": null, "docs/superpowers/specs/s.md": "x\n" } }).run();
    assertExported(result);
    assert.equal(result.gate("private-paths").status, "pass");
    assert.ok(!exported(result).some((f) => f.startsWith("docs/superpowers")));
    assert.ok(exported(result).includes("private/notes.md"), "no drop.txt, so private/ ships");
  });

  it("(i) a nested docs/superpowers tree is dropped too", () => {
    const files = {
      "frontend/docs/superpowers/plans/p.md": "# nested private plan\n",
      "frontend/docs/guide.md": "# public guide\n",
    };
    const result = fixture({ files }).run();
    assertExported(result);
    assert.ok(!exported(result).some((f) => f.includes("docs/superpowers")), "nested private tree is dropped");
    assert.ok(exported(result).includes("frontend/docs/guide.md"));
    assert.match(result.stdout, /\*\*\/docs\/superpowers\s+\(builtin\)/);
  });

  // Usage and configuration errors (exit 2) ----------------------------------

  it("requires --author-email and a plausible address", () => {
    const fx = fixture();
    const none = fx.run(["HEAD", "$OUT"]);
    assert.equal(none.status, 2);
    assert.match(none.stderr, /--author-email is required/);
    assert.ok(!fs.existsSync(none.out));
    assert.equal(fx.run(["HEAD", "$OUT", "--author-email", "not-an-email"]).status, 2);
    assert.equal(fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--author-name", "A <b>"]).status, 2);
  });

  it("refuses an outdir inside the repository or a non-empty outdir", () => {
    const fx = fixture();
    const inside = runExport(fx, ["HEAD", path.join(fx.repo, "exported"), "--author-email", EMAIL], {
      scanners: fx.scanners,
    });
    assert.equal(inside.status, 2);
    assert.match(inside.stderr, /outside the repository/);
    assert.ok(!fs.existsSync(path.join(fx.repo, "exported")), "a refused outdir is removed again");

    const busy = path.join(fx.root, "busy");
    fs.mkdirSync(busy);
    fs.writeFileSync(path.join(busy, "file"), "x");
    const nonEmpty = runExport(fx, ["HEAD", busy, "--author-email", EMAIL], { scanners: fx.scanners });
    assert.equal(nonEmpty.status, 2);
    assert.match(nonEmpty.stderr, /not empty/);
    assert.deepEqual(fs.readdirSync(busy), ["file"]);
  });

  it("refuses an unknown ref, --push and unknown options", () => {
    const fx = fixture();
    assert.equal(fx.run(["no-such-ref", "$OUT", "--author-email", EMAIL]).status, 2);
    const push = fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--push"]);
    assert.equal(push.status, 2);
    assert.match(push.stderr, /never pushes/);
    assert.equal(fx.run(["HEAD", "$OUT", "--author-email", EMAIL, "--bogus"]).status, 2);
  });

  it("refuses to run without forbidden.txt and wipes the raw archive", () => {
    const result = fixture({ files: { "docs/superpowers/oss-export/forbidden.txt": null } }).run();
    assert.equal(result.status, 2, result.output);
    assert.match(result.stderr, /no forbidden\.txt found/);
    assert.ok(!fs.existsSync(result.out), "the unsanitized archive must not be left behind");
  });

  it("rejects an invalid config before touching the tree and wipes the raw archive", () => {
    const result = fixture({ files: { "docs/superpowers/oss-export/scrub.rules": "s#(#x#g\n" } }).run();
    assert.equal(result.status, 2, result.output);
    assert.match(result.stderr, /invalid regular expression/);
    assert.ok(!fs.existsSync(result.out));
  });
});

// Optional: one check against a real gitleaks binary, to prove the pinned
// config and --ignore-gitleaks-allow hold against in-tree suppression.
const realGitleaks = process.env.OSS_EXPORT_REAL_GITLEAKS;
describe("export.sh with real gitleaks", { skip: !realGitleaks && "set OSS_EXPORT_REAL_GITLEAKS to run" }, () => {
  it("ignores an in-tree .gitleaks.toml allowlist and gitleaks:allow comments", () => {
    const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    const key = `AKIA${Array.from(randomBytes(16), (b) => alphabet[b % 32]).join("")}`;
    const fx = fixture({
      files: {
        "src/aws.js": `export const accessKeyId = "${key}"; // gitleaks:allow\n`,
        ".gitleaks.toml": "[allowlist]\npaths = ['''.*''']\n",
      },
    });
    const result = fx.run(undefined, { env: { GITLEAKS_BIN: realGitleaks } });
    assertGateFailed(result, "gitleaks");
    assert.deepEqual(
      result.gate("gitleaks").hits.map((h) => `${h.file}:${h.line}:${h.rule}`),
      ["src/aws.js:1:aws-access-token"],
    );
    assert.ok(!result.output.includes(key));
  });
});
