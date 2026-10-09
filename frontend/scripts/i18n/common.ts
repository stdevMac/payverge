import fs from "node:fs";
import path from "node:path";

export type LocaleDirection = "ltr" | "rtl";
export type PublicRouteMode = "default" | "prefixed";

export interface LocaleRegistryEntry {
  canonical: string;
  pathSegment: string;
  sourceFolder: string;
  displayName: string;
  nativeName: string;
  direction: LocaleDirection;
  emailFamily: string;
  promptFamily: string;
  translationProviderTarget: string;
  publicRoutes: PublicRouteMode;
  operatorLocale: boolean;
  guestLocale: boolean;
  publishable: boolean;
  requiredSurfaces: string[];
}

export interface LocaleRegistry {
  version: 1;
  defaultLocale: string;
  locales: Record<string, LocaleRegistryEntry>;
}

const pathSegmentPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const familyPattern = /^[a-z0-9_]+$/;
const stringFields: Array<keyof LocaleRegistryEntry> = [
  "canonical",
  "pathSegment",
  "sourceFolder",
  "displayName",
  "nativeName",
  "direction",
  "emailFamily",
  "promptFamily",
  "translationProviderTarget",
  "publicRoutes",
];
const booleanFields: Array<keyof LocaleRegistryEntry> = [
  "operatorLocale",
  "guestLocale",
  "publishable",
];
// Surface names a locale can declare it ships. "guestStorefront" pins the
// guest-messages JSON requirement for menu-translation-target locales — the
// public storefront UI is the only surface they're expected to localize, and
// `guestLocale: true` without a guest-messages JSON would silently fall back
// to English at runtime.
const allowedRequiredSurfaces = new Set([
  "frontend",
  "apiErrors",
  "emails",
  "prompts",
  "backendValidation",
  "guest",
  "guestStorefront",
]);

export function registryPath(repoRoot: string): string {
  return path.join(repoRoot, "locales", "registry.json");
}

export function readRegistry(repoRoot: string): LocaleRegistry {
  const parsed = JSON.parse(
    fs.readFileSync(registryPath(repoRoot), "utf8"),
  ) as LocaleRegistry;

  validateRegistry(parsed);

  return parsed;
}

export function canonicalToSegment(
  registry: LocaleRegistry,
  canonical: string,
): string {
  const entry = registry.locales[canonical];

  if (!entry) {
    throw new Error(`Unknown locale canonical: ${canonical}`);
  }

  return entry.pathSegment;
}

export function segmentToCanonical(
  registry: LocaleRegistry,
  segment: string,
): string {
  const entry = Object.values(registry.locales).find(
    (locale) => locale.pathSegment === segment,
  );

  if (!entry) {
    throw new Error(`Unknown locale path segment: ${segment}`);
  }

  return entry.canonical;
}

export function repoRootFromFrontend(): string {
  const cwd = process.cwd();

  return path.basename(cwd) === "frontend" ? path.resolve(cwd, "..") : cwd;
}

function validateRegistry(registry: LocaleRegistry): void {
  if (registry.version !== 1) {
    throw new Error(
      `Locale registry has unsupported version: ${registry.version}`,
    );
  }

  if (!registry.locales || typeof registry.locales !== "object") {
    throw new Error("Locale registry must include a locales object");
  }

  if (!registry.locales[registry.defaultLocale]) {
    throw new Error(
      `Default locale is missing from registry: ${registry.defaultLocale}`,
    );
  }

  const pathSegments = new Set<string>();
  const sourceFolders = new Set<string>();

  for (const [canonical, entry] of Object.entries(registry.locales)) {
    validateEntryShape(canonical, entry);

    if (entry.canonical !== canonical) {
      throw new Error(
        `Locale registry key ${canonical} must match canonical ${entry.canonical}`,
      );
    }

    if (!pathSegmentPattern.test(entry.pathSegment)) {
      throw new Error(
        `Locale ${canonical} has invalid pathSegment: ${entry.pathSegment}`,
      );
    }

    if (!pathSegmentPattern.test(entry.sourceFolder)) {
      throw new Error(
        `Locale ${canonical} has invalid sourceFolder: ${entry.sourceFolder}`,
      );
    }

    if (!familyPattern.test(entry.emailFamily)) {
      throw new Error(
        `Locale ${canonical} has invalid emailFamily: ${entry.emailFamily}`,
      );
    }

    if (!familyPattern.test(entry.promptFamily)) {
      throw new Error(
        `Locale ${canonical} has invalid promptFamily: ${entry.promptFamily}`,
      );
    }

    if (pathSegments.has(entry.pathSegment)) {
      throw new Error(`Duplicate locale pathSegment ${entry.pathSegment}`);
    }

    pathSegments.add(entry.pathSegment);

    if (sourceFolders.has(entry.sourceFolder)) {
      throw new Error(`Duplicate locale sourceFolder ${entry.sourceFolder}`);
    }

    sourceFolders.add(entry.sourceFolder);
  }
}

function validateEntryShape(
  canonical: string,
  entry: LocaleRegistryEntry,
): void {
  for (const field of stringFields) {
    const value = entry[field];

    if (typeof value !== "string" || value.length === 0) {
      throw new Error(`Locale ${canonical} has invalid ${field}`);
    }
  }

  if (entry.direction !== "ltr" && entry.direction !== "rtl") {
    throw new Error(
      `Locale ${canonical} has invalid direction: ${entry.direction}`,
    );
  }

  if (entry.publicRoutes !== "default" && entry.publicRoutes !== "prefixed") {
    throw new Error(
      `Locale ${canonical} has invalid publicRoutes: ${entry.publicRoutes}`,
    );
  }

  for (const field of booleanFields) {
    if (typeof entry[field] !== "boolean") {
      throw new Error(`Locale ${canonical} has invalid ${field}`);
    }
  }

  if (
    !Array.isArray(entry.requiredSurfaces) ||
    entry.requiredSurfaces.length === 0 ||
    entry.requiredSurfaces.some(
      (surface) => typeof surface !== "string" || surface.length === 0,
    )
  ) {
    throw new Error(`Locale ${canonical} has invalid requiredSurfaces`);
  }

  const requiredSurfaces = new Set<string>();

  for (const surface of entry.requiredSurfaces) {
    if (!allowedRequiredSurfaces.has(surface)) {
      throw new Error(
        `Locale ${canonical} has unsupported requiredSurface: ${surface}`,
      );
    }

    if (requiredSurfaces.has(surface)) {
      throw new Error(
        `Locale ${canonical} has duplicate requiredSurface: ${surface}`,
      );
    }

    requiredSurfaces.add(surface);
  }
}
