import {
  canonicalToPathSegment,
  getLocaleCodeLabel,
  getLocaleFlag,
  guestMenuTranslationFallbackChain,
  isSupportedLocale,
  normalizeGuestLangParam,
  pathSegmentToCanonical,
  resolveGuestInitialLanguage,
  resolveGuestMenuLanguage,
  nextGuestMenuLanguage,
  resolveLocaleForBrowser
} from '../localeRegistry';

describe('locale registry helpers', () => {
  test('converts canonical regional locales to path segments', () => {
    expect(canonicalToPathSegment('es-AR')).toBe('es-ar');
  });

  test('converts path segments to canonical locales', () => {
    expect(pathSegmentToCanonical('es-ar')).toBe('es-AR');
  });

  test('checks whether a locale is supported', () => {
    expect(isSupportedLocale('en')).toBe(true);
    expect(isSupportedLocale('es-AR')).toBe(true);
    expect(isSupportedLocale('zz')).toBe(false);
  });

  test('prefers exact regional matches from browser languages', () => {
    expect(resolveLocaleForBrowser(['es-AR', 'es', 'en'])).toBe('es-AR');
  });

  test('exposes shared locale flag metadata', () => {
    expect(getLocaleFlag('es-AR')).toBe('🇦🇷');
    expect(getLocaleFlag('fr')).toBe('🇫🇷');
  });

  test('formats compact chrome locale codes (EN / ES / ES-AR)', () => {
    expect(getLocaleCodeLabel('en')).toBe('EN');
    expect(getLocaleCodeLabel('es')).toBe('ES');
    expect(getLocaleCodeLabel('es-AR')).toBe('ES-AR');
    expect(getLocaleCodeLabel(undefined)).toBe('EN');
    expect(getLocaleCodeLabel('')).toBe('EN');
  });

  test('falls back to supported base language for unsupported regional values', () => {
    expect(resolveLocaleForBrowser(['es-MX', 'en'])).toBe('es');
  });

  // Argentine-defaulting fixes (2026-05-29). A later, more-specific exact match
  // must win over an earlier base-language match, and generic Latin-American
  // Spanish (es-419) should land on our LatAm variant (es-AR), not Spain.
  test('a later exact es-AR wins over an earlier base-es match', () => {
    expect(resolveLocaleForBrowser(['es-419', 'es-AR', 'es'])).toBe('es-AR');
  });

  test('maps generic Latin-American Spanish (es-419) to es-AR', () => {
    expect(resolveLocaleForBrowser(['es-419', 'en'])).toBe('es-AR');
  });

  test('Iberian Spanish (es-ES / bare es) stays on neutral es', () => {
    expect(resolveLocaleForBrowser(['es-ES', 'en'])).toBe('es');
    expect(resolveLocaleForBrowser(['es', 'en'])).toBe('es');
  });

  // BCP-47 tags are case-insensitive; browsers occasionally send non-canonical
  // casing. Matching must not depend on the exact "es-AR" casing.
  test('resolves non-canonical casing (es-ar / ES-AR / ES-419 / ES)', () => {
    expect(resolveLocaleForBrowser(['es-ar', 'en'])).toBe('es-AR');
    expect(resolveLocaleForBrowser(['ES-AR'])).toBe('es-AR');
    expect(resolveLocaleForBrowser(['ES-419', 'en'])).toBe('es-AR');
    expect(resolveLocaleForBrowser(['ES', 'en'])).toBe('es');
  });
});

// LOCALE-2: the guest storefront accepts a `?lang=` query param to pick the
// language for the SEO/share snippet (server generateMetadata) and the
// first-paint language. BCP-47 tags are case-insensitive, so a natural
// lowercase `?lang=es-ar` must resolve to the canonical `es-AR` storefront
// locale (matching the case-insensitive route-prefix path), rather than
// silently falling back to the business default.
describe('normalizeGuestLangParam', () => {
  test('lowercase mixed-case storefront code resolves to canonical (es-ar -> es-AR)', () => {
    expect(normalizeGuestLangParam('es-ar')).toBe('es-AR');
  });

  test('uppercase mixed-case storefront code resolves to canonical (ES-AR -> es-AR)', () => {
    expect(normalizeGuestLangParam('ES-AR')).toBe('es-AR');
  });

  test('already-canonical mixed-case input is preserved (es-AR -> es-AR)', () => {
    expect(normalizeGuestLangParam('es-AR')).toBe('es-AR');
  });

  test('plain lowercase single-segment locales resolve to themselves (fr -> fr)', () => {
    expect(normalizeGuestLangParam('fr')).toBe('fr');
    expect(normalizeGuestLangParam('FR')).toBe('fr');
  });

  test('genuinely-unknown values return null', () => {
    expect(normalizeGuestLangParam('bogus')).toBeNull();
    expect(normalizeGuestLangParam('')).toBeNull();
    expect(normalizeGuestLangParam(null)).toBeNull();
    expect(normalizeGuestLangParam(undefined)).toBeNull();
  });
});

