// Slim operator catalog for root-layout chrome — server-safe (no "use client").
//
// The full operator catalog (./getTranslation, ~1.9 MB of en/es/es-AR source)
// used to reach every route through the root layout, diner routes included.
// Root chrome only needs the handful of subtrees in OPERATOR_CHROME_PATHS, so
// it reads these generated copies instead. Operator pages keep importing the
// full ./getTranslation. Regenerate after editing the picked keys with:
//   UPDATE_OPERATOR_CHROME=1 npx jest operator-chrome-catalog-parity
// Must never import ./getTranslation, ./messages/* or ./SimpleTranslationProvider.

import { createOperatorLookup } from "./operatorLookup";
import en from "./operator-chrome/en.json";
import es from "./operator-chrome/es.json";
import esAr from "./operator-chrome/es-AR.json";

export const getChromeTranslation = createOperatorLookup({
  en,
  es,
  "es-AR": esAr,
});
