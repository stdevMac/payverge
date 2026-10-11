"use client";

import { useCallback, useEffect } from "react";
import { useSimpleLocale } from "./OperatorLocaleProvider";
import { useGuestTranslation } from "./GuestTranslationProvider";
import { loadApiErrorCatalog } from "./apiErrors";
import { getLocalizedApiError } from "@/utils/apiError";

/**
 * H1 (frontend): component-facing translators for caught API errors.
 *
 * `getLocalizedApiError(error, locale)` (utils/apiError) is the pure, locale-aware
 * entry point, but most catch-blocks don't have the active locale handy. These
 * hooks read it from the appropriate tier's provider and hand back a stable
 * `(error) => string` so a catch-block can do:
 *
 *   const localizeError = useApiErrorMessage();
 *   ...
 *   catch (err) { toast.error(localizeError(err)); }
 *
 * Coded backend errors localize via apiErrors.json; uncoded errors fall back to
 * the raw backend string; a code-less, string-less throw degrades to a generic
 * localized message (never an empty toast).
 */

/** Operator-dashboard tier: reads the locale from SimpleTranslationProvider. */
export function useApiErrorMessage(): (error: unknown) => string {
  const { locale } = useSimpleLocale();
  useEffect(() => {
    void loadApiErrorCatalog(locale);
  }, [locale]);
  return useCallback(
    (error: unknown) => getLocalizedApiError(error, locale),
    [locale],
  );
}

/** Guest-storefront tier: reads the language from GuestTranslationProvider. */
export function useGuestApiErrorMessage(): (error: unknown) => string {
  const { currentLanguage } = useGuestTranslation();
  useEffect(() => {
    void loadApiErrorCatalog(currentLanguage);
  }, [currentLanguage]);
  return useCallback(
    (error: unknown) => getLocalizedApiError(error, currentLanguage),
    [currentLanguage],
  );
}
