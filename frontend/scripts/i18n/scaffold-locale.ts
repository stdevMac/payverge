import fs from "node:fs";
import path from "node:path";

import {
  readRegistry,
  repoRootFromFrontend,
  type LocaleRegistryEntry,
} from "./common";

interface ScaffoldLocaleOptions {
  repoRoot: string;
  canonical: string;
}

interface LocaleStatus {
  canonical: string;
  pathSegment: string;
  state: "draft";
  publishable: false;
  aiDraft: Record<string, "not_started">;
  review: Record<string, "not_reviewed">;
  unresolvedQuestions: string[];
}

export function scaffoldLocale(opts: ScaffoldLocaleOptions): void {
  const registry = readRegistry(opts.repoRoot);
  const locale = registry.locales[opts.canonical];

  if (!locale) {
    throw new Error(`Unknown locale canonical: ${opts.canonical}`);
  }

  const defaultLocale = registry.locales[registry.defaultLocale];

  writeMissingJson(
    path.join(opts.repoRoot, "locales", locale.pathSegment, "status.json"),
    buildStatus(locale.canonical, locale.pathSegment, locale.requiredSurfaces),
  );

  if (locale.operatorLocale) {
    scaffoldOperatorMessages(opts.repoRoot, defaultLocale, locale);
  }

  if (locale.requiredSurfaces.includes("guestStorefront")) {
    scaffoldGuestMessages(opts.repoRoot, defaultLocale, locale);
  }

  for (const fileName of ["common.json", "apiErrors.json", "routes.json"]) {
    writeMissingFile(
      path.join(
        opts.repoRoot,
        "frontend",
        "src",
        "i18n",
        "locales",
        locale.sourceFolder,
        fileName,
      ),
      "{}\n",
    );
  }

  writeMissingFile(
    path.join(
      opts.repoRoot,
      "backend",
      "email",
      "layout",
      `base_${locale.emailFamily}.html`,
    ),
    emailLayoutTemplate(locale.pathSegment),
  );
  fs.mkdirSync(
    path.join(opts.repoRoot, "backend", "email", "templates", locale.emailFamily),
    { recursive: true },
  );

  writeMissingFile(
    path.join(
      opts.repoRoot,
      "backend",
      "internal",
      "services",
      "prompts",
      "menu_wizard",
      `${locale.promptFamily}.md`,
    ),
    promptTemplate("Menu wizard", locale.nativeName),
  );
  writeMissingFile(
    path.join(
      opts.repoRoot,
      "backend",
      "internal",
      "services",
      "prompts",
      "director_console",
      `${locale.promptFamily}.md`,
    ),
    promptTemplate("Director console", locale.nativeName),
  );
}

function buildStatus(
  canonical: string,
  pathSegment: string,
  requiredSurfaces: string[],
): LocaleStatus {
  return {
    canonical,
    pathSegment,
    state: "draft",
    publishable: false,
    aiDraft: Object.fromEntries(
      requiredSurfaces.map((surface) => [surface, "not_started"]),
    ),
    review: Object.fromEntries(
      requiredSurfaces.map((surface) => [surface, "not_reviewed"]),
    ),
    unresolvedQuestions: [],
  };
}

// scaffoldOperatorMessages seeds frontend/src/i18n/messages/<src>/ for a new
// operator locale by copying every namespace JSON the default (en) locale ships
// plus a matching index.ts barrel. Copying en's content (rather than empty
// stubs) gives translators a structurally-complete, valid starting point and
// keeps the operator-locale completeness guard (OP-6) green from the first
// scaffold. Idempotent: existing files are never overwritten, so a partial
// translation pass survives a re-scaffold.
function scaffoldOperatorMessages(
  repoRoot: string,
  defaultLocale: LocaleRegistryEntry,
  locale: LocaleRegistryEntry,
): void {
  const sourceDir = messagesDir(repoRoot, defaultLocale.sourceFolder);

  if (!fs.existsSync(sourceDir)) {
    // No source tree to copy from (e.g. a stripped-down test fixture). Nothing
    // to seed — the operator validators will surface the gap.
    return;
  }

  const targetDir = messagesDir(repoRoot, locale.sourceFolder);
  const namespaces = fs
    .readdirSync(sourceDir)
    .filter((name) => name.endsWith(".json"))
    .map((name) => name.slice(0, -".json".length))
    .sort((left, right) => (left < right ? -1 : left > right ? 1 : 0));

  for (const namespace of namespaces) {
    writeMissingFile(
      path.join(targetDir, `${namespace}.json`),
      fs.readFileSync(path.join(sourceDir, `${namespace}.json`), "utf8"),
    );
  }

  writeMissingFile(
    path.join(targetDir, "index.ts"),
    buildMessagesBarrel(namespaces),
  );
}

