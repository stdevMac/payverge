#!/usr/bin/env node
// Inventory of the environment variables Payverge reads at runtime, taken
// from the code rather than from memory. docs/self-hosting/configuration.md
// must document every name this prints (scripts/docs/env-inventory.test.mjs).
//
// Usage, from the repository root:
//   node scripts/docs/env-inventory.mjs           # NAME<TAB>sources
//   node scripts/docs/env-inventory.mjs --json    # [{ name, sources: [...] }]
//   node scripts/docs/env-inventory.mjs --flags   # server command-line flags
//
// Sources scanned (test files excluded):
//   backend   Go: os.Getenv/LookupEnv("X"), env helpers (intEnv, envOr, ...)
//             called with a literal, string constants whose identifier names
//             an env var (fooEnv = "X", EnvFoo = "X"), and []string key lists
//             assigned to an identifier containing "env" (publicURLEnvKeys).
//   frontend  process.env.X / process.env["X"], env.X / env["X"] on an
//             injected env object, and the alias table in
//             src/config/publicConfig.ts.
//   deploy    ${X} interpolations in deploy/docker-compose.yml and env
//             reads in deploy/install.sh (operator-facing, read by compose).
//
// The scan is deliberately lexical: it over-reports rather than misses. A
// name that is not really an operator setting belongs on the commented
// allow-list in the test, not in a cleverer parser.

import { readFileSync, readdirSync, statSync, existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

const NAME = "[A-Z][A-Z0-9_]*[A-Z0-9]";

function walk(dir, accept, skipDirs = new Set()) {
  const out = [];
  if (!existsSync(dir)) return out;
  for (const entry of readdirSync(dir)) {
    const abs = path.join(dir, entry);
    const st = statSync(abs);
    if (st.isDirectory()) {
      if (skipDirs.has(entry) || entry.startsWith(".")) continue;
      out.push(...walk(abs, accept, skipDirs));
    } else if (accept(abs)) {
      out.push(abs);
    }
  }
  return out;
}

function lineOf(text, index) {
  let n = 1;
  for (let i = 0; i < index; i++) if (text.charCodeAt(i) === 10) n++;
  return n;
}

function collect(found, file, text, re, group = 1, area) {
  for (const m of text.matchAll(re)) {
    const name = m[group];
    if (!name) continue;
    const rel = path.relative(ROOT, file);
    const where = `${area}:${rel}:${lineOf(text, m.index)}`;
    if (!found.has(name)) found.set(name, new Set());
    found.get(name).add(where);
  }
}

// Go directories that are not part of the shipped server binaries: test
// harnesses, benchmarks and code generators.
const GO_SKIP = new Set(["vendor", "testdata", "perf", "tests", "testutil", "mocks", "node_modules"]);

export function scanBackend(found = new Map()) {
  const files = walk(
    path.join(ROOT, "backend"),
    (f) => f.endsWith(".go") && !f.endsWith("_test.go"),
    GO_SKIP,
  );
  const callLiteral = new RegExp(
    String.raw`\b(?:os\.(?:Getenv|LookupEnv)|[A-Za-z_]*(?:[a-z]Env|env)[A-Za-z_]*)\(\s*"(${NAME})"`,
    "g",
  );
  const constLiteral = new RegExp(
    String.raw`\b[A-Za-z_]*(?:Env|env)[A-Za-z0-9_]*\s*(?:string\s*)?(?:=|:=|:)\s*"(${NAME})"`,
    "g",
  );
  // The admin CLI resolves its database settings with pick(flag, "DB_X", ...).
  const pickLiteral = new RegExp(String.raw`\bpick\([^,()]*,\s*"(${NAME})"`, "g");
  const listAssign = /\b[A-Za-z_]*(?:Env|env)[A-Za-z0-9_]*\s*(?:=|:=|:)\s*\[\]string\{([^}]*)\}/g;
  const literal = new RegExp(String.raw`"(${NAME})"`, "g");
  for (const file of files) {
    const text = readFileSync(file, "utf8");
    // Setenv / Unsetenv (os., t.) set a variable, they do not read it.
    const reads = text.replace(/\b(?:\w+\.)?(?:Setenv|Unsetenv)\([^)]*\)/g, "");
    collect(found, file, reads, callLiteral, 1, "backend");
    collect(found, file, reads, constLiteral, 1, "backend");
    collect(found, file, reads, pickLiteral, 1, "backend");
    for (const m of reads.matchAll(listAssign)) {
      const offset = m.index;
      for (const lit of m[1].matchAll(literal)) {
        const rel = path.relative(ROOT, file);
        const where = `backend:${rel}:${lineOf(reads, offset)}`;
        if (!found.has(lit[1])) found.set(lit[1], new Set());
        found.get(lit[1]).add(where);
      }
    }
  }
  return found;
}

