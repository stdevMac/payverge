import fs from "node:fs";
import os from "node:os";
import path from "path";

import {
  canonicalToSegment,
  type LocaleRegistry,
  type LocaleRegistryEntry,
  readRegistry,
  segmentToCanonical,
} from "../common";

const repoRoot = path.resolve(__dirname, "../../../..");
const validSurfaces = [
  "frontend",
  "apiErrors",
  "emails",
  "prompts",
  "backendValidation",
  "guest",
];

function validLocale(
  canonical: string,
  overrides: Partial<LocaleRegistryEntry> = {},
): LocaleRegistryEntry {
  return {
    canonical,
    pathSegment: canonical.toLowerCase(),
    sourceFolder: canonical.toLowerCase(),
    displayName: "English",
    nativeName: "English",
    direction: "ltr",
    emailFamily: canonical.replace("-", "_").toLowerCase(),
    promptFamily: canonical.replace("-", "_").toLowerCase(),
    translationProviderTarget: canonical,
    publicRoutes: canonical === "en" ? "default" : "prefixed",
    operatorLocale: true,
    guestLocale: true,
    publishable: true,
    requiredSurfaces: validSurfaces,
    ...overrides,
  };
}

function writeTempRegistry(registry: LocaleRegistry): string {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "locale-registry-"));
  fs.mkdirSync(path.join(tempRoot, "locales"));
  fs.writeFileSync(
    path.join(tempRoot, "locales", "registry.json"),
    JSON.stringify(registry),
  );

  return tempRoot;
}

describe("i18n common registry helpers", () => {
  test("reads the locale registry defaults and es-AR metadata", () => {
    const registry = readRegistry(repoRoot);

    expect(registry.defaultLocale).toBe("en");
    expect(registry.locales["es-AR"].pathSegment).toBe("es-ar");
    expect(registry.locales["es-AR"].emailFamily).toBe("es_ar");
    expect(registry.locales["es-AR"].promptFamily).toBe("es_ar");
  });

  test("maps between canonical locale codes and path segments", () => {
    const registry = readRegistry(repoRoot);

    expect(canonicalToSegment(registry, "es-AR")).toBe("es-ar");
    expect(segmentToCanonical(registry, "es-ar")).toBe("es-AR");
  });

  test("rejects locale entries with invalid metadata beyond path and family patterns", () => {
    const tempRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en", {
          sourceFolder: "",
          direction: "sideways" as LocaleRegistryEntry["direction"],
          requiredSurfaces: [],
        }),
      },
    });

    expect(() => readRegistry(tempRoot)).toThrow(
      "Locale en has invalid sourceFolder",
    );
  });

  test("rejects unsafe sourceFolder values", () => {
    const tempRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en", { sourceFolder: "../x" }),
      },
    });

    expect(() => readRegistry(tempRoot)).toThrow(
      "Locale en has invalid sourceFolder: ../x",
    );
  });

  test("rejects duplicate path segments", () => {
    const tempRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en"),
        es: validLocale("es", { pathSegment: "en" }),
      },
    });

    expect(() => readRegistry(tempRoot)).toThrow(
      "Duplicate locale pathSegment en",
    );
  });

  test("rejects duplicate source folders", () => {
    const tempRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en"),
        es: validLocale("es", { sourceFolder: "en" }),
      },
    });

    expect(() => readRegistry(tempRoot)).toThrow(
      "Duplicate locale sourceFolder en",
    );
  });

  test("rejects unsupported or duplicate required surfaces", () => {
    const unsupportedSurfaceRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en", { requiredSurfaces: ["frontend", "mobile"] }),
      },
    });
    const duplicateSurfaceRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: {
        en: validLocale("en", { requiredSurfaces: ["frontend", "frontend"] }),
      },
    });

    expect(() => readRegistry(unsupportedSurfaceRoot)).toThrow(
      "Locale en has unsupported requiredSurface: mobile",
    );
    expect(() => readRegistry(duplicateSurfaceRoot)).toThrow(
      "Locale en has duplicate requiredSurface: frontend",
    );
  });
});
