import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { type LocaleRegistry, type LocaleRegistryEntry } from "../common";
import { scaffoldLocale } from "../scaffold-locale";

const requiredSurfaces = [
  "frontend",
  "apiErrors",
  "emails",
  "prompts",
  "backendValidation",
  "guest",
  "guestStorefront",
];

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
    requiredSurfaces,
    ...overrides,
  };
}

// The scaffold seeds the operator messages tree and the guest bundle from the
// real `en` source so the new locale starts as a structurally-complete copy.
// Tests therefore stage a minimal-but-real en messages tree + guest bundle in
// the temp repo before scaffolding.
function seedEnSourceAssets(
  root: string,
  enFolder: string,
  namespaces: Record<string, unknown>,
): void {
  const enMessagesDir = path.join(
    root,
    "frontend",
    "src",
    "i18n",
    "messages",
    enFolder,
  );
  fs.mkdirSync(enMessagesDir, { recursive: true });

  const importLines: string[] = [];
  const barrelKeys: string[] = [];
  for (const [name, value] of Object.entries(namespaces)) {
    fs.writeFileSync(
      path.join(enMessagesDir, `${name}.json`),
      `${JSON.stringify(value, null, 2)}\n`,
    );
    importLines.push(`import ${name} from "./${name}.json";`);
    barrelKeys.push(name);
  }
  fs.writeFileSync(
    path.join(enMessagesDir, "index.ts"),
    `${importLines.join("\n")}\n\nconst messages = {\n${barrelKeys
      .map((key) => `  ${key},`)
      .join("\n")}\n};\n\nexport default messages;\n`,
  );

  const guestDir = path.join(root, "frontend", "src", "i18n", "guest-messages");
  fs.mkdirSync(guestDir, { recursive: true });
  fs.writeFileSync(
    path.join(guestDir, "en.json"),
    `${JSON.stringify({ menu: { addToOrder: "Add to order" } }, null, 2)}\n`,
  );
}

function writeTempRegistry(
  registry: LocaleRegistry,
  seedAssets = false,
): string {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), "locale-scaffold-"));
  fs.mkdirSync(path.join(tempRoot, "locales"), { recursive: true });
  fs.writeFileSync(
    path.join(tempRoot, "locales", "registry.json"),
    JSON.stringify(registry),
  );

  if (seedAssets) {
    const enFolder = registry.locales[registry.defaultLocale].sourceFolder;
    seedEnSourceAssets(tempRoot, enFolder, {
      common: { hello: "Hello" },
      navigation: { home: "Home" },
    });
  }

  return tempRoot;
}

