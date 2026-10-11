/**
 * Storefront prose can leak a leftover source-language bucket (Arabic on
 * ?lang=en / historically ?lang=zh) when the business default language does
 * not match the stored description script.
 */

const ARABIC_SCRIPT = /[\u0600-\u06FF\u0750-\u077F\u08A0-\u08FF\uFB50-\uFDFF\uFE70-\uFEFF]/;

function isArabicLocale(locale: string): boolean {
  const lower = locale.trim().toLowerCase();
  return lower === "ar" || lower.startsWith("ar-");
}

export function storefrontProseMatchesLocale(
  text: string,
  locale: string,
): boolean {
  if (!text.trim()) return true;
  if (isArabicLocale(locale)) return true;
  return !ARABIC_SCRIPT.test(text);
}

export function sanitizeStorefrontProse(
  text: string | undefined | null,
  locale: string,
  fallback: string,
): string {
  if (!text || !text.trim()) return fallback;
  return storefrontProseMatchesLocale(text, locale) ? text : fallback;
}
