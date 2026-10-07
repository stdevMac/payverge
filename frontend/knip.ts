// knip configuration (https://knip.dev). TypeScript rather than JSON so the
// `knip --production` run (npm run knip:production) can add the entries and
// ignores that only that mode needs: the default run (npm run knip, gated in
// CI) would report each of them as a redundant configuration hint.
const production =
  process.argv.includes("--production") || process.argv.includes("-p");

// Next.js App Router, Jest, Playwright, ESLint, PostCSS and Tailwind entry
// points come from knip's built-in plugins. Only non-convention entries are
// listed here. A trailing "!" marks a production entry (`knip --production`).
const entry = [
  // Design-system barrel for the design-sync bundle; its re-exports are the
  // public surface, so the components it names are used.
  "ds/index.tsx!",
  "ds/build.mjs",
  "ds/tailwind.config.ts",
  "ds/*.css",
  // webpack alias target in next.config.mjs.
  "src/empty-polyfill.js!",
  // Type-level tests: checked by `npm run typecheck`, never imported.
  "src/**/*.typecheck.ts",
  // Run by CI (.github/workflows/ci.yml), the pre-commit hook or by hand.
  "scripts/check-*.{js,ts}",
  "scripts/backfill-guest-locale-keys.js",
  "scripts/i18n/*.ts",
  // Playwright reporter loaded by path from playwright.config.ts.
  "tests/helpers/no-skipped-tests-reporter.ts",
];

// knip treats next.config.mjs and tailwind.config.ts as plugin config and
// package.json predev/prebuild as dev scripts; `knip --production` follows
// none of them. Without these, it reports the alias redirects that
// next.config.mjs spreads into redirects(), the PDF-worker build hook and the
// tailwind plugins as unused, and deleting them breaks the app.
const productionEntry = [
  "src/config/routeAliasRedirects.cjs!",
  "scripts/copy-pdf-worker.mjs!",
];

const project = [
  "**/*.{js,jsx,ts,tsx,mjs,cjs,css}!",
  "!.next/**",
  "!public/**",
  // Test-only support code. The default run checks that tests use it;
  // `knip --production` excludes test files, so it would report all of it.
  "!jest.setup.js!",
  "!test/**!",
  "!tests/**!",
  "!src/test/**!",
  "!**/__fixtures__/**!",
  "!src/lib/tenantIsolationProbe.ts!",
  "!src/components/business/offerModalHitTest.ts!",
];

const ignoreDependencies = [
  // Type-only import; it must stay the exact version @sentry/nextjs pins, so
  // it is not declared separately (a range would install a second copy).
  "@sentry/core",
  // Loaded by Next only when NEXT_OPTIMIZE_CSS=1 (docs/self-hosting/configuration.md).
  "critters",
  // webpack externals in next.config.mjs (optional peers of wallet SDKs).
  "encoding",
  "pino-pretty",
  // Resolved by name through FlatCompat: compat.extends("next/core-web-vitals").
  "eslint-config-next",
  // Pins the jsdom that jest-environment-jsdom resolves.
  "jsdom",
];

const productionIgnoreDependencies = [
  // Next's image optimizer loads it at runtime in the standalone server.
  "sharp",
  // Plugin in tailwind.config.ts (scrollbar-* utilities in six components).
  "tailwind-scrollbar",
];

const config = {
  $schema: "https://unpkg.com/knip@6/schema.json",
  entry: production ? [...entry, ...productionEntry] : entry,
  project,
  ignoreDependencies: production
    ? [...ignoreDependencies, ...productionIgnoreDependencies]
    : ignoreDependencies,
  // `export function X` + `export default X` pairs are a house style in many
  // components; reported as warnings until imports are normalised.
  rules: { duplicates: "warn" as const },
  // External CLI the Playwright helpers shell out to for test fixtures.
  ignoreBinaries: ["psql"],
};

export default config;