describe('resolveGuestInitialLanguage', () => {
  test('a saved per-business preference wins when still enabled', () => {
    expect(
      resolveGuestInitialLanguage({
        saved: 'es-AR',
        browserLanguages: ['en'],
        enabled: ['en', 'es', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('ignores a saved preference the business no longer offers', () => {
    expect(
      resolveGuestInitialLanguage({
        saved: 'fr',
        browserLanguages: ['it'],
        enabled: ['en', 'es'],
        businessDefault: 'es',
      }),
    ).toBe('es');
  });

  test('auto-detects es-AR from an Argentine browser when the business offers it', () => {
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['es-AR', 'es', 'en'],
        enabled: ['en', 'es', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('maps a generic LatAm (es-419) browser to es-AR when offered', () => {
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['es-419', 'en'],
        enabled: ['en', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('keeps Argentine chrome as es-AR when the business only enabled es', () => {
    // Menu rows fall back es-AR → es on the backend. Collapsing the guest
    // locale to generic es (or worse, EN) is what left storefront chrome
    // Spanish while the selector/menu-data stuck on English.
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['es-AR', 'es'],
        enabled: ['en', 'es'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('keeps a saved es-AR preference when only es is enabled (does not leak to EN)', () => {
    expect(
      resolveGuestInitialLanguage({
        saved: 'es-AR',
        browserLanguages: ['en-US', 'en'],
        enabled: ['en', 'es'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('prefers an Argentine-only menu for a Spanish browser', () => {
    // Business only enabled es-AR (no neutral es); a plain "es" browser should
    // still get Spanish, not fall through to English.
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['es', 'en'],
        enabled: ['en', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('matches a base language for non-Spanish browsers', () => {
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['pt-BR', 'en'],
        enabled: ['en', 'pt'],
        businessDefault: 'en',
      }),
    ).toBe('pt');
  });

  test('matches non-canonical browser-tag casing against the enabled set', () => {
    // lowercase "es-ar" must still hit the canonical "es-AR" the business enabled
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['es-ar', 'es'],
        enabled: ['en', 'es', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
    // uppercase generic LatAm
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['ES-419'],
        enabled: ['en', 'es-AR'],
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
  });

  test('falls back to the business default, then English', () => {
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['ja'],
        enabled: ['en', 'es'],
        businessDefault: 'es',
      }),
    ).toBe('es');
    expect(
      resolveGuestInitialLanguage({
        browserLanguages: ['ja'],
        enabled: ['en', 'es'],
        businessDefault: null,
      }),
    ).toBe('en');
  });
});

describe('guestMenuTranslationFallbackChain', () => {
  test('es-AR reuses es menu rows, then stays on the requested code', () => {
    expect(guestMenuTranslationFallbackChain('es-AR')).toEqual(['es-AR', 'es']);
    expect(guestMenuTranslationFallbackChain('es-ar')).toEqual(['es-AR', 'es']);
    expect(guestMenuTranslationFallbackChain('es')).toEqual(['es']);
    expect(guestMenuTranslationFallbackChain('en')).toEqual(['en']);
  });
});

describe('resolveGuestMenuLanguage', () => {
  test('prefers the live guest locale so ?lang=es-AR is not shadowed by a stale EN save', () => {
    expect(
      resolveGuestMenuLanguage({
        saved: 'en',
        currentLanguage: 'es-AR',
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
    expect(
      resolveGuestMenuLanguage({
        saved: 'es-AR',
        currentLanguage: 'es-AR',
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
    expect(
      resolveGuestMenuLanguage({
        saved: 'es-AR',
        currentLanguage: null,
        businessDefault: 'en',
      }),
    ).toBe('es-AR');
    expect(
      resolveGuestMenuLanguage({
        saved: null,
        currentLanguage: '',
        businessDefault: 'en',
      }),
    ).toBe('en');
  });
});

describe('nextGuestMenuLanguage', () => {
  test('reloads the menu when the provider locale changes, including back to en', () => {
    expect(nextGuestMenuLanguage('es-AR', 'en')).toBe('es-AR');
    expect(nextGuestMenuLanguage('en', 'es')).toBe('en');
    expect(nextGuestMenuLanguage('es-AR', 'es-AR')).toBeNull();
    expect(nextGuestMenuLanguage('es-AR', undefined)).toBe('es-AR');
  });
});
