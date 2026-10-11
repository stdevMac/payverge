// eslint-config-next >= 16 ships native flat configs (ESLint 10 dropped eslintrc).
// The app runs Next 15 (package.json "next"), so this is a version skew taken
// on purpose: eslint-config-next 15 only ships eslintrc configs. Its rules
// target Next 16, so one may flag a pattern that is fine on 15. Bump "next" to
// 16 to close the skew, and recheck the rules turned off below when doing so.
import nextCoreWebVitals from "eslint-config-next/core-web-vitals";

const OFF_PALETTE =
  "\\b(blue|indigo|violet|purple|slate|sky|cyan|fuchsia|pink)-(50|100|200|300|400|500|600|700|800|900|950)\\b";

const paletteRules = [
  {
    selector: `Literal[value=/${OFF_PALETTE}/]`,
    message:
      "Off-palette Tailwind color. Use brand/ink/warm/emerald/amber/rose tokens. Legal third-party brand icons (Google, Telegram, WhatsApp) go under src/components/legal-brand-icons/ with a per-file eslint-disable comment.",
  },
  {
    selector: `TemplateElement[value.raw=/${OFF_PALETTE}/]`,
    message: "Off-palette Tailwind color in template literal. See above.",
  },
];

const hexColorRule = {
  selector: "Literal[value=/#(?:[0-9a-fA-F]{3}){1,2}(?:[0-9a-fA-F]{2})?\\b/]",
  message:
    "Inline hex color. Use a Tailwind token (bg-brand, text-ink-900) or add the color to tailwind.config.cjs first.",
};

// Runtime-class public settings (API/site URL, network, RPC, maintenance,
// VAPID, Sentry DSN, PostHog, ...) are resolved at request time through
// src/config/publicConfig.ts (server env / window.__PAYVERGE_ENV__). A literal
// process.env.NEXT_PUBLIC_* read would be frozen into the bundle at build
// time and silently ignore the operator's runtime config. Only the
// build-identity values below may be inlined. See
// docs/self-hosting/frontend-config.md.
const BUILD_TIME_PUBLIC_ENV = "RELEASE_SHA|VERSION|BUILD_TIMESTAMP|SENTRY_RELEASE";
const RUNTIME_PUBLIC_ENV = `/^NEXT_PUBLIC_(?!(${BUILD_TIME_PUBLIC_ENV})$)/`;
const runtimePublicEnvMessage =
  "Runtime-class NEXT_PUBLIC_* values are inlined at build time. Read them via getPublicConfig()/getSiteUrl() from @/config/publicConfig (build-time allowed: NEXT_PUBLIC_RELEASE_SHA, NEXT_PUBLIC_VERSION, NEXT_PUBLIC_BUILD_TIMESTAMP, NEXT_PUBLIC_SENTRY_RELEASE).";
const runtimePublicEnvRules = [
  {
    selector: `MemberExpression[object.type='MemberExpression'][object.object.name='process'][object.property.name='env'][property.name=${RUNTIME_PUBLIC_ENV}]`,
    message: runtimePublicEnvMessage,
  },
  {
    selector: `MemberExpression[object.type='MemberExpression'][object.object.name='process'][object.property.name='env'][computed=true][property.value=${RUNTIME_PUBLIC_ENV}]`,
    message: runtimePublicEnvMessage,
  },
  {
    selector: `VariableDeclarator[init.type='MemberExpression'][init.object.name='process'][init.property.name='env'] > ObjectPattern > Property[key.name=${RUNTIME_PUBLIC_ENV}]`,
    message: runtimePublicEnvMessage,
  },
];

const eslintConfig = [
  ...nextCoreWebVitals,
  {
    // Same file set as the Next config that registers the plugins these rules use.
    files: ["**/*.{js,jsx,mjs,ts,tsx,mts,cts}"],
    rules: {
      // eslint-plugin-react-hooks 7 (via eslint-config-next 16) turns on the
      // React Compiler diagnostics. Payverge does not build with the React
      // Compiler, so keep the pre-upgrade policy (rules-of-hooks +
      // exhaustive-deps) and adopt these in a dedicated pass.
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/refs": "off",
      "react-hooks/preserve-manual-memoization": "off",
      "react-hooks/purity": "off",
      "react-hooks/immutability": "off",
      "react-hooks/static-components": "off",
      "react-hooks/globals": "off",
      "no-console": ["error", { allow: ["warn", "error"] }],
      // Audit guards — enforced post-campaign (T17). The clickable-div / alt-text /
      // anchor a11y issues the lanes targeted are all resolved; `npm run lint` is
      // clean with these at "error", so they stay enforcing to block regressions.
      "jsx-a11y/click-events-have-key-events": "error",
      "jsx-a11y/no-static-element-interactions": "error",
      "jsx-a11y/no-noninteractive-element-interactions": "error",
      "jsx-a11y/alt-text": "error",
      "jsx-a11y/anchor-is-valid": "error",
      "jsx-a11y/label-has-associated-control": ["error", { assert: "either" }],
      "no-restricted-syntax": [
        "error",
        ...paletteRules,
        hexColorRule,
        ...runtimePublicEnvRules,
      ],
    },
  },
  {
    files: ["scripts/**/*.js", "src/i18n/scripts/**/*.js"],
    rules: {
      "no-console": "off",
    },
  },
  {
    // Test fixtures legitimately use literal hex colors (contrast assertions,
    // mock return values for color-returning helpers). Off-palette Tailwind
    // names are still flagged because tests shouldn't reference those at all.
    files: ["**/__tests__/**", "**/*.test.{ts,tsx,js,jsx}"],
    rules: {
      // Tests may set legacy NEXT_PUBLIC_* names to exercise the fallbacks.
      "no-restricted-syntax": ["error", ...paletteRules],
    },
  },
  {
    // The single sanctioned reader of build-time-inlined NEXT_PUBLIC_* values
    // (legacy fallback when runtime env and window.__PAYVERGE_ENV__ are silent).
    files: ["src/config/publicConfig.ts"],
    rules: {
      "no-restricted-syntax": ["error", ...paletteRules, hexColorRule],
    },
  },
];

export default eslintConfig;
