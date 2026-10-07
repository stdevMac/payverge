import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { type LocaleRegistry, type LocaleRegistryEntry } from "../common";
import { runLocaleValidation } from "../validate-locales";

const frontendFiles = ["common.json", "apiErrors.json", "routes.json"];

function validLocale(
  canonical: string,
  overrides: Partial<LocaleRegistryEntry> = {},
): LocaleRegistryEntry {
  return {
    canonical,
    pathSegment: canonical.toLowerCase(),
    sourceFolder: canonical.toLowerCase(),
    displayName: canonical,
    nativeName: canonical,
    direction: "ltr",
    emailFamily: canonical.toLowerCase().replace("-", "_"),
    promptFamily: canonical.toLowerCase().replace("-", "_"),
    translationProviderTarget: canonical,
    publicRoutes: canonical === "en" ? "default" : "prefixed",
    operatorLocale: true,
    guestLocale: true,
    publishable: canonical === "en",
    requiredSurfaces: ["frontend"],
    ...overrides,
  };
}

function writeTempRegistry(registry: LocaleRegistry): string {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "locale-validation-"));
  fs.mkdirSync(path.join(tempRoot, "locales"), { recursive: true });
  fs.writeFileSync(
    path.join(tempRoot, "locales", "registry.json"),
    JSON.stringify(registry),
  );

  return tempRoot;
}

function writeStatus(
  repoRoot: string,
  locale: LocaleRegistryEntry,
  overrides: Record<string, unknown> = {},
): void {
  const statusPath = path.join(
    repoRoot,
    "locales",
    locale.pathSegment,
    "status.json",
  );
  fs.mkdirSync(path.dirname(statusPath), { recursive: true });
  fs.writeFileSync(
    statusPath,
    JSON.stringify({
      canonical: locale.canonical,
      pathSegment: locale.pathSegment,
      state: "draft",
      publishable: locale.publishable,
      aiDraft: Object.fromEntries(
        locale.requiredSurfaces.map((surface) => [surface, "not_started"]),
      ),
      reviewOwner: "Test language owner",
      reviewDate: "2026-08-01",
      review: Object.fromEntries(
        locale.requiredSurfaces.map((surface) => [surface, "approved"]),
      ),
      unresolvedQuestions: [],
      ...overrides,
    }),
  );
}

function writeFrontendLocaleFiles(
  repoRoot: string,
  locale: LocaleRegistryEntry,
): void {
  const localeRoot = path.join(
    repoRoot,
    "frontend",
    "src",
    "i18n",
    "locales",
    locale.sourceFolder,
  );
  fs.mkdirSync(localeRoot, { recursive: true });

  for (const fileName of frontendFiles) {
    fs.writeFileSync(path.join(localeRoot, fileName), "{}\n");
  }
}

function writeSourceFile(repoRoot: string, relativePath: string, contents: string): void {
  const filePath = path.join(repoRoot, relativePath);
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, contents);
}

describe("runLocaleValidation", () => {
  test("reports missing frontend locale files for a publishable registry locale", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en);

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(false);
    expect(result.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "en",
          surface: "frontend",
          code: "MISSING_FRONTEND_LOCALE_FILE",
        }),
      ]),
    );
  });

  test("blocks publishable locales with unresolved review questions", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en, {
      unresolvedQuestions: ["Confirm legal copy tone."],
    });
    writeFrontendLocaleFiles(repoRoot, en);

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(false);
    expect(result.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "en",
          surface: "review",
          code: "UNRESOLVED_REVIEW_QUESTIONS",
        }),
      ]),
    );
  });

  test("blocks publishable locales without a recorded language owner and review date", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en, {
      reviewOwner: undefined,
      reviewDate: undefined,
    });
    writeFrontendLocaleFiles(repoRoot, en);

    const result = runLocaleValidation({ repoRoot });

    expect(result.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ code: "MISSING_REVIEW_OWNER" }),
        expect.objectContaining({ code: "MISSING_REVIEW_DATE" }),
      ]),
    );
  });

  test("reports registry and status publishable mismatches as warnings only", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en, { publishable: false });
    writeFrontendLocaleFiles(repoRoot, en);

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.errors).toEqual([]);
    expect(result.warnings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "en",
          surface: "status",
          code: "PUBLISHABLE_MISMATCH",
        }),
      ]),
    );
  });

  test("blocks public/operator hardcoded user-facing JSX text", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en);
    writeFrontendLocaleFiles(repoRoot, en);
    writeSourceFile(
      repoRoot,
      "frontend/src/components/business/Hardcoded.tsx",
      "export function Hardcoded() { return <button>Save Changes</button>; }\n",
    );

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(false);
    expect(result.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          locale: "*",
          surface: "frontend",
          code: "HARDCODED_USER_FACING_TEXT",
        }),
      ]),
    );
  });

  test("ignores tests and admin-only files in hardcoded-string validation", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en);
    writeFrontendLocaleFiles(repoRoot, en);
    writeSourceFile(
      repoRoot,
      "frontend/src/components/business/__tests__/Fixture.test.tsx",
      "export function Fixture() { return <button>Save Changes</button>; }\n",
    );
    writeSourceFile(
      repoRoot,
      "frontend/src/components/business/shared/ActivationPanel.test.tsx",
      "export function Fixture() { return <button>Activate</button>; }\n",
    );
    writeSourceFile(
      repoRoot,
      "frontend/src/app/admin/page.tsx",
      "export default function AdminPage() { return <h1>Admin Dashboard</h1>; }\n",
    );

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.errors).toEqual([]);
  });

  test("allows non-translatable brand literals", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en);
    writeFrontendLocaleFiles(repoRoot, en);
    writeSourceFile(
      repoRoot,
      "frontend/src/components/business-page/Footer.tsx",
      "export function Footer() { return <span>Payverge</span>; }\n",
    );

    const result = runLocaleValidation({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.errors).toEqual([]);
  });

  test("waives explicitly allow-listed path:text strings while still catching new text", () => {
    const en = validLocale("en", { publishable: true });
    const repoRoot = writeTempRegistry({
      version: 1,
      defaultLocale: "en",
      locales: { en },
    });
    writeStatus(repoRoot, en);
    writeFrontendLocaleFiles(repoRoot, en);
    // The production per-file/per-string allow-list is intentionally empty (the
    // real DashboardSidebar's "Printers" was localized in 111ce8cda, so no live
    // entry is needed). Inject a self-contained waiver to exercise the mechanism
    // without depending on production config. "Waived Tab" is allow-listed for
    // this exact file; a sibling non-allow-listed string in the SAME file MUST
    // still be reported — the allow-list is per path:exactText, not a blanket
    // file/dir ignore.
    writeSourceFile(
      repoRoot,
      "frontend/src/components/business/DashboardSidebar.tsx",
      "export function Side() { return (<nav><span>Waived Tab</span><span>Brand New Tab</span></nav>); }\n",
    );

    const result = runLocaleValidation({
      repoRoot,
      extraAllowedHardcodedStrings: [
        {
          path: "frontend/src/components/business/DashboardSidebar.tsx",
          text: "Waived Tab",
          reason: "test fixture: exercise the per-file/per-string waiver",
        },
      ],
    });

    const hardcoded = result.errors.filter(
      (e) => e.code === "HARDCODED_USER_FACING_TEXT",
    );
    // "Waived Tab" is waived; "Brand New Tab" is not.
    expect(
      hardcoded.some((e) => e.message.includes("Waived Tab")),
    ).toBe(false);
    expect(
      hardcoded.some((e) => e.message.includes("Brand New Tab")),
    ).toBe(true);
  });
});
