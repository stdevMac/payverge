/**
 * Advertised-feature contract validator.
 *
 * Loads docs/product/advertised-feature-contract.json (+ schema) and fails
 * (non-zero exit) when the registry drifts from reality or internal rules.
 *
 * Usage:
 *   npx tsx scripts/check-advertised-feature-contract.ts
 *   npx tsx scripts/check-advertised-feature-contract.ts --strict-tests
 *   npx tsx scripts/check-advertised-feature-contract.ts --help
 *
 * Rules (default = 1–7; --strict-tests also enforces 8):
 *   1. Schema / unknown fields / unknown enum values
 *   2. Duplicate ids
 *   3. live rows must have non-empty evidence
 *   4. every evidence.path must exist on disk
 *   5. every keys entry file#dot.path must resolve in en message JSON (or file for docs/code)
 *   6. market coherence vs status
 *   7. curated REQUIRED_CLAIM_KEYS must be covered by some row
 *   8. live rows must have test_ids that reference existing Playwright tests
 */

import * as fs from "node:fs";
import * as path from "node:path";
import ts from "typescript";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type FeatureStatus =
  | "live"
  | "market_limited"
  | "beta"
  | "coming_soon"
  | "removed";

export type FeatureAudience =
  | "guest"
  | "operator"
  | "admin"
  | "public-marketing";


export interface EvidenceEntry {
  path: string;
  note: string;
}

export interface LastVerified {
  sha: string;
  date: string;
}

export interface FeatureRow {
  id: string;
  title: string;
  keys: string[];
  status: FeatureStatus;
  audience: FeatureAudience;
  markets: string[];
  prerequisites: string[];
  evidence: EvidenceEntry[];
  test_ids: string[];
  last_verified: LastVerified;
  owner: string;
}

export interface FeatureRegistry {
  version: number;
  features: FeatureRow[];
}

export interface ValidationError {
  rule: number | "schema";
  message: string;
  id?: string;
}

export interface ValidateOptions {
  /** Absolute or relative path to registry JSON. */
  registryPath?: string;
  /** Absolute or relative path to schema JSON. */
  schemaPath?: string;
  /** Repo root (parent of frontend/). Default: inferred from this file. */
  repoRoot?: string;
  /** en messages directory. Default: <repoRoot>/frontend/src/i18n/messages/en */
  messagesDir?: string;
  /** Playwright specs directory. Default: <repoRoot>/frontend/tests */
  playwrightTestsDir?: string;
  /** When true, enforce rule 8 (live test_ids). Default false. */
  strictTests?: boolean;
  /** Override curated claim keys (rule 7). */
  requiredClaimKeys?: readonly string[];
  /** In-memory registry (fixture tests). Skips loading registryPath when set. */
  registry?: FeatureRegistry;
  /** In-memory schema (fixture tests). */
  schema?: Record<string, unknown>;
  /** Optional existence override (fixture tests). */
  pathExists?: (repoRelativePath: string) => boolean;
  /** Optional Playwright corpus override (fixture tests). */
  playwrightCorpus?: string;
}

