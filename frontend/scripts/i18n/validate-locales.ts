import fs from "node:fs";
import path from "node:path";

import {
  readRegistry,
  repoRootFromFrontend,
  type LocaleRegistryEntry,
} from "./common";
import {
  findHardcodedStrings,
  type AllowedHardcodedString,
} from "./check-hardcoded-strings";

export interface LocaleValidationError {
  locale: string;
  surface: string;
  code: string;
  message: string;
  path?: string;
  expected?: string;
  actual?: string;
}

export interface LocaleValidationResult {
  ok: boolean;
  errors: LocaleValidationError[];
  warnings: LocaleValidationError[];
}

interface LocaleStatus {
  canonical?: unknown;
  pathSegment?: unknown;
  publishable?: unknown;
  reviewOwner?: unknown;
  reviewDate?: unknown;
  review?: unknown;
  unresolvedQuestions?: unknown;
}

const frontendLocaleFiles = [
  "common.json",
  "apiErrors.json",
  "routes.json",
];

export function runLocaleValidation(opts: {
  repoRoot: string;
  // Test-only: extra per-file/per-string hardcoded-text waivers layered on top
  // of the production allow-list. Production callers omit this.
  extraAllowedHardcodedStrings?: readonly AllowedHardcodedString[];
}): LocaleValidationResult {
  const registry = readRegistry(opts.repoRoot);
  const errors: LocaleValidationError[] = [];
  const warnings: LocaleValidationError[] = [];

  for (const locale of Object.values(registry.locales)) {
    const statusPath = path.join(
      opts.repoRoot,
      "locales",
      locale.pathSegment,
      "status.json",
    );
    const status = readLocaleStatus(opts.repoRoot, locale, statusPath, errors);

    validateRequiredSurfaceFiles(opts.repoRoot, locale, errors);

    if (!status) {
      continue;
    }

    validateStatusMetadata(locale, status, errors);
    validatePublishableState(locale, status, warnings);
    validatePublishableReview(locale, status, errors);
  }

  validateHardcodedStrings(
    opts.repoRoot,
    errors,
    opts.extraAllowedHardcodedStrings ?? [],
  );

  return { ok: errors.length === 0, errors, warnings };
}

function validateHardcodedStrings(
  repoRoot: string,
  errors: LocaleValidationError[],
  extraAllowedHardcodedStrings: readonly AllowedHardcodedString[],
): void {
  const findings = findHardcodedStrings(repoRoot, extraAllowedHardcodedStrings);

  for (const finding of findings) {
    errors.push({
      locale: "*",
      surface: "frontend",
      code: "HARDCODED_USER_FACING_TEXT",
      message: `Hardcoded user-facing text candidate: ${finding.text}`,
      path: `${relativePath(repoRoot, finding.filePath)}:${finding.line}:${finding.column}`,
    });
  }
}

function readLocaleStatus(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  statusPath: string,
  errors: LocaleValidationError[],
): LocaleStatus | undefined {
  if (!fs.existsSync(statusPath)) {
    errors.push({
      locale: locale.canonical,
      surface: "status",
      code: "MISSING_STATUS_FILE",
      message: `Missing status file for ${locale.canonical}`,
      path: relativePath(repoRoot, statusPath),
    });

    return undefined;
  }

  try {
    return JSON.parse(fs.readFileSync(statusPath, "utf8")) as LocaleStatus;
  } catch (error) {
    errors.push({
      locale: locale.canonical,
      surface: "status",
      code: "INVALID_STATUS_FILE",
      message: `Invalid status file for ${locale.canonical}: ${errorMessage(
        error,
      )}`,
      path: relativePath(repoRoot, statusPath),
    });

    return undefined;
  }
}

