import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { fixtureAllowlist, scanHistoryPatch, scanText } from "./scan-secrets.mjs";

const scannerPath = fileURLToPath(new URL("./scan-secrets.mjs", import.meta.url));
const repoRoot = path.resolve(path.dirname(scannerPath), "..");

// Synthetic values are assembled at runtime so this file never matches itself.
const stripeLike = (fill) => `sk_live_${fill.repeat(24)}`;

function git(repo, ...args) {
  return execFileSync("git", args, { cwd: repo, encoding: "utf8" });
}

function writeRepoFile(repo, file, contents) {
  mkdirSync(path.dirname(path.join(repo, file)), { recursive: true });
  writeFileSync(path.join(repo, file), contents);
}

function assertNoValueLeak(result, value) {
  assert.ok(!`${result.stdout}${result.stderr}`.includes(value));
}

function withGitRepository(run) {
  const repo = mkdtempSync(path.join(os.tmpdir(), "payverge-secret-scan-"));
  try {
    execFileSync("git", ["init", "-q"], { cwd: repo });
    execFileSync("git", ["config", "user.email", "scanner@test.invalid"], { cwd: repo });
    execFileSync("git", ["config", "user.name", "Secret Scanner Test"], { cwd: repo });
    execFileSync("git", ["config", "commit.gpgsign", "false"], { cwd: repo });
    run(repo);
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
}

function runScanner(repo, ...args) {
  return spawnSync(process.execPath, [scannerPath, ...args], {
    cwd: repo,
    encoding: "utf8",
  });
}

test("harmless fake-secret canary proves detection without a real credential", () => {
  const findings = scanText(
    "PAYVERGE_FAKE_SECRET_CANARY_DO_NOT_USE=unit-only-canary-0123456789",
    "scripts/fixtures/fake-secret-canary.txt",
    { includeTestCanary: true },
  );
  assert.equal(findings.length, 1);
  assert.equal(findings[0].rule, "payverge-test-canary");
  assert.ok(!JSON.stringify(findings).includes("unit-only-canary-0123456789"));
});

test("ordinary documentation is not a finding", () => {
  assert.deepEqual(scanText("Set TOKEN through SSM before release.", "README.md"), []);
});

test("history findings name the exact commit and path without exposing values", () => {
  const syntheticValue = `sk_live_${"x".repeat(24)}`;
  const findings = scanHistoryPatch(`commit:${"a".repeat(40)}
diff --git a/deleted/backup.sql b/deleted/backup.sql
deleted file mode 100644
index 1111111..0000000
--- a/deleted/backup.sql
+++ /dev/null
@@ -1 +0,0 @@
+++ PostgreSQL database dump complete
-provider_secret=${syntheticValue}
`);

  assert.equal(findings.length, 1);
  assert.equal(findings[0].rule, "stripe-live-secret");
  assert.equal(
    findings[0].file,
    `git-history:${"a".repeat(40)}:deleted/backup.sql`,
  );
  assert.ok(!JSON.stringify(findings).includes(syntheticValue));
});

test("repository scan covers untracked worktree files", () => {
  withGitRepository((repo) => {
    const syntheticValue = `sk_live_${"u".repeat(24)}`;
    writeFileSync(path.join(repo, "untracked.txt"), `secret=${syntheticValue}\n`);

    const result = runScanner(repo);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: untracked\.txt:1/);
    assert.ok(!`${result.stdout}${result.stderr}`.includes(syntheticValue));
  });
});

test("repository scan covers index content that differs from the worktree", () => {
  withGitRepository((repo) => {
    const syntheticValue = `sk_live_${"i".repeat(24)}`;
    writeFileSync(path.join(repo, "staged.txt"), `secret=${syntheticValue}\n`);
    execFileSync("git", ["add", "staged.txt"], { cwd: repo });
    writeFileSync(path.join(repo, "staged.txt"), "safe worktree replacement\n");

    const result = runScanner(repo);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: git-index:staged\.txt:1/);
    assert.ok(!`${result.stdout}${result.stderr}`.includes(syntheticValue));
  });
});

test("history scan includes deleted env backup paths", () => {
  withGitRepository((repo) => {
    const syntheticValue = `sk_live_${"h".repeat(24)}`;
    writeFileSync(path.join(repo, ".env.bak.20200101"), `secret=${syntheticValue}\n`);
    execFileSync("git", ["add", ".env.bak.20200101"], { cwd: repo });
    execFileSync("git", ["commit", "-qm", "fixture add"], { cwd: repo });
    execFileSync("git", ["rm", "-q", ".env.bak.20200101"], { cwd: repo });
    execFileSync("git", ["commit", "-qm", "fixture remove"], { cwd: repo });

    const result = runScanner(repo, "--history");
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: git-history:[a-f0-9]{40}:\.env\.bak\.20200101:1/);
    assert.ok(!`${result.stdout}${result.stderr}`.includes(syntheticValue));
  });
});

test("staged scan flags a staged secret before the first commit", () => {
  withGitRepository((repo) => {
    const syntheticValue = stripeLike("s");
    const file = "config/with space ñ.env";
    writeRepoFile(repo, file, `ok=1\nsecret=${syntheticValue}\n`);
    git(repo, "add", file);

    const result = runScanner(repo, "--staged");
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: config\/with space ñ\.env:2/);
    assertNoValueLeak(result, syntheticValue);
  });
});

