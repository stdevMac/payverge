/**
 * Contract scanner: every production-required backend env var must be
 * forwarded into the backend service by docker-compose.yml and every
 * self-host compose file under deploy/.
 *
 * Source of truth for the required list:
 *   backend/internal/config/production_preflight.go
 *
 * A backend service that loads a mandatory env_file forwards every key of that
 * file, so it passes without listing each variable under environment.
 *
 * Run from repo root:
 *   npx --yes tsx scripts/check-compose-env-contract.ts
 *
 * Or from frontend/:
 *   npm run check:compose-env
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

// ---------------------------------------------------------------------------
// Authoritative production-required env vars
// Source of truth: backend/internal/config/production_preflight.go
// Keep this list in sync when ValidateProduction gains/loses required fields.
// ---------------------------------------------------------------------------
const PRODUCTION_REQUIRED_ENV_VARS: readonly string[] = [
  "JWT_SECRET_KEY",
  "PLUGIN_SECRET_KEY",
  "RPC_URL",
  "EMAIL_PROVIDER",
  "EMAIL_API_KEY",
  "FROM_EMAIL",
  "FROM_EMAIL_UPDATES",
  "S3_BUCKET",
  "AWS_ACCESS_KEY",
  "AWS_SECRET_KEY",
  "S3_PROTECTED_BUCKET",
  "AWS_PROTECTED_ACCESS_KEY",
  "AWS_PROTECTED_SECRET_KEY",
  "ALLOWED_ORIGINS",
  "COOKIE_DOMAIN",
  "TRUSTED_PLATFORM",
  "TRUSTED_PROXIES",
  "AI_DAILY_BUDGET_USD",
];

/** Not required by preflight — never flag these as missing. */
const NOT_REQUIRED_BY_PREFLIGHT = new Set([
  "S3_ENDPOINT",
  "S3_PUBLIC_BASE_URL",
  "S3_PROTECTED_ENDPOINT",
]);


const scriptDir =
  typeof __dirname !== "undefined"
    ? __dirname
    : path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(scriptDir, "..");

/**
 * The local stack plus every self-host compose file under deploy/ (one or two
 * levels deep). Files without a backend service are skipped by the checker.
 */
export function composeFiles(root: string = REPO_ROOT): string[] {
  const files = ["docker-compose.yml"];
  const walk = (relDir: string, depth: number): void => {
    const absDir = path.join(root, relDir);
    if (!fs.existsSync(absDir)) return;
    for (const entry of fs.readdirSync(absDir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const rel = path.posix.join(relDir, entry.name);
      if (entry.isDirectory() && depth > 0) walk(rel, depth - 1);
      else if (entry.isFile() && /\.ya?ml$/.test(entry.name)) files.push(rel);
    }
  };
  walk("deploy", 1);
  return files;
}

type YamlLoadFn = (input: string) => unknown;

export function tryLoadYamlParser(): YamlLoadFn | null {
  const frontendPkg = path.join(REPO_ROOT, "frontend", "package.json");
  const requireFromFrontend = createRequire(frontendPkg);

  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const yaml = requireFromFrontend("yaml") as {
      parse?: YamlLoadFn;
      default?: { parse?: YamlLoadFn };
    };
    if (typeof yaml.parse === "function") return yaml.parse.bind(yaml);
    if (typeof yaml.default?.parse === "function") {
      return yaml.default.parse.bind(yaml.default);
    }
  } catch {
    // fall through
  }

  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const jsyaml = requireFromFrontend("js-yaml") as {
      load?: YamlLoadFn;
      default?: { load?: YamlLoadFn };
    };
    if (typeof jsyaml.load === "function") return jsyaml.load.bind(jsyaml);
    if (typeof jsyaml.default?.load === "function") {
      return jsyaml.default.load.bind(jsyaml.default);
    }
  } catch {
    // fall through
  }

  return null;
}

type Section = "environment" | "command" | "entrypoint" | "args" | "env_file";
const SECTION_RE = /^(environment|command|entrypoint|args|env_file)\s*:/;

/**
 * Minimal fallback: extract backend service environment, env_file and
 * command/entrypoint/args from a compose YAML without a full parser. Handles
 * list-form env and command, and scalar or list-form env_file.
 */
