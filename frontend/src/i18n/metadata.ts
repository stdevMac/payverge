import {
  canonicalToPathSegment,
  defaultLocale,
  type Locale
} from './localeRegistry';

export function publicPathForLocale(locale: Locale, path: string): string {
  if (locale === defaultLocale) {
    return path;
  }

  return `/${canonicalToPathSegment(locale)}${path === '/' ? '' : path}`;
}
