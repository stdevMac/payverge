#!/usr/bin/env node

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { validateRunbookCatalog } from "./runbook-catalog.mjs";
import { parseDropList } from "../../tools/oss-export/lib/config.mjs";
import { withBuiltinDrops } from "../../tools/oss-export/lib/steps.mjs";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const read = (...parts) => readFileSync(path.join(ROOT, ...parts), "utf8");

test("Cloudflare trusted-proxy snapshot updater can verify freshness", () => {
  const updater = read("scripts", "update-cloudflare-cidrs.ts");
  assert.match(updater, /--check/);
  assert.match(updater, /--max-age-days/);
  assert.match(updater, /snapshot hash does not match Cloudflare/);
  assert.match(updater, /snapshot is older than/);
});

test("self-host edge runs the stock Caddy image with the checked-in deploy files", () => {
  const compose = read("deploy", "docker-compose.yml");
  // The official image (no custom build), pinned to a 2.x tag and its digest.
  assert.match(compose, /^\s+image: caddy:2\.\d+\.\d+(-alpine)?@sha256:[0-9a-f]{64}\s*$/m);
  assert.doesNotMatch(compose, /caddy\/Dockerfile/);
  for (const file of ["Caddyfile", "Caddyfile.cloudflare", "payverge.caddy", "cloudflare-cidrs.caddy"]) {
    assert.ok(compose.includes(`./${file}:/etc/caddy/${file}:ro`), `compose mounts ${file}`);
    assert.ok(read("deploy", file).length > 0, `deploy/${file} exists`);
  }
  assert.match(read("deploy", "Caddyfile.cloudflare"), /import cloudflare-cidrs\.caddy/);
});

test("production rejects OpenRouter audit mode", () => {
  assert.match(
    read("backend", "internal", "config", "production_preflight.go"),
    /openrouter\.zdr_mode\.audit/,
  );
});

test("every active runbook has an owner and bounded review dates", (t) => {
  const catalog = JSON.parse(read("docs", "runbooks", "catalog.json"));
  const tracked = readdirSync(path.join(ROOT, "docs", "runbooks"))
    .filter((name) => name.endsWith(".md"))
    .sort();
  const now = process.env.RUNBOOK_REVIEW_NOW
    ? Date.parse(process.env.RUNBOOK_REVIEW_NOW)
    : Date.now();
  // A passing date alone must not turn every contributor's (and every fork's)
  // CI red, so an overdue review is a diagnostic here. A maintainer job sets
  // RUNBOOK_REVIEW_ENFORCE=1 to make it fail.
  const enforceDue = process.env.RUNBOOK_REVIEW_ENFORCE === "1";
  let overdue = [];
  assert.doesNotThrow(() => {
    ({ overdue } = validateRunbookCatalog(catalog, tracked, now, { enforceDue }));
  });
  for (const runbook of overdue) {
    t.diagnostic(`${runbook} review is overdue (RUNBOOK_REVIEW_ENFORCE=1 fails on this)`);
  }
});

test("runbook catalog rejects an overdue review using an injectable UTC clock", () => {
  const catalog = {
    runbooks: [
      {
        path: "example.md",
        owner: "operations_owner",
        reviewed_on: "2026-01-01",
        review_due: "2026-02-01",
      },
    ],
  };
  assert.throws(
    () =>
      validateRunbookCatalog(
        catalog,
        ["example.md"],
        Date.parse("2026-02-02T00:00:00Z"),
      ),
    /example\.md review is overdue/,
  );
});

test("an overdue runbook review is reported, not fatal, unless enforced", () => {
  const entry = {
    path: "example.md",
    owner: "operations_owner",
    reviewed_on: "2026-01-01",
    review_due: "2026-02-01",
  };
  const late = Date.parse("2026-02-02T00:00:00Z");
  assert.deepEqual(
    validateRunbookCatalog({ runbooks: [entry] }, ["example.md"], late, { enforceDue: false }),
    { overdue: ["example.md"] },
  );
  assert.deepEqual(
    validateRunbookCatalog({ runbooks: [entry] }, ["example.md"], Date.parse("2026-02-01T00:00:00Z"), {
      enforceDue: false,
    }),
    { overdue: [] },
  );
  // Structural problems stay fatal even when the due date is advisory.
  assert.throws(
    () =>
      validateRunbookCatalog(
        { runbooks: [{ ...entry, review_due: "2026-12-01" }] },
        ["example.md"],
        late,
        { enforceDue: false },
      ),
    /review interval exceeds 184 days/,
  );
  assert.throws(
    () =>
      validateRunbookCatalog({ runbooks: [{ ...entry, owner: "TBD" }] }, ["example.md"], late, {
        enforceDue: false,
      }),
    /example\.md owner/,
  );
});