function minimalParseBackendService(content: string): {
  environment: unknown;
  command: unknown;
  entrypoint: unknown;
  args: unknown;
  env_file: unknown;
} {
  const lines = content.split(/\r?\n/);
  let inServices = false;
  let inBackend = false;
  let backendIndent = -1;
  let section: "none" | Section = "none";
  let sectionIndent = -1;

  const environment: string[] = [];
  const command: string[] = [];
  const entrypoint: string[] = [];
  const args: string[] = [];
  const envFile: string[] = [];
  const unquote = (value: string): string => value.replace(/^['"]|['"]$/g, "");

  for (const line of lines) {
    if (/^services\s*:/.test(line)) {
      inServices = true;
      continue;
    }
    if (!inServices) continue;

    const indent = line.match(/^(\s*)/)?.[1].length ?? 0;
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    // Top-level key under services (e.g. "  backend:")
    if (inServices && indent === 2 && /^[a-zA-Z0-9_-]+\s*:/.test(trimmed)) {
      if (/^backend\s*:/.test(trimmed)) {
        inBackend = true;
        backendIndent = indent;
        section = "none";
      } else if (inBackend && indent <= backendIndent) {
        inBackend = false;
        section = "none";
      }
      continue;
    }

    if (!inBackend) continue;

    // Leaving backend for a peer service or top-level key
    if (indent <= backendIndent && /^[a-zA-Z0-9_-]+\s*:/.test(trimmed)) {
      inBackend = false;
      section = "none";
      continue;
    }

    // Section headers under backend
    if (indent === backendIndent + 2 && SECTION_RE.test(trimmed)) {
      section = trimmed.split(":")[0].trim() as Section;
      sectionIndent = indent;
      // Inline map form "environment: { KEY: val }" is rare; ignore value side.
      // Scalar env_file ("env_file: .env") is common, so keep that value.
      const inline = trimmed.slice(trimmed.indexOf(":") + 1).trim();
      if (section === "env_file" && inline && !inline.startsWith("#")) {
        envFile.push(unquote(inline));
      }
      continue;
    }

    if (section === "none") continue;

    // Exited current section
    if (indent <= sectionIndent && /^[a-zA-Z0-9_-]+\s*:/.test(trimmed)) {
      if (SECTION_RE.test(trimmed)) {
        section = trimmed.split(":")[0].trim() as Section;
        sectionIndent = indent;
        const inline = trimmed.slice(trimmed.indexOf(":") + 1).trim();
        if (section === "env_file" && inline && !inline.startsWith("#")) {
          envFile.push(unquote(inline));
        }
      } else {
        section = "none";
      }
      continue;
    }

    // List items
    if (trimmed.startsWith("- ") && indent > sectionIndent) {
      const item = trimmed.slice(2).trim();
      // Strip surrounding quotes
      const unquoted = unquote(item);
      if (section === "environment") environment.push(unquoted);
      else if (section === "command") command.push(unquoted);
      else if (section === "entrypoint") entrypoint.push(unquoted);
      else if (section === "args") args.push(unquoted);
      // Long-form entries ("- path: .env") need the YAML parser; skipping
      // them here only makes the fallback stricter.
      else if (section === "env_file" && !/^[A-Za-z_]+\s*:/.test(unquoted)) envFile.push(unquoted);
    }
  }

  return {
    environment,
    command,
    entrypoint,
    args,
    env_file: envFile.length > 0 ? envFile : undefined,
  };
}

function collectEnvKeysFromEnvironment(environment: unknown): Set<string> {
  const keys = new Set<string>();
  if (environment == null) return keys;

  if (Array.isArray(environment)) {
    for (const item of environment) {
      if (typeof item !== "string") continue;
      const s = item.trim();
      // Proper KEY=value (or KEY only). Bare ${VAR:-} has no KEY= and does NOT count.
      const eq = s.indexOf("=");
      if (eq === -1) {
        // Docker allows "- VARNAME" to pass through host env; that still names the key.
        // Bare expansion values like "${S3_BUCKET:-}" must not count.
        if (/^\$\{/.test(s)) continue;
        if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(s)) keys.add(s);
        continue;
      }
      const key = s.slice(0, eq).trim();
      if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) keys.add(key);
    }
    return keys;
  }

  if (typeof environment === "object") {
    for (const key of Object.keys(environment as Record<string, unknown>)) {
      if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) keys.add(key);
    }
  }
  return keys;
}

const INTERPOLATION_RE = /\$\{([A-Za-z_][A-Za-z0-9_]*)[^}]*\}/g;

function collectVarsFromStringBlocks(blocks: unknown[]): Set<string> {
  const vars = new Set<string>();
  for (const block of blocks) {
    if (block == null) continue;
    const strings: string[] = [];
    if (typeof block === "string") {
      strings.push(block);
    } else if (Array.isArray(block)) {
      for (const item of block) {
        if (typeof item === "string") strings.push(item);
        else if (item != null && typeof item === "object") {
          // rare map form
          strings.push(JSON.stringify(item));
        }
      }
    } else if (typeof block === "object") {
      strings.push(JSON.stringify(block));
    }
    for (const s of strings) {
      INTERPOLATION_RE.lastIndex = 0;
      let m: RegExpExecArray | null;
      while ((m = INTERPOLATION_RE.exec(s)) !== null) {
        vars.add(m[1]);
      }
    }
  }
  return vars;
}