// buildMessagesBarrel mirrors the existing messages/<src>/index.ts pattern:
// one default-import per namespace JSON, collected into a `messages` object,
// exported as default.
function buildMessagesBarrel(namespaces: string[]): string {
  const imports = namespaces
    .map((namespace) => `import ${namespace} from "./${namespace}.json";`)
    .join("\n");
  const entries = namespaces.map((namespace) => `  ${namespace},`).join("\n");

  return `${imports}\n\nconst messages = {\n${entries}\n};\n\nexport default messages;\n`;
}

// scaffoldGuestMessages seeds frontend/src/i18n/guest-messages/<code>.json from
// the default (en) guest bundle for a locale that declares the guestStorefront
// surface. The file name is the CANONICAL code (es-AR.json, not es-ar.json) —
// the guest provider resolves it via dynamic import on the canonical code.
// Idempotent: never overwrites an existing bundle.
function scaffoldGuestMessages(
  repoRoot: string,
  defaultLocale: LocaleRegistryEntry,
  locale: LocaleRegistryEntry,
): void {
  const sourceBundle = guestBundlePath(repoRoot, defaultLocale.canonical);

  if (!fs.existsSync(sourceBundle)) {
    return;
  }

  writeMissingFile(
    guestBundlePath(repoRoot, locale.canonical),
    fs.readFileSync(sourceBundle, "utf8"),
  );
}

function messagesDir(repoRoot: string, sourceFolder: string): string {
  return path.join(
    repoRoot,
    "frontend",
    "src",
    "i18n",
    "messages",
    sourceFolder,
  );
}

function guestBundlePath(repoRoot: string, canonical: string): string {
  return path.join(
    repoRoot,
    "frontend",
    "src",
    "i18n",
    "guest-messages",
    `${canonical}.json`,
  );
}

function writeMissingJson(filePath: string, value: unknown): void {
  writeMissingFile(filePath, `${JSON.stringify(value, null, 2)}\n`);
}

function writeMissingFile(filePath: string, contents: string): void {
  if (fs.existsSync(filePath)) {
    return;
  }

  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, contents);
}

function emailLayoutTemplate(pathSegment: string): string {
  return `<!DOCTYPE html>
<html lang="${pathSegment}">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Payverge</title>
</head>
<body>
  {{ template "content" . }}
</body>
</html>
`;
}

function promptTemplate(surface: string, nativeName: string): string {
  return `# ${surface} ${nativeName} draft instructions

Write concise, hospitality-focused responses in ${nativeName}. Keep terminology consistent with Payverge product language and flag uncertain translations for review.
`;
}

function parseLocaleArg(args: string[]): string {
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];

    if (arg === "--locale") {
      const locale = args[index + 1];

      if (!locale) {
        throw new Error("Missing value for --locale");
      }

      return locale;
    }

    if (arg.startsWith("--locale=")) {
      const locale = arg.slice("--locale=".length);

      if (!locale) {
        throw new Error("Missing value for --locale");
      }

      return locale;
    }
  }

  throw new Error("Usage: npm run i18n:scaffold -- --locale <locale>");
}

function main(): void {
  const canonical = parseLocaleArg(process.argv.slice(2));

  scaffoldLocale({ repoRoot: repoRootFromFrontend(), canonical });
  console.log(`Scaffolded ${canonical}`);
}

if (process.argv[1] && /^scaffold-locale\.[jt]s$/.test(path.basename(process.argv[1]))) {
  main();
}