test("staged scan reads the index blob, not the worktree file", () => {
  withGitRepository((repo) => {
    const syntheticValue = stripeLike("b");
    writeRepoFile(repo, "staged.txt", `secret=${syntheticValue}\n`);
    git(repo, "add", "staged.txt");
    writeRepoFile(repo, "staged.txt", "safe worktree replacement\n");

    const result = runScanner(repo, "--staged");
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: staged\.txt:1/);
    assertNoValueLeak(result, syntheticValue);
  });
});

test("staged scan flags a secret added to an already-committed file", () => {
  withGitRepository((repo) => {
    const syntheticValue = stripeLike("m");
    writeRepoFile(repo, "app/settings.py", "DEBUG = False\n");
    git(repo, "add", "app/settings.py");
    git(repo, "commit", "-qm", "fixture base");
    writeRepoFile(repo, "app/settings.py", `DEBUG = False\nKEY = "${syntheticValue}"\n`);
    git(repo, "add", "app/settings.py");

    const result = runScanner(repo, "--staged");
    assert.equal(result.status, 1);
    assert.match(result.stderr, /stripe-live-secret: app\/settings\.py:2/);
    assertNoValueLeak(result, syntheticValue);
  });
});

test("staged scan ignores unstaged edits, untracked files and staged deletions", () => {
  withGitRepository((repo) => {
    const committedValue = stripeLike("d");
    writeRepoFile(repo, "tracked.txt", "safe\n");
    writeRepoFile(repo, "removed.txt", `secret=${committedValue}\n`);
    git(repo, "add", "tracked.txt", "removed.txt");
    git(repo, "commit", "-qm", "fixture base");

    writeRepoFile(repo, "tracked.txt", `secret=${stripeLike("w")}\n`);
    writeRepoFile(repo, "untracked.txt", `secret=${stripeLike("u")}\n`);
    git(repo, "rm", "-q", "removed.txt");
    writeRepoFile(repo, "clean.txt", "nothing to see\n");
    git(repo, "add", "clean.txt");

    const result = runScanner(repo, "--staged");
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout, "staged secret scan passed\n");

    // The full scan still sees the worktree and untracked material.
    const full = runScanner(repo, "--all");
    assert.equal(full.status, 1);
    assert.match(full.stderr, /stripe-live-secret: tracked\.txt:1/);
    assert.match(full.stderr, /stripe-live-secret: untracked\.txt:1/);
  });
});

test("staged scan skips binary blobs like the full scan", () => {
  withGitRepository((repo) => {
    writeFileSync(
      path.join(repo, "image.bin"),
      Buffer.concat([Buffer.from([0, 1, 2, 0]), Buffer.from(`k=${stripeLike("n")}\n`)]),
    );
    git(repo, "add", "image.bin");

    const result = runScanner(repo, "--staged");
    assert.equal(result.status, 0, result.stderr);
  });
});

// The first allowlisted fixture still present in this checkout, so the test
// survives fixtures being moved or deleted.
function trackedAllowlistedFixture() {
  for (const entry of fixtureAllowlist) {
    const file = entry.split(":")[1];
    let contents;
    try {
      contents = readFileSync(path.join(repoRoot, file));
    } catch {
      continue;
    }
    if (scanText(contents.toString("utf8"), file).length > 0) return { file, contents };
  }
  return null;
}

test("staged and full scans honor the tracked fixture allowlist", (t) => {
  const fixture = trackedAllowlistedFixture();
  if (!fixture) {
    t.skip("no allowlisted fixture left in this checkout");
    return;
  }
  const { file, contents } = fixture;
  withGitRepository((repo) => {
    writeRepoFile(repo, file, contents);
    git(repo, "add", file);
    assert.equal(runScanner(repo, "--staged").status, 0);
    assert.equal(runScanner(repo, "--all").status, 0);

    // The same value under any other path is not allowlisted.
    const copy = `copy/${path.basename(file)}`;
    writeRepoFile(repo, copy, contents);
    assert.equal(runScanner(repo, "--all").status, 1);
    git(repo, "add", copy);
    const moved = runScanner(repo, "--staged");
    assert.equal(moved.status, 1);
    assert.ok(moved.stderr.includes(`: ${copy}:`), moved.stderr);
    assert.ok(!moved.stderr.includes(`: ${file}:`), moved.stderr);
  });
});

test("full scan without flags equals --all", () => {
  withGitRepository((repo) => {
    writeRepoFile(repo, "untracked.txt", `secret=${stripeLike("a")}\n`);
    const bare = runScanner(repo);
    const all = runScanner(repo, "--all");
    assert.equal(all.status, 1);
    assert.equal(bare.status, all.status);
    assert.equal(bare.stderr, all.stderr);
  });
});

test("unknown or conflicting flags fail closed with usage", () => {
  withGitRepository((repo) => {
    for (const args of [["--stage"], ["--all", "--staged"]]) {
      const result = runScanner(repo, ...args);
      assert.equal(result.status, 2, args.join(" "));
      assert.match(result.stderr, /usage: node scripts\/scan-secrets\.mjs/);
    }
  });
});

test("pre-commit hook scans only staged content", () => {
  const hook = readFileSync(path.join(repoRoot, "scripts", "hooks", "pre-commit"), "utf8");
  assert.match(hook, /scripts\/scan-secrets\.mjs" --staged/);
});