function extractBackendService(doc: unknown): Record<string, unknown> | null {
  if (!doc || typeof doc !== "object") return null;
  const services = (doc as Record<string, unknown>).services;
  if (!services || typeof services !== "object") return null;
  const backend = (services as Record<string, unknown>).backend;
  if (!backend || typeof backend !== "object") return null;
  return backend as Record<string, unknown>;
}

/**
 * True when the backend service loads a mandatory env_file. Compose hands every
 * key of that file to the container, so whatever the operator sets there is
 * forwarded and no required variable can be silently dropped. Entries marked
 * `required: false` do not count: when the file is absent nothing is passed.
 */
export function forwardsEnvFile(envFile: unknown): boolean {
  const entries = Array.isArray(envFile) ? envFile : [envFile];
  return entries.some((entry) => {
    if (typeof entry === "string") return entry.trim() !== "";
    if (!entry || typeof entry !== "object") return false;
    const { path: file, required } = entry as { path?: unknown; required?: unknown };
    return typeof file === "string" && file.trim() !== "" && required !== false;
  });
}

function requiredForService(forwarded: Set<string>): string[] {
  const required: string[] = [];
  for (const v of PRODUCTION_REQUIRED_ENV_VARS) {
    if (NOT_REQUIRED_BY_PREFLIGHT.has(v)) continue;
    required.push(v);
  }
  return required;
}

function isSatisfied(varName: string, forwarded: Set<string>): boolean {
  return forwarded.has(varName);
}

type BackendService = {
  environment?: unknown;
  command?: unknown;
  entrypoint?: unknown;
  args?: unknown;
  env_file?: unknown;
};

/**
 * A deploy/ file named docker-compose.<variant>.yml is an overlay: it is only ever used
 * as a later -f file on top of a base docker-compose.yml (deploy/README.md,
 * the COMPOSE_FILE examples in each overlay's header). Returns the base it is
 * merged onto: the nearest docker-compose.yml from the overlay's directory up
 * to deploy/. Returns null for a base file, or for an overlay-named file with
 * no base, which is then checked on its own.
 */
export function overlayBase(root: string, rel: string): string | null {
  if (!/^deploy\/(?:.+\/)?docker-compose\.[^/.]+\.ya?ml$/.test(rel)) return null;
  let dir = path.posix.dirname(rel);
  for (;;) {
    for (const name of ["docker-compose.yml", "docker-compose.yaml"]) {
      const candidate = path.posix.join(dir, name);
      if (fs.existsSync(path.join(root, candidate))) return candidate;
    }
    if (dir === "deploy" || dir === "." || dir === "") return null;
    dir = path.posix.dirname(dir);
  }
}

function loadBackendService(composePath: string, yamlLoad: YamlLoadFn | null): BackendService | null {
  const content = fs.readFileSync(composePath, "utf8");
  if (!yamlLoad) return minimalParseBackendService(content);
  try {
    return extractBackendService(yamlLoad(content));
  } catch (err) {
    console.error(
      `WARN: YAML parse failed for ${composePath}, using minimal parser:`,
      err instanceof Error ? err.message : err,
    );
    return minimalParseBackendService(content);
  }
}

/**
 * What compose forwards to the backend once the overlay is merged onto its
 * base: environment merges key by key and env_file lists append, so either
 * file can forward a variable; command, entrypoint and args are replaced by
 * the overlay when it sets them.
 */
function mergedForwarding(base: BackendService | null, overlay: BackendService): {
  forwarded: Set<string>;
  envFile: boolean;
} {
  const pick = (key: "command" | "entrypoint" | "args"): unknown =>
    overlay[key] !== undefined ? overlay[key] : base?.[key];
  const forwarded = new Set([
    ...collectEnvKeysFromEnvironment(base?.environment),
    ...collectEnvKeysFromEnvironment(overlay.environment),
    ...collectVarsFromStringBlocks([pick("command"), pick("entrypoint"), pick("args")]),
  ]);
  return {
    forwarded,
    envFile: forwardsEnvFile(base?.env_file) || forwardsEnvFile(overlay.env_file),
  };
}

