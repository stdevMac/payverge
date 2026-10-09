import React from "react";
import { Spinner } from "@nextui-org/react";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";

/**
 * Full-bleed loading state for a dashboard tab.
 *
 * The previous implementation painted a `bg-ink-200` rectangle, which
 * flashed nearly-charcoal against the cream page background every time
 * a tab swapped. That single-frame jolt was the loudest "this app is
 * working" tell in the product. The fix is to match the warm canvas the
 * dashboard already uses so the spinner reads as "the page is settling,"
 * not "the page got replaced."
 */
export const PrimarySpinner = () => {
  const { locale } = useSimpleLocale();
  const loadingLabel =
    (getTranslation("common.loading", locale) as string) || "Loading…";
  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed inset-0 flex items-center justify-center bg-warm-50/95 backdrop-blur-sm transition-colors duration-200"
    >
      <Spinner size="lg" color="primary" />
      <span className="sr-only">{loadingLabel}</span>
    </div>
  );
};