export function scanFrontend(found = new Map()) {
  const fe = path.join(ROOT, "frontend");
  const isSource = (f) =>
    /\.(?:ts|tsx|mjs|js)$/.test(f) && !/\.(?:test|spec)\.[tj]sx?$/.test(f) && !/\.d\.ts$/.test(f);
  const files = [
    ...walk(path.join(fe, "src"), isSource, new Set(["__tests__", "__mocks__", "node_modules"])),
    ...["next.config.mjs", "instrumentation-client.ts", "sentry.server.config.ts", "sentry.edge.config.ts"]
      .map((f) => path.join(fe, f))
      .filter(existsSync),
  ];
  const processEnv = new RegExp(String.raw`process\.env(?:\.(${NAME})|\[\s*["'](${NAME})["']\s*\])`, "g");
  const injectedEnv = new RegExp(String.raw`\benv(?:\.(${NAME})|\[\s*["'](${NAME})["']\s*\])`, "g");
  // const KEY = "X" read through env[KEY], in a file that indexes an env.
  const keyConst = new RegExp(String.raw`\b(?:const|let)\s+\w*(?:KEY|Key|ENV|Env)\w*\s*=\s*["'](${NAME})["']`, "g");
  for (const file of files) {
    // Comments describe reads; they are not reads.
    const text = readFileSync(file, "utf8")
      .replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "))
      .replace(/(^|[^:"'`\\])\/\/.*$/gm, "$1");
    if (/\benv\[\s*[A-Za-z_]/.test(text)) collect(found, file, text, keyConst, 1, "frontend");
    collect(found, file, text, processEnv, 1, "frontend");
    collect(found, file, text, processEnv, 2, "frontend");
    collect(found, file, text, injectedEnv, 1, "frontend");
    collect(found, file, text, injectedEnv, 2, "frontend");
  }
  // The runtime alias table: every name in its arrays is read at request time.
  const publicConfig = path.join(fe, "src", "config", "publicConfig.ts");
  if (existsSync(publicConfig)) {
    const text = readFileSync(publicConfig, "utf8");
    const start = text.indexOf("RUNTIME_ENV_NAMES");
    const end = text.indexOf("\n};", start);
    if (start >= 0 && end > start) {
      const block = text.slice(start, end);
      for (const arr of block.matchAll(/\[([^\]]*)\]/g)) {
        for (const lit of arr[1].matchAll(new RegExp(String.raw`"(${NAME})"`, "g"))) {
          const where = `frontend:${path.relative(ROOT, publicConfig)}:${lineOf(text, start + arr.index)}`;
          if (!found.has(lit[1])) found.set(lit[1], new Set());
          found.get(lit[1]).add(where);
        }
      }
    }
  }
  return found;
}

export function scanDeploy(found = new Map()) {
  const compose = path.join(ROOT, "deploy", "docker-compose.yml");
  if (existsSync(compose)) {
    const text = readFileSync(compose, "utf8").replace(/^\s*#.*$/gm, "");
    collect(found, compose, text, new RegExp(String.raw`\$\{(${NAME})(?=[:?}-])`, "g"), 1, "deploy");
  }
  return found;
}

// Command-line flags of the server binary (backend/cmd/app), for the flag
// table in configuration.md.
export function serverFlags() {
  const dir = path.join(ROOT, "backend", "cmd", "app");
  const files = walk(dir, (f) => f.endsWith(".go") && !f.endsWith("_test.go"));
  const re = /\bflag\.(?:String|Int|Int64|Bool|Duration|Float64|Uint)\(\s*"([a-z0-9][a-z0-9-]*)"/g;
  const names = new Set();
  for (const file of files) {
    for (const m of readFileSync(file, "utf8").matchAll(re)) names.add(m[1]);
  }
  return [...names].sort();
}

export function inventory() {
  const found = new Map();
  scanBackend(found);
  scanFrontend(found);
  scanDeploy(found);
  return [...found.entries()]
    .map(([name, sources]) => ({ name, sources: [...sources].sort() }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const rows = inventory();
  if (process.argv.includes("--flags")) {
    for (const flag of serverFlags()) process.stdout.write(`--${flag}\n`);
  } else if (process.argv.includes("--json")) {
    process.stdout.write(`${JSON.stringify(rows, null, 2)}\n`);
  } else {
    for (const row of rows) {
      const areas = [...new Set(row.sources.map((s) => s.split(":")[0]))].join(",");
      process.stdout.write(`${row.name}\t${areas}\t${row.sources[0]}${row.sources.length > 1 ? ` (+${row.sources.length - 1})` : ""}\n`);
    }
  }
}
