import { getLocaleDirection, type LocaleCode } from "./localeRegistry";

/**
 * The <html lang> the operator tier (SimpleTranslationProvider) wants. Kept
 * outside React so GuestTranslationProvider can hand the document back to it
 * on unmount without depending on the operator context.
 */
let operatorHtmlLang: LocaleCode = "en";

export function setOperatorHtmlLang(locale: LocaleCode): void {
  operatorHtmlLang = locale;
}

/** Restore <html lang>/<dir> to the operator tier's locale. */
export function restoreOperatorHtmlAttributes(): void {
  if (typeof document === "undefined") return;
  document.documentElement.lang = operatorHtmlLang;
  document.documentElement.dir = getLocaleDirection(operatorHtmlLang);
}