export interface ValidateResult {
  ok: boolean;
  errors: ValidationError[];
  exitCode: number;
  warnings: string[];
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STATUSES: readonly FeatureStatus[] = [
  "live",
  "market_limited",
  "beta",
  "coming_soon",
  "removed",
];
const AUDIENCES: readonly FeatureAudience[] = [
  "guest",
  "operator",
  "admin",
  "public-marketing",
];

/**
 * Curated disputed / major marketing claim keys that MUST appear in some
 * registry row's `keys`. Adding or changing a public claim without a registry
 * entry fails rule 7.
 *
 * Paths are relative to frontend/src/i18n/messages/en/ unless they start with
 * docs/ or a config path.
 */
export const REQUIRED_CLAIM_KEYS: readonly string[] = [
  // Google Reviews: automatic capture is still marked Coming Soon.
  "businessSettings.json#businessPage.googleReviewsDescription",
  // AI autonomy claims: the AI asks before acting, which the
  // propose/apply/undo rail must keep true.
  "businessDashboard.json#reservations.toggle.features.bookingDesc",
];

const ROW_REQUIRED_FIELDS = [
  "id",
  "title",
  "keys",
  "status",
  "audience",
  "markets",
  "prerequisites",
  "evidence",
  "test_ids",
  "last_verified",
  "owner",
] as const;

const ROW_ALLOWED_FIELDS = new Set<string>([...ROW_REQUIRED_FIELDS]);

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

function defaultRepoRoot(): string {
  // scripts/ → frontend/ → repo root
  return path.resolve(__dirname, "..", "..");
}

function defaultMessagesDir(repoRoot: string): string {
  return path.join(repoRoot, "frontend", "src", "i18n", "messages", "en");
}

function defaultPlaywrightDir(repoRoot: string): string {
  return path.join(repoRoot, "frontend", "tests");
}

function defaultRegistryPath(repoRoot: string): string {
  return path.join(
    repoRoot,
    "docs",
    "product",
    "advertised-feature-contract.json",
  );
}

function defaultSchemaPath(repoRoot: string): string {
  return path.join(
    repoRoot,
    "docs",
    "product",
    "advertised-feature-contract.schema.json",
  );
}

function loadJsonFile(filePath: string): unknown {
  const raw = fs.readFileSync(filePath, "utf8");
  return JSON.parse(raw);
}

function fileExistsOnDisk(absPath: string): boolean {
  try {
    return fs.existsSync(absPath);
  } catch {
    return false;
  }
}

function repoPathStatus(
  repoRoot: string,
  relPath: string,
): "exact" | "case_mismatch" | "missing" {
  const segments = relPath.split(/[\\/]+/).filter(Boolean);
  let current = repoRoot;
  let sawCaseMismatch = false;

  for (const segment of segments) {
    let entries: string[];
    try {
      entries = fs.readdirSync(current);
    } catch {
      return "missing";
    }

    if (entries.includes(segment)) {
      current = path.join(current, segment);
      continue;
    }

    const lowerSegment = segment.toLowerCase();
    const actualSegment = entries.find(
      (entry) => entry.toLowerCase() === lowerSegment,
    );
    if (!actualSegment) {
      return "missing";
    }

    sawCaseMismatch = true;
    current = path.join(current, actualSegment);
  }

  return sawCaseMismatch ? "case_mismatch" : "exact";
}

/**
 * Resolve a registry `keys` entry.
 * - `businessSettings.json#businessPage.googleReviewsDescription` → en messages JSON key
 * - `docs/ONBOARDING.md` or `docs/foo.md#section` → repo file
 * - `frontend/src/app/layout.tsx` → repo file (no JSON key)
 */
export function parseKeyEntry(key: string): {
  file: string;
  pointer: string | null;
} {
  const hash = key.indexOf("#");
  if (hash === -1) {
    return { file: key, pointer: null };
  }
  return {
    file: key.slice(0, hash),
    pointer: key.slice(hash + 1) || null,
  };
}

/**
 * Walk a JSON value by a dotted path that may include array indices:
 *   hero.ctaSubtext
 *   paymentsBridge.methods[2]
 *   items.analytics.bullets[2]
 */
export function resolveJsonPointer(
  root: unknown,
  pointer: string,
): {
  found: boolean;
  value?: unknown;
} {
  if (!pointer) {
    return { found: true, value: root };
  }
  // Tokenize: split on '.' but keep [n] attached to the segment before it,
  // then expand [n] as separate array steps.
  const tokens: Array<string | number> = [];
  for (const segment of pointer.split(".")) {
    let m: RegExpExecArray | null;
    let rest = segment;
    // Handle name[0][1] form
    const nameMatch = /^([^\[]+)/.exec(rest);
    if (nameMatch) {
      tokens.push(nameMatch[1]);
      rest = rest.slice(nameMatch[1].length);
    }
    while ((m = /^\[(\d+)\]/.exec(rest))) {
      tokens.push(Number(m[1]));
      rest = rest.slice(m[0].length);
    }
    if (rest.length > 0 && !nameMatch) {
      // bare [n] shouldn't happen after split; treat as failure
      return { found: false };
    }
  }

  let cur: unknown = root;
  for (const tok of tokens) {
    if (typeof tok === "number") {
      if (!Array.isArray(cur) || tok < 0 || tok >= cur.length) {
        return { found: false };
      }
      cur = cur[tok];
    } else {
      if (
        cur === null ||
        typeof cur !== "object" ||
        Array.isArray(cur) ||
        !(tok in (cur as Record<string, unknown>))
      ) {
        return { found: false };
      }
      cur = (cur as Record<string, unknown>)[tok];
    }
  }
  return { found: true, value: cur };
}

// ---------------------------------------------------------------------------
// Schema validation (rule 1) — draft-2020-12 subset for this registry
// ---------------------------------------------------------------------------

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function validateSchemaShape(
  registry: unknown,
  _schema: Record<string, unknown> | undefined,
): ValidationError[] {
  const errors: ValidationError[] = [];

  if (!isPlainObject(registry)) {
    errors.push({
      rule: "schema",
      message: "Registry root must be a JSON object",
    });
    return errors;
  }

  const unknownRoot = Object.keys(registry).filter(
    (k) => k !== "version" && k !== "features" && k !== "$schema",
  );
  for (const k of unknownRoot) {
    errors.push({
      rule: 1,
      message: `Unknown top-level field: "${k}"`,
    });
  }

  if (
    typeof registry.version !== "number" ||
    !Number.isInteger(registry.version) ||
    registry.version < 1
  ) {
    errors.push({
      rule: 1,
      message: `Invalid version: expected integer >= 1, got ${JSON.stringify(registry.version)}`,
    });
  }

  if (!Array.isArray(registry.features)) {
    errors.push({
      rule: 1,
      message: `Invalid features: expected array, got ${typeof registry.features}`,
    });
    return errors;
  }

  if (registry.features.length === 0) {
    errors.push({
      rule: 1,
      message: "features array must not be empty",
    });
  }

  registry.features.forEach((row, index) => {
    const loc = `features[${index}]`;
    if (!isPlainObject(row)) {
      errors.push({
        rule: 1,
        message: `${loc}: expected object`,
      });
      return;
    }

    for (const field of ROW_REQUIRED_FIELDS) {
      if (!(field in row)) {
        errors.push({
          rule: 1,
          message: `${loc}: missing required field "${field}"`,
          id: typeof row.id === "string" ? row.id : undefined,
        });
      }
    }

    for (const key of Object.keys(row)) {
      if (!ROW_ALLOWED_FIELDS.has(key)) {
        errors.push({
          rule: 1,
          message: `${loc}: unknown field "${key}"`,
          id: typeof row.id === "string" ? row.id : undefined,
        });
      }
    }

    const id = row.id;
    if (typeof id !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(id)) {
      errors.push({
        rule: 1,
        message: `${loc}: id must be kebab-case string, got ${JSON.stringify(id)}`,
      });
    }

    if (typeof row.title !== "string" || row.title.length === 0) {
      errors.push({
        rule: 1,
        message: `${loc}: title must be a non-empty string`,
        id: typeof id === "string" ? id : undefined,
      });
    }

    if (
      typeof row.status !== "string" ||
      !STATUSES.includes(row.status as FeatureStatus)
    ) {
      errors.push({
        rule: 1,
        message: `${loc}: unknown status ${JSON.stringify(row.status)}; allowed: ${STATUSES.join(", ")}`,
        id: typeof id === "string" ? id : undefined,
      });
    }

    if (
      typeof row.audience !== "string" ||
      !AUDIENCES.includes(row.audience as FeatureAudience)
    ) {
      errors.push({
        rule: 1,
        message: `${loc}: unknown audience ${JSON.stringify(row.audience)}; allowed: ${AUDIENCES.join(", ")}`,
        id: typeof id === "string" ? id : undefined,
      });
    }

    for (const arrField of [
      "keys",
      "markets",
      "prerequisites",
      "test_ids",
    ] as const) {
      if (!Array.isArray(row[arrField])) {
        errors.push({
          rule: 1,
          message: `${loc}: ${arrField} must be an array`,
          id: typeof id === "string" ? id : undefined,
        });
      } else {
        for (const item of row[arrField] as unknown[]) {
          if (typeof item !== "string") {
            errors.push({
              rule: 1,
              message: `${loc}: ${arrField} entries must be strings`,
              id: typeof id === "string" ? id : undefined,
            });
          }
        }
      }
    }

    if (!Array.isArray(row.evidence)) {
      errors.push({
        rule: 1,
        message: `${loc}: evidence must be an array`,
        id: typeof id === "string" ? id : undefined,
      });
    } else {
      row.evidence.forEach((ev, ei) => {
        if (!isPlainObject(ev)) {
          errors.push({
            rule: 1,
            message: `${loc}.evidence[${ei}]: expected object`,
            id: typeof id === "string" ? id : undefined,
          });
          return;
        }
        for (const k of Object.keys(ev)) {
          if (k !== "path" && k !== "note") {
            errors.push({
              rule: 1,
              message: `${loc}.evidence[${ei}]: unknown field "${k}"`,
              id: typeof id === "string" ? id : undefined,
            });
          }
        }
        if (typeof ev.path !== "string" || ev.path.length === 0) {
          errors.push({
            rule: 1,
            message: `${loc}.evidence[${ei}]: path must be a non-empty string`,
            id: typeof id === "string" ? id : undefined,
          });
        }
        if (typeof ev.note !== "string" || ev.note.length === 0) {
          errors.push({
            rule: 1,
            message: `${loc}.evidence[${ei}]: note must be a non-empty string`,
            id: typeof id === "string" ? id : undefined,
          });
        }
      });
    }

    if (!isPlainObject(row.last_verified)) {
      errors.push({
        rule: 1,
        message: `${loc}: last_verified must be an object`,
        id: typeof id === "string" ? id : undefined,
      });
    } else {
      for (const k of Object.keys(row.last_verified)) {
        if (k !== "sha" && k !== "date") {
          errors.push({
            rule: 1,
            message: `${loc}.last_verified: unknown field "${k}"`,
            id: typeof id === "string" ? id : undefined,
          });
        }
      }
      if (
        typeof row.last_verified.sha !== "string" ||
        !/^[0-9a-f]{7,40}$/i.test(row.last_verified.sha)
      ) {
        errors.push({
          rule: 1,
          message: `${loc}.last_verified.sha must be a 7-40 character hexadecimal commit`,
          id: typeof id === "string" ? id : undefined,
        });
      }
      if (
        typeof row.last_verified.date !== "string" ||
        !/^\d{4}-\d{2}-\d{2}$/.test(row.last_verified.date)
      ) {
        errors.push({
          rule: 1,
          message: `${loc}.last_verified.date must be YYYY-MM-DD`,
          id: typeof id === "string" ? id : undefined,
        });
      }
    }

    if (typeof row.owner !== "string" || row.owner.length === 0) {
      errors.push({
        rule: 1,
        message: `${loc}: owner must be a non-empty string`,
        id: typeof id === "string" ? id : undefined,
      });
    }
  });

  return errors;
}

// ---------------------------------------------------------------------------
// Rules 2–8
// ---------------------------------------------------------------------------

function asRegistry(registry: unknown): FeatureRegistry | null {
  if (!isPlainObject(registry) || !Array.isArray(registry.features)) {
    return null;
  }
  return registry as unknown as FeatureRegistry;
}

function ruleDuplicateIds(features: FeatureRow[]): ValidationError[] {
  const seen = new Map<string, number>();
  const errors: ValidationError[] = [];
  features.forEach((row, i) => {
    if (typeof row.id !== "string") return;
    if (seen.has(row.id)) {
      errors.push({
        rule: 2,
        id: row.id,
        message: `Duplicate id "${row.id}" at features[${i}] (first at features[${seen.get(row.id)}])`,
      });
    } else {
      seen.set(row.id, i);
    }
  });
  return errors;
}

function ruleLiveEvidence(features: FeatureRow[]): ValidationError[] {
  const errors: ValidationError[] = [];
  for (const row of features) {
    if (
      row.status === "live" &&
      (!Array.isArray(row.evidence) || row.evidence.length === 0)
    ) {
      errors.push({
        rule: 3,
        id: row.id,
        message: `live row "${row.id}" has empty evidence — every live claim needs at least one evidence path`,
      });
    }
  }
  return errors;
}

function ruleEvidencePathsExist(
  features: FeatureRow[],
  repoRoot: string,
): ValidationError[] {
  const errors: ValidationError[] = [];
  for (const row of features) {
    if (!Array.isArray(row.evidence)) continue;
    for (const ev of row.evidence) {
      if (typeof ev?.path !== "string" || !ev.path) continue;
      // Reject absolute paths and parent traversal
      if (ev.path.startsWith("/") || ev.path.includes("..")) {
        errors.push({
          rule: 4,
          id: row.id,
          message: `evidence path must be repo-relative without "..": "${ev.path}" (row "${row.id}")`,
        });
        continue;
      }
      const pathStatus = repoPathStatus(repoRoot, ev.path);
      if (pathStatus === "missing") {
        errors.push({
          rule: 4,
          id: row.id,
          message: `evidence path does not exist: "${ev.path}" (row "${row.id}")`,
        });
        continue;
      }
      if (pathStatus === "case_mismatch") {
        errors.push({
          rule: 4,
          id: row.id,
          message: `evidence path case mismatch: "${ev.path}" (row "${row.id}")`,
        });
      }
    }
  }
  return errors;
}

function isEnMessageJson(file: string): boolean {
  if (file.startsWith("docs/")) return false;
  if (file.startsWith("frontend/") || file.startsWith("backend/")) return false;
  return file.endsWith(".json") && !file.includes("/");
}

function resolveKeyFileAbs(
  file: string,
  repoRoot: string,
  messagesDir: string,
): string {
  if (
    file.startsWith("docs/") ||
    file.startsWith("frontend/") ||
    file.startsWith("backend/")
  ) {
    return path.join(repoRoot, file);
  }
  // Bare i18n message file name
  if (file.endsWith(".json")) {
    return path.join(messagesDir, file);
  }
  // Bare config or other path under frontend
  return path.join(repoRoot, file);
}

function ruleKeysResolve(
  features: FeatureRow[],
  repoRoot: string,
  messagesDir: string,
  pathExists: (repoRelativePath: string) => boolean,
  messageCache: Map<string, unknown>,
): ValidationError[] {
  const errors: ValidationError[] = [];

  const loadMessage = (file: string): unknown | null => {
    if (messageCache.has(file)) return messageCache.get(file);
    const abs = path.join(messagesDir, file);
    if (!fileExistsOnDisk(abs)) {
      messageCache.set(file, null);
      return null;
    }
    try {
      const data = loadJsonFile(abs);
      messageCache.set(file, data);
      return data;
    } catch (e) {
      messageCache.set(file, null);
      return null;
    }
  };

  for (const row of features) {
    if (!Array.isArray(row.keys)) continue;
    for (const key of row.keys) {
      if (typeof key !== "string" || !key) {
        errors.push({
          rule: 5,
          id: row.id,
          message: `row "${row.id}": empty or non-string keys entry`,
        });
        continue;
      }
      const { file, pointer } = parseKeyEntry(key);

      if (isEnMessageJson(file)) {
        const data = loadMessage(file);
        if (data === null) {
          errors.push({
            rule: 5,
            id: row.id,
            message: `row "${row.id}": keys entry "${key}" — message file not found: ${file}`,
          });
          continue;
        }
        if (pointer) {
          const { found } = resolveJsonPointer(data, pointer);
          if (!found) {
            errors.push({
              rule: 5,
              id: row.id,
              message: `row "${row.id}": stale key — "${key}" does not resolve in en/${file}`,
            });
          }
        }
      } else {
        // Non-i18n: file must exist on disk (ignore pointer fragment)
        const rel =
          file.startsWith("docs/") ||
          file.startsWith("frontend/") ||
          file.startsWith("backend/")
            ? file
            : file;
        const abs = resolveKeyFileAbs(file, repoRoot, messagesDir);
        const exists =
          pathExists(rel) || fileExistsOnDisk(abs) || pathExists(file);
        if (!exists) {
          errors.push({
            rule: 5,
            id: row.id,
            message: `row "${row.id}": keys entry "${key}" — file does not exist: ${file}`,
          });
        }
      }
    }
  }
  return errors;
}

function ruleMarketCoherence(features: FeatureRow[]): ValidationError[] {
  const errors: ValidationError[] = [];
  for (const row of features) {
    if (!row || typeof row.status !== "string") continue;

    // markets must be non-empty for every non-removed status
    if (
      row.status !== "removed" &&
      (!Array.isArray(row.markets) || row.markets.length === 0)
    ) {
      errors.push({
        rule: 6,
        id: row.id,
        message: `row "${row.id}": status "${row.status}" requires non-empty markets (e.g. ["global"] or ["AR"])`,
      });
    }

    // market_limited must name a non-global market restriction
    if (
      row.status === "market_limited" &&
      Array.isArray(row.markets) &&
      row.markets.length > 0 &&
      row.markets.every((m) => m === "global")
    ) {
      errors.push({
        rule: 6,
        id: row.id,
        message: `row "${row.id}": status "market_limited" contradicts markets=["global"] — list the limited market(s)`,
      });
    }

    // prerequisites that mention a country code should appear in markets
    if (Array.isArray(row.prerequisites) && Array.isArray(row.markets)) {
      for (const pre of row.prerequisites) {
        if (typeof pre !== "string") continue;
        const m = pre.match(/\b(AR|US|AE|EU|UK|MX|BR)\b/);
        if (
          m &&
          !row.markets.includes(m[1]) &&
          !row.markets.includes("global")
        ) {
          errors.push({
            rule: 6,
            id: row.id,
            message: `row "${row.id}": prerequisite mentions market "${m[1]}" but markets is ${JSON.stringify(row.markets)}`,
          });
        }
      }
    }
  }
  return errors;
}

function ruleRequiredClaimKeys(
  features: FeatureRow[],
  required: readonly string[],
): ValidationError[] {
  const covered = new Set<string>();
  for (const row of features) {
    if (!Array.isArray(row.keys)) continue;
    for (const k of row.keys) {
      if (typeof k === "string") covered.add(k);
    }
  }
  const errors: ValidationError[] = [];
  for (const key of required) {
    if (!covered.has(key)) {
      errors.push({
        rule: 7,
        message: `Unrepresented claim key — no registry row covers "${key}". Add a row (or update keys) for this public claim.`,
      });
    }
  }
  return errors;
}

function loadPlaywrightCorpus(testsDir: string): string {
  if (!fileExistsOnDisk(testsDir)) return "";
  const parts: string[] = [];
  const callPath = (expr: ts.Expression): string => {
    if (ts.isIdentifier(expr)) return expr.text;
    if (ts.isPropertyAccessExpression(expr)) {
      return `${callPath(expr.expression)}.${expr.name.text}`;
    }
    return "";
  };
  const walk = (dir: string) => {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const ent of entries) {
      const full = path.join(dir, ent.name);
      if (ent.isDirectory()) {
        walk(full);
      } else if (/\.(spec|test)\.(ts|js|tsx|jsx)$/.test(ent.name)) {
        try {
          const source = fs.readFileSync(full, "utf8");
          const sf = ts.createSourceFile(
            full,
            source,
            ts.ScriptTarget.Latest,
            true,
            full.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
          );
          const visit = (node: ts.Node): void => {
            if (
              ts.isCallExpression(node) &&
              callPath(node.expression) === "test"
            ) {
              const title = node.arguments[0];
              if (
                title &&
                (ts.isStringLiteral(title) ||
                  ts.isNoSubstitutionTemplateLiteral(title))
              ) {
                parts.push(`${path.basename(full)}::${title.text}`);
              }
            }
            ts.forEachChild(node, visit);
          };
          visit(sf);
        } catch {
          // ignore unreadable
        }
      }
    }
  };
  walk(testsDir);
  return parts.join("\n");
}

function ruleStrictTests(
  features: FeatureRow[],
  corpus: string,
): ValidationError[] {
  const errors: ValidationError[] = [];
  for (const row of features) {
    if (row.status !== "live") continue;
    if (!Array.isArray(row.test_ids) || row.test_ids.length === 0) {
      errors.push({
        rule: 8,
        id: row.id,
        message: `live row "${row.id}" has empty test_ids (strict-tests mode requires Playwright journey coverage)`,
      });
      continue;
    }
    for (const tid of row.test_ids) {
      if (typeof tid !== "string" || !tid) {
        errors.push({
          rule: 8,
          id: row.id,
          message: `live row "${row.id}" has empty test_id entry`,
        });
        continue;
      }
      if (!corpus.includes(tid)) {
        errors.push({
          rule: 8,
          id: row.id,
          message: `live row "${row.id}" test_id "${tid}" not found in any Playwright spec under frontend/tests`,
        });
      }
    }
  }
  return errors;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function validateAdvertisedFeatureContract(
  options: ValidateOptions = {},
): ValidateResult {
  const repoRoot = path.resolve(options.repoRoot ?? defaultRepoRoot());
  const messagesDir = path.resolve(
    options.messagesDir ?? defaultMessagesDir(repoRoot),
  );
  const playwrightTestsDir = path.resolve(
    options.playwrightTestsDir ?? defaultPlaywrightDir(repoRoot),
  );
  const registryPath = path.resolve(
    options.registryPath ?? defaultRegistryPath(repoRoot),
  );
  const schemaPath = path.resolve(
    options.schemaPath ?? defaultSchemaPath(repoRoot),
  );
  const requiredKeys = options.requiredClaimKeys ?? REQUIRED_CLAIM_KEYS;
  const strictTests = Boolean(options.strictTests);

  const errors: ValidationError[] = [];
  const warnings: string[] = [];

  let schema: Record<string, unknown> | undefined = options.schema;
  if (!schema) {
    if (!fileExistsOnDisk(schemaPath)) {
      errors.push({
        rule: "schema",
        message: `Schema file not found: ${schemaPath}`,
      });
    } else {
      try {
        schema = loadJsonFile(schemaPath) as Record<string, unknown>;
      } catch (e) {
        errors.push({
          rule: "schema",
          message: `Failed to parse schema: ${e instanceof Error ? e.message : String(e)}`,
        });
      }
    }
  }

  let registryRaw: unknown = options.registry;
  if (registryRaw === undefined) {
    if (!fileExistsOnDisk(registryPath)) {
      errors.push({
        rule: "schema",
        message: `Registry file not found: ${registryPath}`,
      });
      return { ok: false, errors, exitCode: 1, warnings };
    }
    try {
      registryRaw = loadJsonFile(registryPath);
    } catch (e) {
      errors.push({
        rule: "schema",
        message: `Failed to parse registry: ${e instanceof Error ? e.message : String(e)}`,
      });
      return { ok: false, errors, exitCode: 1, warnings };
    }
  }

  // Rule 1
  errors.push(...validateSchemaShape(registryRaw, schema));

  const registry = asRegistry(registryRaw);
  if (!registry) {
    return {
      ok: false,
      errors,
      exitCode: 1,
      warnings,
    };
  }

  const features = registry.features as FeatureRow[];

  const pathExists =
    options.pathExists ??
    ((rel: string) => fileExistsOnDisk(path.join(repoRoot, rel)));

  // Rule 2
  errors.push(...ruleDuplicateIds(features));
  // Rule 3
  errors.push(...ruleLiveEvidence(features));
  // Rule 4
  errors.push(...ruleEvidencePathsExist(features, repoRoot));
  // Rule 5
  errors.push(
    ...ruleKeysResolve(features, repoRoot, messagesDir, pathExists, new Map()),
  );
  // Rule 6
  errors.push(...ruleMarketCoherence(features));
  // Rule 7
  errors.push(...ruleRequiredClaimKeys(features, requiredKeys));
  // Rule 8 (gated)
  if (strictTests) {
    const corpus =
      options.playwrightCorpus ?? loadPlaywrightCorpus(playwrightTestsDir);
    errors.push(...ruleStrictTests(features, corpus));
  }

  const ok = errors.length === 0;
  return {
    ok,
    errors,
    exitCode: ok ? 0 : 1,
    warnings,
  };
}

export function formatValidationResult(result: ValidateResult): string {
  const lines: string[] = [];
  if (result.ok) {
    lines.push("advertised-feature-contract: PASS");
    if (result.warnings.length) {
      for (const w of result.warnings) lines.push(`  warning: ${w}`);
    }
    return lines.join("\n");
  }
  lines.push(
    `advertised-feature-contract: FAIL (${result.errors.length} error${result.errors.length === 1 ? "" : "s"})`,
  );
  for (const err of result.errors) {
    const ruleLabel = err.rule === "schema" ? "schema" : `rule ${err.rule}`;
    const idPart = err.id ? ` [${err.id}]` : "";
    lines.push(`  - (${ruleLabel})${idPart} ${err.message}`);
  }
  return lines.join("\n");
}

export function printHelp(): string {
  return `Usage: check-advertised-feature-contract [options]

Validate docs/product/advertised-feature-contract.json against its schema and
repo evidence.

Options:
  --strict-tests   Also enforce rule 8: live rows need non-empty test_ids that
                   appear in frontend/tests Playwright specs (off by default;
                   journeys land in Task 4).
  --registry PATH  Override registry JSON path
  --schema PATH    Override schema JSON path
  --repo-root PATH Override monorepo root
  --help, -h       Show this help

Exit codes:
  0  all enforced rules pass
  1  one or more rule failures

Rules:
  1  Schema / unknown fields / unknown status|audience|plan enums
  2  Duplicate feature ids
  3  live rows must have non-empty evidence
  4  evidence.path must exist on disk
  5  keys file#dot.path must resolve in en message JSON (or file must exist)
  6  market coherence vs status
  7  curated REQUIRED_CLAIM_KEYS must be covered
  8  live test_ids coverage (--strict-tests only)
`;
}

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

function parseArgs(argv: string[]): {
  help: boolean;
  strictTests: boolean;
  registryPath?: string;
  schemaPath?: string;
  repoRoot?: string;
} {
  const out: ReturnType<typeof parseArgs> = {
    help: false,
    strictTests: false,
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--help" || a === "-h") out.help = true;
    else if (a === "--strict-tests") out.strictTests = true;
    else if (a === "--registry") out.registryPath = argv[++i];
    else if (a === "--schema") out.schemaPath = argv[++i];
    else if (a === "--repo-root") out.repoRoot = argv[++i];
    else if (a.startsWith("-")) {
      console.error(`Unknown option: ${a}`);
      out.help = true;
    }
  }
  return out;
}

function main(argv: string[]): number {
  const args = parseArgs(argv);
  if (args.help) {
    // eslint-disable-next-line no-console
    console.log(printHelp());
    return 0;
  }
  const result = validateAdvertisedFeatureContract({
    strictTests: args.strictTests,
    registryPath: args.registryPath,
    schemaPath: args.schemaPath,
    repoRoot: args.repoRoot,
  });
  // eslint-disable-next-line no-console
  console.log(formatValidationResult(result));
  return result.exitCode;
}

// Run when executed directly (tsx / node / ts-node). Jest imports this module
// and must not trigger CLI exit.
const invokedAsCli = (() => {
  if (typeof require !== "undefined" && typeof module !== "undefined") {
    try {
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      if (require.main === module) return true;
    } catch {
      /* ignore */
    }
  }
  if (typeof process === "undefined" || !process.argv[1]) return false;
  const base = path.basename(process.argv[1]);
  return base.includes("check-advertised-feature-contract");
})();

if (invokedAsCli) {
  process.exit(main(process.argv.slice(2)));
}
