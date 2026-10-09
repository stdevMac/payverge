import fs from "node:fs";
import path from "node:path";

import { repoRootFromFrontend } from "./common";

export interface HardcodedStringFinding {
  filePath: string;
  line: number;
  column: number;
  text: string;
}

const jsxTextPattern = />([A-Z][^<>{}\n]{2,})</g;
const guestJsxTextPattern = />([^<>{}\n]{3,})</g;
const allowedExactText = new Set(["Payverge"]);

// Per-file/per-string allow-list for KNOWN-acceptable hardcoded text.
//
// Unlike `ignoredPathSegments` (which skips whole directories) and
// `allowedExactText` (which waives a literal everywhere), this list waives a
// SINGLE exact string in ONE specific file, identified by a repo-relative,
// forward-slash `frontend/src/...:Exact text` key. It does NOT blanket-ignore
// the file — any OTHER hardcoded string in the same file is still reported, so
// the gate keeps catching NEW debt.
//
// Every entry must carry a reason. Add here only when the English text is
// intentional and tracked elsewhere (e.g. dark-shipped feature behind a flag,
// or a server-rendered error/not-found boundary with no translation provider
// in request scope where a hook would throw).
export interface AllowedHardcodedString {
  // Repo-relative path with forward slashes, e.g. "frontend/src/foo/Bar.tsx".
  path: string;
  // Exact trimmed JSX text the validator would otherwise flag.
  text: string;
  // Why this specific string is acceptable as hardcoded English.
  reason: string;
}

const allowedHardcodedStrings: AllowedHardcodedString[] = [
  // Guest-facing error / not-found boundaries under /t, /b, /profile, and
  // /tools now resolve copy via provider-free resolveErrorBoundaryCopy()
  // (frontend/src/i18n/errorBoundaryCopy.ts). No intentional hardcoded English
  // remains in those JSX trees — do not re-add allow-list entries for them.
  {
    path: "frontend/src/app/business/[businessId]/route.ts",
    text: "Page not found — Payverge",
    reason:
      "Route handler emits an authoritative 404 HTML fallback outside the App Router translation provider.",
  },
  {
    path: "frontend/src/app/business/[businessId]/route.ts",
    text: "Page not found",
    reason:
      "Route handler emits an authoritative 404 HTML fallback outside the App Router translation provider.",
  },
  {
    path: "frontend/src/app/business/[businessId]/route.ts",
    text: "This business page does not exist or is not published.",
    reason:
      "Route handler emits an authoritative 404 HTML fallback outside the App Router translation provider.",
  },
  {
    path: "frontend/src/app/business/[businessId]/route.ts",
    text: "Back to Payverge",
    reason:
      "Route handler emits an authoritative 404 HTML fallback outside the App Router translation provider.",
  },
];

// Fast lookup keyed on "<repo-relative forward-slash path>::<exact text>".
const allowedHardcodedKeys = new Set(
  allowedHardcodedStrings.map((entry) => `${entry.path}::${entry.text}`),
);

const ignoredPathSegments = [
  "/i18n/",
  "/contracts/",
  "/types/",
  "/__tests__/",
  "/test/",
  "/app/admin/",
  "/components/admin/",
  // Guest components are NOT skipped — per-string allow-list only (Batch F2a).
];

function main(): void {
  const repoRoot = repoRootFromFrontend();
  const findings = findHardcodedStrings(repoRoot);

  for (const finding of findings) {
    console.log(
      `${path.relative(repoRoot, finding.filePath)}:${finding.line}:${finding.column} ${finding.text}`,
    );
  }

  process.exitCode = findings.length > 0 ? 1 : 0;
}

