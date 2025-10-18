import { locales, type Locale } from './config';

/**
 * Validates if a locale is supported
 */
export function isValidLocale(locale: string): locale is Locale {
  return locales.includes(locale as Locale);
}

/**
 * Gets the default locale
 */
export function getDefaultLocale(): Locale {
  return 'en';
}

/**
 * Gets locale from pathname
 */
export function getLocaleFromPathname(pathname: string): Locale | null {
  const segments = pathname.split('/');
  const potentialLocale = segments[1];
  
  if (potentialLocale && isValidLocale(potentialLocale)) {
    return potentialLocale;
  }
  
  return null;
}

/**
 * Removes locale from pathname
 */
export function removeLocaleFromPathname(pathname: string): string {
  const locale = getLocaleFromPathname(pathname);
  if (locale) {
    return pathname.replace(`/${locale}`, '') || '/';
  }
  return pathname;
}

/**
 * Adds locale to pathname
 */
export function addLocaleToPathname(pathname: string, locale: Locale): string {
  const cleanPath = removeLocaleFromPathname(pathname);
  if (locale === 'en') {
    // Don't add locale prefix for default locale
    return cleanPath;
  }
  return `/${locale}${cleanPath}`;
}
