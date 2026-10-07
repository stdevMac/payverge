// Barrel for the operator translation tier. Deliberately NOT "use client":
// it only re-exports. The provider, context and locale helpers live in
// ./OperatorLocaleProvider (a client module); the full operator catalog lookup
// lives in the server-safe ./getTranslation. Operator pages and their tests
// keep importing (and mocking) this path. Root-layout chrome must import
// ./OperatorLocaleProvider and ./operatorChromeCatalog directly instead, or the
// ~1.9 MB operator catalog lands on every diner route (see
// src/__tests__/guest-route-import-graph.test.ts).
//
// Named re-exports only: Next.js rejects `export *` across a client boundary.

export {
  getPublicLocaleSwitchHref,
  OperatorTranslationProvider,
  persistOperatorLocaleChoice,
  resolveRuntimeLocale,
  SimpleTranslationProvider,
  useSimpleLocale,
} from "./OperatorLocaleProvider";
export { getTranslation } from "./getTranslation";
