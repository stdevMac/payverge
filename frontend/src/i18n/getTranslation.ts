// OPERATOR-TIER translation lookup — server-safe.
//
// This module deliberately carries NO "use client" directive. The operator
// `getTranslation` helper is a pure function (no React hooks, no browser APIs)
// and must be callable from server code — specifically the marketing pages'
// `generateMetadata()` functions. It previously lived inside the "use client"
// SimpleTranslationProvider; importing it into a server module turned it into a
// client reference, so calling it threw "Attempted to call getTranslation()
// from the server but getTranslation is on the client" and tripped the page
// error boundary. SimpleTranslationProvider now re-exports `getTranslation`
// from here, so all client call sites are unaffected.
//
// The `Locale` type here is `OperatorLocale` = exactly { en, es, es-AR }. The
// broader 21-locale guest set is served by GuestTranslationProvider — do not
// confuse the two.

import type { Locale } from "./localeRegistry";
import { createOperatorLookup } from "./operatorLookup";

// Import translation messages
import enMessages from "./messages/en";
import esMessages from "./messages/es";
import esArOverrides from "./messages/es-ar";
import enCommon from "./locales/en/common.json";
import esCommon from "./locales/es/common.json";
import esArCommon from "./locales/es-ar/common.json";
import { deepMerge } from "./deepMerge";

// es-AR is the neutral `es` base with a thin Rioplatense override layer merged
// on top (DRY: `messages/es-ar/*` and `locales/es-ar/common.json` carry ONLY
// the keys that differ in Argentine Spanish — voseo, local vocab — everything
// else inherits es). This structurally guarantees es-AR can never have a
// "missing key" gap relative to es. It replaces the previous arrangement where
// es-AR reused esMessages wholesale and was therefore byte-identical to es.
const enTree = {
  ...enMessages,
  common: { ...enMessages.common, ...enCommon },
};
const esTree = {
  ...esMessages,
  common: { ...esMessages.common, ...esCommon },
};
const esArTree = deepMerge(deepMerge(esTree, esArOverrides), {
  common: esArCommon,
});

export const messages: Record<Locale, Record<string, any>> = {
  en: enTree,
  es: esTree,
  "es-AR": esArTree,
};

// Simple translation function that doesn't use hooks. Lookup order and
// fallback reporting live in ./operatorLookup (shared with the slim chrome
// catalog in ./operatorChromeCatalog).
export const getTranslation = createOperatorLookup(messages);