function validateStatusMetadata(
  locale: LocaleRegistryEntry,
  status: LocaleStatus,
  errors: LocaleValidationError[],
): void {
  if (
    status.canonical !== locale.canonical ||
    status.pathSegment !== locale.pathSegment
  ) {
    errors.push({
      locale: locale.canonical,
      surface: "status",
      code: "STATUS_LOCALE_MISMATCH",
      message: `Status metadata does not match registry for ${locale.canonical}`,
      expected: `${locale.canonical}/${locale.pathSegment}`,
      actual: `${String(status.canonical)}/${String(status.pathSegment)}`,
    });
  }
}

function validatePublishableState(
  locale: LocaleRegistryEntry,
  status: LocaleStatus,
  warnings: LocaleValidationError[],
): void {
  if (locale.publishable === status.publishable) {
    return;
  }

  warnings.push({
    locale: locale.canonical,
    surface: "status",
    code: "PUBLISHABLE_MISMATCH",
    message: `Registry publishable=${String(
      locale.publishable,
    )} differs from status publishable=${String(status.publishable)}`,
    expected: String(locale.publishable),
    actual: String(status.publishable),
  });
}

function validatePublishableReview(
  locale: LocaleRegistryEntry,
  status: LocaleStatus,
  errors: LocaleValidationError[],
): void {
  if (!isPublishable(locale, status)) {
    return;
  }

  if (
    typeof status.reviewOwner !== "string" ||
    status.reviewOwner.trim().length < 2
  ) {
    errors.push({
      locale: locale.canonical,
      surface: "review",
      code: "MISSING_REVIEW_OWNER",
      message: `Publishable locale ${locale.canonical} has no recorded language owner`,
    });
  }

  if (
    typeof status.reviewDate !== "string" ||
    !/^\d{4}-\d{2}-\d{2}$/.test(status.reviewDate)
  ) {
    errors.push({
      locale: locale.canonical,
      surface: "review",
      code: "MISSING_REVIEW_DATE",
      message: `Publishable locale ${locale.canonical} has no ISO review date`,
    });
  }

  if (
    Array.isArray(status.unresolvedQuestions) &&
    status.unresolvedQuestions.length > 0
  ) {
    errors.push({
      locale: locale.canonical,
      surface: "review",
      code: "UNRESOLVED_REVIEW_QUESTIONS",
      message: `Publishable locale ${locale.canonical} has unresolved review questions`,
    });
  }

  const review = isRecord(status.review) ? status.review : {};

  for (const surface of locale.requiredSurfaces) {
    if (!(surface in review)) {
      errors.push({
        locale: locale.canonical,
        surface,
        code: "MISSING_SURFACE_REVIEW_STATUS",
        message: `Publishable locale ${locale.canonical} is missing review status for ${surface}`,
      });
      continue;
    }

    if (review[surface] !== "approved") {
      errors.push({
        locale: locale.canonical,
        surface,
        code: "SURFACE_NOT_APPROVED",
        message: `Publishable locale ${locale.canonical} has ${surface} review state ${String(
          review[surface],
        )}`,
        expected: "approved",
        actual: String(review[surface]),
      });
    }
  }
}

