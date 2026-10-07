import { getLocaleFlag, getLocaleDisplayName, locales, type Locale } from './localeRegistry';

export { locales };
export type { Locale };

// Language names for display
export const languageNames = Object.fromEntries(
  locales.map((locale) => [locale, getLocaleDisplayName(locale)])
) as Record<Locale, string>;

export const languageFlags = Object.fromEntries(
  locales.map((locale) => [locale, getLocaleFlag(locale)])
) as Record<Locale, string>;