describe("scaffoldLocale", () => {
  test("creates the missing locale scaffold from registry metadata without overwriting files", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          it: validLocale("it", {
            displayName: "Italian",
            nativeName: "Italiano",
          }),
        },
      },
      true,
    );
    const existingCommonPath = path.join(
      root,
      "frontend",
      "src",
      "i18n",
      "locales",
      "it",
      "common.json",
    );
    fs.mkdirSync(path.dirname(existingCommonPath), { recursive: true });
    fs.writeFileSync(existingCommonPath, JSON.stringify({ reviewed: true }));

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    expect(fs.existsSync(path.join(root, "locales", "it", "status.json"))).toBe(
      true,
    );
    expect(fs.existsSync(existingCommonPath)).toBe(true);
    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "locales", "it", "apiErrors.json"),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "locales", "it", "seo.json"),
      ),
    ).toBe(false);
    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "locales", "it", "routes.json"),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(path.join(root, "backend", "email", "templates", "it")),
    ).toBe(true);
    expect(
      fs.existsSync(path.join(root, "backend", "email", "layout", "base_it.html")),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(
          root,
          "backend",
          "internal",
          "services",
          "prompts",
          "menu_wizard",
          "it.md",
        ),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(
          root,
          "backend",
          "internal",
          "services",
          "prompts",
          "director_console",
          "it.md",
        ),
      ),
    ).toBe(true);
    expect(fs.existsSync(path.join(root, "frontend", "src", "content", "blog"))).toBe(
      false,
    );

    expect(JSON.parse(fs.readFileSync(existingCommonPath, "utf8"))).toEqual({
      reviewed: true,
    });
    expect(
      JSON.parse(
        fs.readFileSync(path.join(root, "locales", "it", "status.json"), "utf8"),
      ),
    ).toEqual({
      canonical: "it",
      pathSegment: "it",
      state: "draft",
      publishable: false,
      aiDraft: Object.fromEntries(
        requiredSurfaces.map((surface) => [surface, "not_started"]),
      ),
      review: Object.fromEntries(
        requiredSurfaces.map((surface) => [surface, "not_reviewed"]),
      ),
      unresolvedQuestions: [],
    });
  });

  test("seeds the operator messages tree (every en namespace + index barrel) for an operator locale", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          it: validLocale("it", {
            displayName: "Italian",
            nativeName: "Italiano",
          }),
        },
      },
      true,
    );

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    const itMessagesDir = path.join(
      root,
      "frontend",
      "src",
      "i18n",
      "messages",
      "it",
    );

    // Every namespace en ships must be present in the new locale's tree.
    expect(fs.existsSync(path.join(itMessagesDir, "common.json"))).toBe(true);
    expect(fs.existsSync(path.join(itMessagesDir, "navigation.json"))).toBe(
      true,
    );
    expect(fs.existsSync(path.join(itMessagesDir, "index.ts"))).toBe(true);

    // The seeded namespace content mirrors en as a starting point.
    expect(
      JSON.parse(fs.readFileSync(path.join(itMessagesDir, "common.json"), "utf8")),
    ).toEqual({ hello: "Hello" });

    // The index barrel imports + exports the same namespaces en's barrel does.
    const barrel = fs.readFileSync(path.join(itMessagesDir, "index.ts"), "utf8");
    expect(barrel).toContain(`import common from "./common.json";`);
    expect(barrel).toContain(`import navigation from "./navigation.json";`);
    expect(barrel).toContain("export default messages;");
  });

  test("seeds the guest-messages bundle from en for a guestStorefront locale", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          it: validLocale("it", {
            displayName: "Italian",
            nativeName: "Italiano",
          }),
        },
      },
      true,
    );

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    // The bundle file name uses the canonical code, NOT the path segment.
    const guestBundle = path.join(
      root,
      "frontend",
      "src",
      "i18n",
      "guest-messages",
      "it.json",
    );
    expect(fs.existsSync(guestBundle)).toBe(true);
    expect(JSON.parse(fs.readFileSync(guestBundle, "utf8"))).toEqual({
      menu: { addToOrder: "Add to order" },
    });
  });

  test("does NOT seed a guest-messages bundle for a locale without the guestStorefront surface", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          it: validLocale("it", {
            displayName: "Italian",
            nativeName: "Italiano",
            requiredSurfaces: ["frontend", "apiErrors"],
          }),
        },
      },
      true,
    );

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "guest-messages", "it.json"),
      ),
    ).toBe(false);
  });

  test("does NOT seed an operator messages tree for a guest-only locale", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          vi: validLocale("vi", {
            displayName: "Vietnamese",
            nativeName: "Tiếng Việt",
            operatorLocale: false,
            publishable: false,
            requiredSurfaces: ["guestStorefront"],
          }),
        },
      },
      true,
    );

    scaffoldLocale({ repoRoot: root, canonical: "vi" });

    // Guest-only locale: no operator messages tree, but a guest bundle exists.
    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "messages", "vi"),
      ),
    ).toBe(false);
    expect(
      fs.existsSync(
        path.join(root, "frontend", "src", "i18n", "guest-messages", "vi.json"),
      ),
    ).toBe(true);
  });

  test("is idempotent: re-scaffolding does not overwrite existing seeded files", () => {
    const root = writeTempRegistry(
      {
        version: 1,
        defaultLocale: "en",
        locales: {
          en: validLocale("en", {
            displayName: "English",
            nativeName: "English",
          }),
          it: validLocale("it", {
            displayName: "Italian",
            nativeName: "Italiano",
          }),
        },
      },
      true,
    );

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    // Operator + edit a seeded namespace to simulate a real translation pass.
    const commonPath = path.join(
      root,
      "frontend",
      "src",
      "i18n",
      "messages",
      "it",
      "common.json",
    );
    fs.writeFileSync(commonPath, JSON.stringify({ hello: "Ciao" }));
    const guestBundle = path.join(
      root,
      "frontend",
      "src",
      "i18n",
      "guest-messages",
      "it.json",
    );
    fs.writeFileSync(guestBundle, JSON.stringify({ menu: { translated: true } }));

    scaffoldLocale({ repoRoot: root, canonical: "it" });

    expect(JSON.parse(fs.readFileSync(commonPath, "utf8"))).toEqual({
      hello: "Ciao",
    });
    expect(JSON.parse(fs.readFileSync(guestBundle, "utf8"))).toEqual({
      menu: { translated: true },
    });
  });
});