function validateRequiredSurfaceFiles(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  errors: LocaleValidationError[],
): void {
  const checkedPaths = new Set<string>();

  for (const surface of locale.requiredSurfaces) {
    switch (surface) {
      case "frontend":
        for (const fileName of frontendLocaleFiles) {
          requireFrontendFile(repoRoot, locale, "frontend", fileName, errors, checkedPaths);
        }
        break;
      case "apiErrors":
      case "backendValidation":
        requireFrontendFile(
          repoRoot,
          locale,
          surface,
          "apiErrors.json",
          errors,
          checkedPaths,
        );
        break;
      case "guest":
        requireFrontendFile(
          repoRoot,
          locale,
          surface,
          "common.json",
          errors,
          checkedPaths,
        );
        requireFrontendFile(
          repoRoot,
          locale,
          surface,
          "routes.json",
          errors,
          checkedPaths,
        );
        break;
      case "guestStorefront":
        // Guest-only locales (operatorLocale: false) ship a single JSON bundle
        // for the public storefront/menu surface — see
        // frontend/src/i18n/GuestTranslationProvider.tsx dynamic import. The
        // canonical code (NOT the path segment) is the file name on disk.
        requirePath(
          repoRoot,
          locale,
          "guestStorefront",
          path.join(
            repoRoot,
            "frontend",
            "src",
            "i18n",
            "guest-messages",
            `${locale.canonical}.json`,
          ),
          "file",
          "MISSING_GUEST_STOREFRONT_BUNDLE",
          errors,
          checkedPaths,
        );
        break;
      case "emails":
        requirePath(
          repoRoot,
          locale,
          "emails",
          path.join(
            repoRoot,
            "backend",
            "email",
            "layout",
            `base_${locale.emailFamily}.html`,
          ),
          "file",
          "MISSING_REQUIRED_LOCALE_FILE",
          errors,
          checkedPaths,
        );
        requirePath(
          repoRoot,
          locale,
          "emails",
          path.join(
            repoRoot,
            "backend",
            "email",
            "templates",
            locale.emailFamily,
          ),
          "directory",
          "MISSING_REQUIRED_LOCALE_FILE",
          errors,
          checkedPaths,
        );
        break;
      case "prompts":
        requirePath(
          repoRoot,
          locale,
          "prompts",
          path.join(
            repoRoot,
            "backend",
            "internal",
            "services",
            "prompts",
            "menu_wizard",
            `${locale.promptFamily}.md`,
          ),
          "file",
          "MISSING_REQUIRED_LOCALE_FILE",
          errors,
          checkedPaths,
        );
        requirePath(
          repoRoot,
          locale,
          "prompts",
          path.join(
            repoRoot,
            "backend",
            "internal",
            "services",
            "prompts",
            "director_console",
            `${locale.promptFamily}.md`,
          ),
          "file",
          "MISSING_REQUIRED_LOCALE_FILE",
          errors,
          checkedPaths,
        );
        // AI-waiter prompt assets. A locale that declares the `prompts`
        // surface must ship reviewed (active, NOT review_pending) waiter
        // prompts for every mode plus a greeting asset — otherwise the Go
        // contract test is the only thing standing between a stale stub and
        // production. The waiter prompt families key off promptFamily
        // (e.g. es_ar); the greeting asset keys off the canonical code
        // (e.g. es-AR) — matching the Go loaders in waiter_prompts.go /
        // waiter_greeting.go.
        for (const waiterMode of ["concierge", "ordering", "whatsapp"]) {
          requireReviewedPromptFile(
            repoRoot,
            locale,
            path.join(
              repoRoot,
              "backend",
              "internal",
              "services",
              "prompts",
              "ai_waiter",
              `${waiterMode}_${locale.promptFamily}.md`,
            ),
            errors,
            checkedPaths,
          );
        }
        requirePath(
          repoRoot,
          locale,
          "prompts",
          path.join(
            repoRoot,
            "backend",
            "internal",
            "services",
            "prompts",
            "ai_waiter",
            "greetings",
            `${locale.canonical}.md`,
          ),
          "file",
          "MISSING_REQUIRED_LOCALE_FILE",
          errors,
          checkedPaths,
        );
        break;
    }
  }
}

function requireFrontendFile(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  surface: string,
  fileName: string,
  errors: LocaleValidationError[],
  checkedPaths: Set<string>,
): void {
  requirePath(
    repoRoot,
    locale,
    surface,
    path.join(
      repoRoot,
      "frontend",
      "src",
      "i18n",
      "locales",
      locale.sourceFolder,
      fileName,
    ),
    "file",
    "MISSING_FRONTEND_LOCALE_FILE",
    errors,
    checkedPaths,
  );
}