test("active entry-point documentation names current providers", () => {
  const agents = read("AGENTS.md");
  // Email: Resend and Postmark are peer API providers; `log` is the default.
  assert.match(agents, /\*\*Email\*\*: the API providers are Resend and Postmark, chosen with `EMAIL_PROVIDER`/);
  assert.match(agents, /With nothing configured the provider is `log`/);
  assert.match(agents, /\*\*AI\*\*: OpenRouter/);

  const readme = read("README.md");
  assert.match(readme, /EMAIL_PROVIDER/);
  assert.match(readme, /EMAIL_API_KEY/);
  assert.match(readme, /RESEND_WEBHOOK_SECRET/);
  assert.doesNotMatch(readme, /production\.yml.*PAYVERGE_IMAGE_TAG.*latest/);

  const dependencies = read("docs", "CODEMAPS", "dependencies.md");
  assert.match(dependencies, /OpenRouter \(Gemini models\)/);
  assert.doesNotMatch(dependencies, /\| Google Gemini \| AI waiter/);
  assert.doesNotMatch(dependencies, /google\.golang\.org\/genai/);
});

test("entry-point documentation matches release pins and migration startup semantics", () => {
  const nodeVersion = read(".nvmrc").trim();
  const migrationVersions = readdirSync(path.join(ROOT, "backend", "migrations"))
    .map((name) => name.match(/^(\d{6})_.+\.up\.sql$/)?.[1])
    .filter(Boolean)
    .map(Number);
  // 0 means no numbered migration exists: the genesis baseline is the whole
  // schema, and the guides must say the next migration is 000001.
  const migrationHead = migrationVersions.length ? Math.max(...migrationVersions) : 0;
  const headClaim = migrationHead
    ? new RegExp(`HEAD currently ${migrationHead}\\)`)
    : /next (?:migration )?is `000001`/;

  const agents = read("AGENTS.md");
  assert.match(agents, new RegExp(`\\.nvmrc.*Node version pin \\(${nodeVersion.replaceAll(".", "\\.")}\\)`));
  assert.match(agents.match(/^- \*\*Migration Files\*\*.*$/m)?.[0] ?? "", headClaim);
  assert.doesNotMatch(agents, /main\.go.*~\d+ lines/);

  const claude = read("CLAUDE.md");
  assert.match(claude.match(/^- \*\*Migrations \/ schema ownership\*\*.*$/m)?.[0] ?? "", headClaim);
  // The language target and toolchain come from backend/go.mod, so a Go bump
  // that forgets the guide fails here.
  const goMod = read("backend/go.mod");
  const goLang = goMod.match(/^go (\d+\.\d+)(?:\.\d+)?$/m)?.[1];
  const goToolchain = goMod.match(/^toolchain go(\d+\.\d+\.\d+)$/m)?.[1];
  assert.ok(goLang && goToolchain, "backend/go.mod must declare go and toolchain");
  const esc = (v) => v.replaceAll(".", "\\.");
  assert.match(
    claude,
    new RegExp(`Go ${esc(goLang)} language target \\(\`go [^\`]+\` in backend/go\\.mod\\); repository and CI use Go ${esc(goToolchain)}\\.`),
  );
  assert.match(agents, new RegExp(`\\*\\*Language\\*\\*: Go ${esc(goLang)} .*pinned to Go ${esc(goToolchain)}`));
  assert.match(claude, /`log` writes to the backend log and is the default with nothing configured; `smtp`; or the API providers Resend and Postmark/);
  assert.doesNotMatch(claude, /~1700 lines|Go 1\.25|Notifications\*\*: Postmark/);

  for (const relative of [
    "docs/ONBOARDING.md",
    "docs/CODEMAPS/backend.md",
    "docs/CODEMAPS/data.md",
  ]) {
    const source = read(...relative.split("/"));
    assert.doesNotMatch(source, /Fatalf.*commented out|failure logs but does(?:n't| not) abort/);
  }

  const onboarding = read("docs", "ONBOARDING.md");
  assert.match(onboarding, headClaim);
  assert.match(onboarding, /SQL migration failure aborts startup/);
  assert.match(onboarding, /RegisterPluginInitializer\("<name>", func/);
  assert.doesNotMatch(onboarding, /despite the dep/);

  const dataMap = read("docs", "CODEMAPS", "data.md");
  assert.match(dataMap, new RegExp(`\\*\\*${migrationHead} migrations\\*\\* currently`));
  assert.match(dataMap, /Main `database\.RunMigrations` failure aborts startup/);

  const backendMap = read("docs", "CODEMAPS", "backend.md");
  assert.match(backendMap, /Main SQL migration failure aborts startup/);
  assert.match(backendMap, /RegisterPluginInitializer\("name", func/);

  const dependencies = read("docs", "CODEMAPS", "dependencies.md");
  assert.match(dependencies, /`next-intl` is not a direct dependency/);
  assert.doesNotMatch(dependencies, /`next-intl` is installed/);
});

test("required runbooks and CI hygiene hooks cannot disappear", () => {
  const required = [
    "docs/runbooks/credential-exposure-response.md",
    "docs/runbooks/migration-dirty-recovery.md",
    "docs/runbooks/runtime-launch-controls.md",
  ];
  for (const relative of required) {
    assert.equal(existsSync(path.join(ROOT, relative)), true, relative);
  }
  const ci = read(".github", "workflows", "ci.yml");
  assert.match(ci, /node --test scripts\/ci\/d10_hygiene_contract\.test\.mjs/);
  assert.match(ci, /scripts\/update-cloudflare-cidrs\.test\.ts/);
});

test("credential exposure response requires rotation before history remediation", () => {
  const runbook = read("docs", "runbooks", "credential-exposure-response.md");
  assert.match(runbook, /REGISTRATION_MODE=closed/);
  assert.doesNotMatch(runbook, /payverge_backup_|Current known finding/);
  assert.match(runbook, /rotate[^\n]+before[^\n]+history rewrite/i);
  assert.match(runbook, /superseded credential[^\n]+fails/i);
  assert.match(runbook, /git filter-repo/);
  assert.match(runbook, /scan-secrets\.mjs --history/);
  assert.match(runbook, /Never record[^\n]+secret/i);
});

test("docs workspace is plain Markdown with no Docusaurus build step", () => {
  // Source of truth: docs/ is a plain Markdown tree. There is no docs/package.json,
  // no Docusaurus site, and no npm start/build for documentation.
  assert.equal(
    existsSync(path.join(ROOT, "docs", "package.json")),
    false,
    "docs/package.json must not exist — docs are plain Markdown, not a Node package",
  );

  const entryPoints = ["AGENTS.md", "README.md", "docs/ONBOARDING.md", "CLAUDE.md"];
  for (const relative of entryPoints) {
    const source = read(...relative.split("/"));
    assert.doesNotMatch(
      source,
      /Docusaurus/,
      `${relative} must not claim docs are a Docusaurus site`,
    );
    assert.doesNotMatch(
      source,
      /cd docs\s*&&\s*npm (start|install|run build)/,
      `${relative} must not instruct a docs npm build/start`,
    );
  }

  // Positive contract: entry docs state plain Markdown and no build step.
  const agents = read("AGENTS.md");
  assert.match(agents, /plain Markdown/i);
  assert.match(agents, /no build step/i);

  const claude = read("CLAUDE.md");
  assert.match(claude, /Plain Markdown tree \(no build step, no package\.json\)/);

  const onboarding = read("docs", "ONBOARDING.md");
  assert.match(onboarding, /plain Markdown/i);

  const readme = read("README.md");
  assert.match(readme, /plain Markdown|Markdown tree/i);
});

// The public repository is published from this tree. Private operations,
// review, sales and agent-tooling material must never be tracked again.
const trackedFiles = () =>
  execFileSync("git", ["ls-files", "-z"], { cwd: ROOT, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 })
    .split("\0")
    .filter(Boolean);

test("public tree tracks no private, production-only or agent-tooling paths", () => {
  const deniedPrefixes = [
    "reviews/",
    ".grok/",
    ".agents/",
    ".design-sync/",
    "stdevMac/",
    "docs/audit/",
    "docs/audits/",
    "docs/launch-evidence/",
    "docs/legal/",
    "docs/payverge-audit/",
    "docs/pitch/",
    "docs/qa/",
    "docs/sales/",
    "docs/security/",
    "docs/operations/",
    "scripts/email/",
    "scripts/monitoring/",
    "scripts/performance/",
    "scripts/production/",
    "scripts/prod-local/",
    "scripts/qa/",
    "scripts/seo/",
    "scripts/support/",
    "backend/perf/ci/",
    "infra/cloudflare/",
    "infra/email/",
    "infra/monitoring/",
    "infra/seo/",
    "infra/status/",
    "infra/storage/",
    "infra/support/",
  ];
  const deniedFiles = new Set([
    ".emdash.json",
    "info.md",
    "Caddyfile",
    "docker-compose.production.yml",
    "docker-compose.prod-local.yml",
    "docker-compose.digest-deploy.yml",
    "docker-compose.release-smoke.yml",
    "frontend/playwright.release-smoke.config.ts",
    "scripts/seed-showcase-prod.sh",
    "scripts/validate-production-config.mjs",
    "scripts/hooks/commit-msg",
    ".github/workflows/aws-lite-release.yml",
    ".github/workflows/backup-freshness.yml",
    ".github/workflows/backup-restore-drill.yml",
    ".github/workflows/cloudflare-cidr-refresh.yml",
    ".github/workflows/payment-sandbox.yml",
    ".github/workflows/perf-bench.yml",
    ".github/workflows/perf-drift.yml",
    ".github/workflows/production-synthetics.yml",
    ".github/workflows/release-images.yml",
    ".github/workflows/seo-edge-contract.yml",
    ".github/workflows/seo-search-submission.yml",
    ".github/workflows/status-incident-drill.yml",
  ]);
  // .claude/ is agent tooling and stays private, except the shared project
  // skills: plain-Markdown SKILL.md playbooks under .claude/skills/<name>/.
  const isPrivateClaudePath = (file) =>
    file.startsWith(".claude/") && !/^\.claude\/skills\/[a-z0-9-]+\/SKILL\.md$/.test(file);
  const leaked = trackedFiles().filter(
    (file) =>
      deniedFiles.has(file) ||
      deniedPrefixes.some((prefix) => file.startsWith(prefix)) ||
      isPrivateClaudePath(file),
  );
  assert.deepEqual(leaked, [], "private material must stay out of the public tree");
});

// Private planning trees may live in the private repository, but must never
// reach the exported public tree. The check runs on the export view: in the
// private repository (its drop list is tracked) the tracked files minus what
// tools/oss-export drops; in the public tree, the tracked files as they are.
const PRIVATE_DROP_LIST = "docs/superpowers/oss-export/drop.txt";
const EXPORT_DENIED = [
  // docs/superpowers at any depth (frontend/docs/superpowers/ too), any case.
  /(^|\/)docs\/superpowers\//i,
  /^docs\/plans\//,
  // Private backlog, sales roadmap, a live-dashboard audit and a captured
  // fiscal page snapshot: kept in the private repository only.
  /^docs\/improvements\//,
  /^docs\/product\/improvement-paths-and-killer-features\.md$/,
  /^docs\/design\/sidebar-gap-list\.md$/,
  /^docs\/fiscal\/payverge-fiscal-snapshot\.md$/,
];
const exportView = (files, dropText) => {
  if (dropText === undefined) return files;
  const entries = withBuiltinDrops(parseDropList(dropText));
  return files.filter((file) => !entries.some((entry) => entry.match(file)));
};
const exportLeaks = (files, dropText) =>
  exportView(files, dropText).filter((file) => EXPORT_DENIED.some((re) => re.test(file)));

test("the export guard flags private planning trees", () => {
  // Public tree (no drop list): a planning file is a leak wherever it sits.
  for (const file of [
    "docs/superpowers/plans/x.md",
    "frontend/docs/superpowers/plans/x.md",
    "Docs/SuperPowers/specs/x.md",
    "docs/plans/2026-01-01-x.md",
    "docs/improvements/README.md",
    "docs/product/improvement-paths-and-killer-features.md",
    "docs/design/sidebar-gap-list.md",
    "docs/fiscal/payverge-fiscal-snapshot.md",
  ]) {
    assert.deepEqual(exportLeaks([file, "docs/ONBOARDING.md"], undefined), [file], file);
  }
  // Private repository: docs/superpowers is always dropped by the exporter,
  // but docs/plans/ ships unless drop.txt removes it.
  const privateFiles = [PRIVATE_DROP_LIST, "frontend/docs/superpowers/a.md", "docs/plans/p.md"];
  assert.deepEqual(exportLeaks(privateFiles, "reviews\n"), ["docs/plans/p.md"]);
  assert.deepEqual(exportLeaks(privateFiles, "docs/plans\n"), []);
});

test("shipped guides never route new files into a denied planning tree", () => {
  // A contributor following a "where it goes" table must not be sent to a
  // path the export guard rejects: that file would fail this contract.
  const isDenied = (target) => {
    const probe = target.endsWith("/") ? `${target}x.md` : target;
    return EXPORT_DENIED.some((re) => re.test(probe));
  };
  const guides = ["AGENTS.md", "CLAUDE.md", "README.md", "docs/ONBOARDING.md", "docs/BACKLOG.md", ".github/CONTRIBUTING.md"];
  const routed = [];
  for (const relative of guides) {
    if (!existsSync(path.join(ROOT, relative))) continue;
    for (const line of read(...relative.split("/")).split("\n")) {
      const row = /^\|[^|]*\|\s*`([^`]+)`/.exec(line);
      if (row && isDenied(row[1])) routed.push(`${relative}: ${line.trim()}`);
      const verb = /\b(?:in|into|under|to)\s+`([^`]+)`/g;
      for (const match of line.matchAll(verb)) {
        if (isDenied(match[1]) && !/not exported|private/i.test(line)) routed.push(`${relative}: ${line.trim()}`);
      }
    }
  }
  assert.deepEqual(routed, [], "guides must not direct new files into a tree the export denies");
  assert.equal(isDenied("docs/plans/"), true, "probe self-check");

  // Benchmark logs ship publicly: they must not cite private planning paths.
  for (const relative of ["summary.md", "backend/summary.md"]) {
    assert.doesNotMatch(read(...relative.split("/")), /docs\/superpowers\//, `${relative} cites a private plan path`);
  }
});

test("the export tree carries no private planning trees", () => {
  const files = trackedFiles();
  const dropText = files.includes(PRIVATE_DROP_LIST) ? read(...PRIVATE_DROP_LIST.split("/")) : undefined;
  assert.deepEqual(exportLeaks(files, dropText), [], "planning notes must not reach the public tree");
});

test("only .claude/skills/ is shareable under .claude/", () => {
  const ignored = (relative) => {
    try {
      execFileSync("git", ["check-ignore", "--no-index", "-q", relative], { cwd: ROOT });
      return true;
    } catch (error) {
      if (error.status === 1) return false;
      throw error;
    }
  };
  assert.equal(ignored(".claude/skills/example/SKILL.md"), false, "project skills must be trackable");
  for (const relative of [
    ".claude/settings.json",
    ".claude/settings.local.json",
    ".claude/commands/example.md",
    ".claude/worktrees/example/README.md",
    "frontend/.claude/settings.json",
    // Only <name>/SKILL.md is shared: nothing else under .claude/skills/.
    ".claude/skills/README.md",
    ".claude/skills/example/notes.md",
    ".claude/skills/example/.env",
    ".claude/skills/example/scripts/run.sh",
    ".claude/skills/example/references/SKILL.md",
  ]) {
    assert.equal(ignored(relative), true, `${relative} must stay ignored`);
  }
});

test("secrets, keys and backup output stay ignored; templates stay trackable", () => {
  const ignored = (relative) => {
    try {
      execFileSync("git", ["check-ignore", "--no-index", "-q", relative], { cwd: ROOT });
      return true;
    } catch (error) {
      if (error.status === 1) return false;
      throw error;
    }
  };
  for (const relative of [
    ".env",
    ".env.production",
    "deploy/.env.local",
    "backend/.env-staging",
    "frontend/.env_local",
    "prod.env",
    ".envrc",
    "tls/server.pem",
    "tls/server.key",
    "certs/keystore.p12",
    "backups/payverge_20260101.sql",
    "deploy/backups/latest.dump",
    "var/backup/.last-success",
    "var/backup/.last-upload-success",
  ]) {
    assert.equal(ignored(relative), true, `${relative} must stay ignored`);
  }
  for (const relative of [".env.example", "deploy/.env.example", "backend/perf/staging/.env.example"]) {
    assert.equal(ignored(relative), false, `${relative} must stay trackable`);
  }
  // check-ignore exits 1 when no path is ignored: that is the passing case.
  let trackedButIgnored = "";
  try {
    trackedButIgnored = execFileSync("git", ["check-ignore", "--no-index", "--stdin"], {
      cwd: ROOT,
      input: trackedFiles().join("\n"),
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    }).trim();
  } catch (error) {
    if (error.status !== 1) throw error;
  }
  assert.equal(trackedButIgnored, "", "a tracked file must not match .gitignore");
});

test("repository root is a closed set", () => {
  // Extend this list deliberately when a new root file is genuinely needed;
  // everything else has a home under docs/, scripts/ or a workspace.
  const allowed = new Set([
    ".env.example",
    ".gitignore",
    ".nvmrc",
    "AGENTS.md",
    "CLAUDE.md",
    "LICENSE",
    "Makefile",
    "NOTICE",
    "README.md",
    "TRADEMARKS.md",
    "docker-compose.yml",
    "package.json",
    "promptfooconfig.yaml",
    "summary.md",
  ]);
  const unexpected = trackedFiles()
    .filter((file) => !file.includes("/"))
    .filter((file) => !allowed.has(file));
  assert.deepEqual(unexpected, [], "new root files need a deliberate allow-list entry");
});

// GitHub Actions. The Go module in scripts/ci/workflowcontract parses the
// workflows and checks them in depth; these text checks need no dependencies,
// so the contract job reports them even when that module does not build.
const WORKFLOWS = ["acceptance.yml", "ci.yml", "codeql.yml", "e2e.yml", "release.yml", "scorecard.yml"];
const workflowLines = (name) =>
  read(".github", "workflows", name)
    .split("\n")
    .map((line, index) => ({ line, number: index + 1 }));

test("the public workflow set is exact", () => {
  const present = readdirSync(path.join(ROOT, ".github", "workflows")).sort();
  assert.deepEqual(present, WORKFLOWS, "add or remove a workflow deliberately, here and in workflowcontract");
  const tracked = trackedFiles()
    .filter((file) => file.startsWith(".github/workflows/"))
    .map((file) => file.slice(".github/workflows/".length))
    .sort();
  // Untracked files are fine while a change is being written; once tracked,
  // nothing outside the set may ship.
  assert.deepEqual(
    tracked.filter((name) => !WORKFLOWS.includes(name)),
    [],
    "tracked workflow outside the public set",
  );
});

test("every workflow action is pinned to a full commit SHA with a version comment", () => {
  const pinned = /^\s*(?:-\s+)?uses:\s*[\w.-]+\/[\w./-]+@[0-9a-f]{40}\s+#\s*v\d+\.\d+\.\d+\s*$/;
  const unpinned = [];
  for (const name of WORKFLOWS) {
    for (const { line, number } of workflowLines(name)) {
      if (!/^\s*(?:-\s+)?uses:/.test(line)) continue;
      if (/uses:\s*\.\//.test(line)) continue; // local reusable workflow or action
      if (!pinned.test(line)) unpinned.push(`${name}:${number}: ${line.trim()}`);
    }
  }
  assert.deepEqual(unpinned, []);
  // Service and docker run images are pinned by digest too.
  for (const name of WORKFLOWS) {
    for (const { line, number } of workflowLines(name)) {
      const image = line.match(/^\s*image:\s*([\w./-]+:[^\s#]+)/); // skips matrix `image: [..]`
      if (image) assert.match(image[1], /@sha256:[0-9a-f]{64}$/, `${name}:${number}`);
    }
  }
});

test("workflows default to a read-only token and never run fork code with secrets", () => {
  for (const name of WORKFLOWS) {
    const source = read(".github", "workflows", name);
    assert.match(source, /^permissions:\n {2}contents: read\n(?!\s{2}\S)/m, `${name}: top-level permissions must be exactly contents: read`);
    assert.doesNotMatch(source, /^\s*(pull_request_target|workflow_run):/m, `${name}: forbidden trigger`);
    assert.doesNotMatch(source, /permissions:\s*write-all/, name);
    assert.doesNotMatch(source, /runs-on:\s*\[?\s*self-hosted/, `${name}: self-hosted runners`);
    for (const { line, number } of workflowLines(name)) {
      if (/^\s+[a-z-]+:\s*write\b/.test(line)) {
        assert.match(line, /#\s*\S/, `${name}:${number}: write permission needs an inline reason`);
      }
    }
  }
});

test("release-please configuration lives under .github/", () => {
  const config = JSON.parse(read(".github", "release-please-config.json"));
  const manifest = JSON.parse(read(".github", ".release-please-manifest.json"));
  assert.equal(config["include-v-in-tag"], true);
  assert.ok(config.packages["."], "the repository root is the one released package");
  assert.match(config["changelog-path"], /^docs\//, "the changelog stays out of the closed root");
  assert.match(manifest["."], /^\d+\.\d+\.\d+$/);
  for (const stray of ["release-please-config.json", ".release-please-manifest.json", "CHANGELOG.md"]) {
    assert.equal(existsSync(path.join(ROOT, stray)), false, `${stray} must not be at the repository root`);
  }
  const release = read(".github", "workflows", "release.yml");
  assert.match(release, /config-file: \.github\/release-please-config\.json/);
  assert.match(release, /manifest-file: \.github\/\.release-please-manifest\.json/);
});

// Every sibling workstream has merged, so every path a workflow runs must
// exist. Add an entry here only for a file a not-yet-merged branch delivers.
const SIBLING_PATHS = new Set([]);

test("every repository path a workflow runs exists or is a declared sibling path", () => {
  const candidate = /(?:^|[\s"'(=<])((?:\.\/)?(?:scripts|backend\/scripts|backend\/perf|frontend\/scripts|tools|\.github)\/[\w./-]+\.(?:sh|mjs|js|ts|sql|json|toml))\b/g;
  const missing = [];
  for (const name of WORKFLOWS) {
    for (const { line, number } of workflowLines(name)) {
      if (/^\s*#/.test(line)) continue;
      for (const match of line.matchAll(candidate)) {
        const relative = match[1].replace(/^\.\//, "");
        if (relative.includes("*")) continue;
        // Paths in steps with working-directory: frontend are relative to it.
        const exists =
          existsSync(path.join(ROOT, relative)) || existsSync(path.join(ROOT, "frontend", relative));
        if (!exists && !SIBLING_PATHS.has(relative)) missing.push(`${name}:${number}: ${relative}`);
      }
    }
  }
  assert.deepEqual(missing, []);
});


test("container images ship the root LICENSE and NOTICE with OCI labels (D-8)", () => {
  for (const image of ["backend", "frontend"]) {
    for (const file of ["LICENSE", "NOTICE"]) {
      assert.equal(read(image, file), read(file), `${image}/${file} must be a byte-identical copy of the root ${file}`);
    }
    // NOTICE points at LICENSES/ for verbatim texts, and each image ships its
    // own LICENSES/ copy, so the copies must match the root set exactly.
    const rootLicences = readdirSync(path.join(ROOT, "LICENSES")).sort();
    assert.deepEqual(readdirSync(path.join(ROOT, image, "LICENSES")).sort(), rootLicences, `${image}/LICENSES must hold the root LICENSES/ files`);
    for (const file of rootLicences) {
      assert.equal(read(image, "LICENSES", file), read("LICENSES", file), `${image}/LICENSES/${file} must be a byte-identical copy`);
    }
    const dockerfile = read(image, "Dockerfile");
    assert.match(dockerfile, /^COPY (--chown=\S+ )?LICENSE NOTICE \/usr\/share\/doc\/payverge\/$/m, `${image}/Dockerfile must copy LICENSE and NOTICE`);
    assert.match(dockerfile, /org\.opencontainers\.image\.licenses="Apache-2\.0 AND LGPL-3\.0-or-later"/, `${image}/Dockerfile licenses label`);
    assert.match(dockerfile, /org\.opencontainers\.image\.source="https:\/\/github\.com\/stdevMac\/payverge"/, `${image}/Dockerfile source label`);
  }
});