export function findHardcodedStrings(
  repoRoot: string,
  // Optional per-file/per-string waivers to layer on top of the built-in
  // allow-list. Production callers omit this and get only the module-level
  // list; tests pass a self-contained entry to exercise the waiver mechanism
  // without depending on (or polluting) the production allow-list.
  extraAllowedHardcodedStrings: readonly AllowedHardcodedString[] = [],
): HardcodedStringFinding[] {
  const sourceRoot = path.join(repoRoot, "frontend", "src");

  if (!fs.existsSync(sourceRoot)) {
    return [];
  }

  const allowedKeys =
    extraAllowedHardcodedStrings.length === 0
      ? allowedHardcodedKeys
      : new Set([
          ...allowedHardcodedKeys,
          ...extraAllowedHardcodedStrings.map(
            (entry) => `${entry.path}::${entry.text}`,
          ),
        ]);

  const findings: HardcodedStringFinding[] = [];

  for (const filePath of walkSourceFiles(sourceRoot)) {
    const contents = fs.readFileSync(filePath, "utf8");
    const repoRelativePath = path
      .relative(repoRoot, filePath)
      .split(path.sep)
      .join("/");

    const pattern = isGuestFacingSource(repoRelativePath)
      ? guestJsxTextPattern
      : jsxTextPattern;
    for (const match of contents.matchAll(pattern)) {
      const rawText = match[1].trim();

      if (rawText.length === 0) {
        continue;
      }

      // Operators, punctuation, and numeric-only fragments are not prose.
      if (!/\p{L}/u.test(rawText)) {
        continue;
      }
      if (!/^\p{L}/u.test(rawText) || /(?:&&|\|\||=>|[|])/u.test(rawText)) {
        continue;
      }
      if (/^(?:Promise\b|void\b|Math\.)/u.test(rawText)) {
        continue;
      }

      if (allowedExactText.has(rawText)) {
        continue;
      }

      // Per-file/per-string waiver: only this exact string in this exact file
      // is allowed. Any other hardcoded text in the same file still fails.
      if (allowedKeys.has(`${repoRelativePath}::${rawText}`)) {
        continue;
      }

      const location = lineAndColumn(contents, (match.index ?? 0) + 1);
      findings.push({
        filePath,
        line: location.line,
        column: location.column,
        text: rawText,
      });
    }
  }

  return findings;
}

const guestFacingPrefixes = [
  "frontend/src/app/t/",
  "frontend/src/app/b/",
  "frontend/src/app/delivery/",
  "frontend/src/app/reservations/",
  "frontend/src/app/(shop)/customer/",
  "frontend/src/components/guest/",
  "frontend/src/components/business-page/",
  "frontend/src/components/payment/",
  "frontend/src/components/splitting/",
  "frontend/src/components/delivery/",
  "frontend/src/components/customer/",
];

function isGuestFacingSource(repoRelativePath: string): boolean {
  return guestFacingPrefixes.some((prefix) =>
    repoRelativePath.startsWith(prefix),
  );
}

function walkSourceFiles(sourceRoot: string): string[] {
  const files: string[] = [];

  for (const entry of fs.readdirSync(sourceRoot, { withFileTypes: true })) {
    const entryPath = path.join(sourceRoot, entry.name);

    if (shouldIgnore(entryPath)) {
      continue;
    }

    if (entry.isDirectory()) {
      files.push(...walkSourceFiles(entryPath));
      continue;
    }

    if (entry.isFile() && /\.(ts|tsx)$/.test(entry.name)) {
      files.push(entryPath);
    }
  }

  return files;
}

function shouldIgnore(filePath: string): boolean {
  const normalized = filePath.split(path.sep).join("/");
  const fileName = path.basename(filePath);

  return (
    /\.(test|spec)\.(ts|tsx)$/.test(fileName) ||
    ignoredPathSegments.some((segment) => normalized.includes(segment))
  );
}

function lineAndColumn(
  contents: string,
  index: number,
): { line: number; column: number } {
  const prefix = contents.slice(0, index);
  const lines = prefix.split("\n");

  return {
    line: lines.length,
    column: lines[lines.length - 1].length + 1,
  };
}

if (
  process.argv[1] &&
  /^check-hardcoded-strings\.[jt]s$/.test(path.basename(process.argv[1]))
) {
  main();
}