function requirePath(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  surface: string,
  targetPath: string,
  targetType: "file" | "directory",
  code: string,
  errors: LocaleValidationError[],
  checkedPaths: Set<string>,
): void {
  const checkKey = `${targetType}:${targetPath}`;

  if (checkedPaths.has(checkKey)) {
    return;
  }

  checkedPaths.add(checkKey);

  if (pathExistsAs(targetPath, targetType)) {
    return;
  }

  errors.push({
    locale: locale.canonical,
    surface,
    code,
    message: `Missing ${targetType} for ${locale.canonical}: ${relativePath(
      repoRoot,
      targetPath,
    )}`,
    path: relativePath(repoRoot, targetPath),
  });
}

// requireReviewedPromptFile asserts an AI-waiter prompt asset exists AND is in
// a valid review state for a locale that declares the `prompts` surface: the
// file must be present and must NOT carry `review_pending: true` front-matter.
// A review_pending stub serves the English body via the Go loader's fallback,
// so a locale that claims `prompts` coverage while still shipping a pending
// stub is mis-declared — the validator surfaces that instead of relying solely
// on the backend contract test.
function requireReviewedPromptFile(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  targetPath: string,
  errors: LocaleValidationError[],
  checkedPaths: Set<string>,
): void {
  const checkKey = `prompt-review:${targetPath}`;
  if (checkedPaths.has(checkKey)) {
    return;
  }
  checkedPaths.add(checkKey);

  if (!pathExistsAs(targetPath, "file")) {
    errors.push({
      locale: locale.canonical,
      surface: "prompts",
      code: "MISSING_REQUIRED_LOCALE_FILE",
      message: `Missing file for ${locale.canonical}: ${relativePath(
        repoRoot,
        targetPath,
      )}`,
      path: relativePath(repoRoot, targetPath),
    });
    return;
  }

  if (isReviewPendingPrompt(targetPath)) {
    errors.push({
      locale: locale.canonical,
      surface: "prompts",
      code: "PROMPT_REVIEW_PENDING",
      message: `Prompt asset for ${locale.canonical} is still review_pending and cannot back the prompts surface: ${relativePath(
        repoRoot,
        targetPath,
      )}`,
      path: relativePath(repoRoot, targetPath),
    });
  }
}

// isReviewPendingPrompt reports whether a prompt markdown file's leading
// front-matter block declares `review_pending: true`. Mirrors
// parseReviewPending in backend/internal/services/waiter_prompts.go.
function isReviewPendingPrompt(targetPath: string): boolean {
  let raw: string;
  try {
    raw = fs.readFileSync(targetPath, "utf8");
  } catch {
    return false;
  }

  const trimmed = raw.replace(/^[﻿ \t\r\n]+/, "");
  if (!trimmed.startsWith("---")) {
    return false;
  }
  const rest = trimmed.slice(3);
  const end = rest.indexOf("---");
  if (end < 0) {
    return false;
  }
  return /review_pending:\s*true/i.test(rest.slice(0, end));
}

function pathExistsAs(targetPath: string, targetType: "file" | "directory"): boolean {
  try {
    const stat = fs.statSync(targetPath);

    return targetType === "file" ? stat.isFile() : stat.isDirectory();
  } catch {
    return false;
  }
}

function isPublishable(locale: LocaleRegistryEntry, status: LocaleStatus): boolean {
  return locale.publishable || status.publishable === true;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function relativePath(repoRoot: string, targetPath: string): string {
  return path.relative(repoRoot, targetPath);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function printEntry(prefix: "[error]" | "[warn]", entry: LocaleValidationError): void {
  const pathSuffix = entry.path ? ` (${entry.path})` : "";
  console.log(
    `${prefix} ${entry.locale} ${entry.surface} ${entry.code}: ${entry.message}${pathSuffix}`,
  );
}

function main(): void {
  const result = runLocaleValidation({ repoRoot: repoRootFromFrontend() });

  for (const warning of result.warnings) {
    printEntry("[warn]", warning);
  }

  for (const error of result.errors) {
    printEntry("[error]", error);
  }

  process.exitCode = result.ok ? 0 : 1;
}

if (
  process.argv[1] &&
  /^validate-locales\.[jt]s$/.test(path.basename(process.argv[1]))
) {
  main();
}
