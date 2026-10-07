import { publicPathForLocale } from "@/i18n/metadata";
import type { Locale } from "@/i18n/localeRegistry";

/** Locale-prefixed staff home so ES/AR login does not 404 after redirect. */
export function staffHomeHref(locale: Locale): string {
  return publicPathForLocale(locale, "/staff/home");
}
