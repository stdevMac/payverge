// Can be imported from a shared config
export const locales = ['en', 'es'] as const;
export type Locale = (typeof locales)[number];

export const defaultLocale: Locale = 'en';

// Language names for display
export const languageNames = {
  en: 'English',
  es: 'Español'
} as const;

// Language flags for display
export const languageFlags = {
  en: '🇺🇸',
  es: '🇪🇸'
} as const;