function checkComposeFile(
  root: string,
  rel: string,
  yamlLoad: YamlLoadFn | null,
): { missing: string[]; ok: boolean; skipped: boolean; mergedWith?: string } {
  const composePath = path.join(root, rel);
  if (!fs.existsSync(composePath)) {
    return { missing: [], ok: true, skipped: true };
  }

  const backend = loadBackendService(composePath, yamlLoad);
  if (!backend) {
    // No backend service — nothing to check for this file.
    return { missing: [], ok: true, skipped: true };
  }

  // An overlay (deploy/docker-compose.build.yml, deploy/demo/docker-compose.demo.yml)
  // never runs alone, so it is checked as compose sees it: merged onto its base.
  // Overriding a forwarded variable with an empty value still forwards it.
  const baseRel = overlayBase(root, rel);
  const base = baseRel ? loadBackendService(path.join(root, baseRel), yamlLoad) : null;
  const { forwarded, envFile } = mergedForwarding(base, backend);
  const mergedWith = baseRel ? { mergedWith: baseRel } : {};

  if (envFile) {
    return { missing: [], ok: true, skipped: false, ...mergedWith };
  }

  const required = requiredForService(forwarded);
  const missing = required.filter((v) => !isSatisfied(v, forwarded));
  return { missing, ok: missing.length === 0, skipped: false, ...mergedWith };
}

export type ComposeResult = {
  file: string;
  missing: string[];
  skipped: boolean;
  /** Set for an overlay: the base compose file it was merged onto. */
  mergedWith?: string;
};

/**
 * Once deploy/ exists it is the self-host production surface, so at least one
 * compose file there must declare a backend service. Without this, nesting the
 * stack more than one directory below deploy/ or renaming the service would
 * silently shrink the gate back to the dev docker-compose.yml.
 */
export function deployCoverageError(
  root: string,
  results: readonly ComposeResult[],
): string | null {
  if (!fs.existsSync(path.join(root, "deploy"))) return null;
  if (results.some((r) => !r.skipped && r.file.startsWith("deploy/"))) return null;
  return (
    "deploy/ exists but no compose file in deploy/ or deploy/*/ declares a backend service, " +
    "so the self-host stack is unchecked. Keep the compose file within that depth with a " +
    "'backend' service, or extend composeFiles() in scripts/check-compose-env-contract.ts."
  );
}

export function checkComposeContract(
  root: string,
  yamlLoad: YamlLoadFn | null,
): { results: ComposeResult[]; deployError: string | null } {
  const results: ComposeResult[] = [];
  for (const rel of composeFiles(root)) {
    const { missing, skipped, mergedWith } = checkComposeFile(root, rel, yamlLoad);
    results.push(mergedWith ? { file: rel, missing, skipped, mergedWith } : { file: rel, missing, skipped });
  }
  return { results, deployError: deployCoverageError(root, results) };
}

function main(): void {
  const yamlLoad = tryLoadYamlParser();
  if (!yamlLoad) {
    console.error(
      "WARN: neither 'yaml' nor 'js-yaml' resolved from frontend/; using minimal backend-service parser",
    );
  }

  const { results, deployError } = checkComposeContract(REPO_ROOT, yamlLoad);
  if (deployError) {
    console.error(`FAIL: ${deployError}`);
    process.exit(1);
  }
  const anyFailed = results.some((r) => r.missing.length > 0);

  if (anyFailed) {
    console.error(
      "FAIL: production-required backend env vars are not forwarded by compose.",
    );
    console.error(
      "Source of truth: backend/internal/config/production_preflight.go",
    );
    console.error("");
    for (const r of results) {
      if (r.skipped || r.missing.length === 0) continue;
      console.error(r.mergedWith ? `${r.file} (merged onto ${r.mergedWith}):` : `${r.file}:`);
      for (const v of r.missing) {
        console.error(`  - ${v}`);
      }
      console.error("");
    }
    console.error(
      "A var counts as forwarded only as environment KEY=... / map key, or as ${VAR} in command/entrypoint/args;",
    );
    console.error(
      "a backend service with a mandatory env_file forwards every variable of that file.",
    );
    console.error(
      "Bare list items like '- ${VAR:-}' do NOT forward the variable under its own name.",
    );
    process.exit(1);
  }

  const checked = results.filter((r) => !r.skipped).map((r) => r.file);
  console.log(
    `OK: all production-required backend env vars are forwarded (${checked.join(", ")})`,
  );
  process.exit(0);
}

// Run only when this file is the process entrypoint (tsx / node), not on import.
const entry = process.argv[1] ? path.resolve(process.argv[1]) : "";
if (/(?:^|\/)check-compose-env-contract\.(?:ts|js|mjs|cjs)$/.test(entry)) {
  main();
}
