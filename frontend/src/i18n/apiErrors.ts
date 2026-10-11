import enErrors from './locales/en/apiErrors.json';
import esErrors from './locales/es/apiErrors.json';
import esArErrors from './locales/es-ar/apiErrors.json';
import {
  defaultLocale,
  guestLocales,
  isGuestLocale,
  type GuestLocale,
} from './localeRegistry';

export interface ApiErrorPayload {
  code?: string;
  error?: string;
  params?: Record<string, string | number>;
}

type ApiErrorCatalog = Record<string, string>;
type ApiErrorCatalogModule = { default: ApiErrorCatalog };

// Operator locales stay in the main bundle. The other 18 guest catalogs load
// on demand so dashboard routes do not download them.
const staticCatalogs: Record<string, ApiErrorCatalog> = {
  en: enErrors,
  es: esErrors,
  'es-AR': esArErrors,
};

const loadedCatalogs = new Map<GuestLocale, ApiErrorCatalog>(
  Object.entries(staticCatalogs) as Array<[GuestLocale, ApiErrorCatalog]>,
);

const guestCatalogLoaders: Record<string, () => Promise<ApiErrorCatalogModule>> = {
  ar: () => import('./locales/ar/apiErrors.json'),
  da: () => import('./locales/da/apiErrors.json'),
  de: () => import('./locales/de/apiErrors.json'),
  fr: () => import('./locales/fr/apiErrors.json'),
  hi: () => import('./locales/hi/apiErrors.json'),
  it: () => import('./locales/it/apiErrors.json'),
  ja: () => import('./locales/ja/apiErrors.json'),
  ko: () => import('./locales/ko/apiErrors.json'),
  nl: () => import('./locales/nl/apiErrors.json'),
  no: () => import('./locales/no/apiErrors.json'),
  pl: () => import('./locales/pl/apiErrors.json'),
  pt: () => import('./locales/pt/apiErrors.json'),
  ru: () => import('./locales/ru/apiErrors.json'),
  sv: () => import('./locales/sv/apiErrors.json'),
  th: () => import('./locales/th/apiErrors.json'),
  tr: () => import('./locales/tr/apiErrors.json'),
  vi: () => import('./locales/vi/apiErrors.json'),
  zh: () => import('./locales/zh/apiErrors.json'),
};

const escapeRegExp = (value: string): string =>
  value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

function interpolate(
  template: string,
  params?: Record<string, string | number>,
): string {
  if (!params) {
    return template;
  }

  return Object.entries(params).reduce(
    (message, [key, value]) =>
      message.replace(
        new RegExp(`\\{${escapeRegExp(key)}\\}`, 'g'),
        () => String(value),
      ),
    template,
  );
}

// Widened from the operator tier (isSupportedLocale) to the full guest tier:
// guests toast these on loyalty redeem/undo in their storefront language.
// Operator locales (en/es/es-AR) resolve exactly as before.
function resolveApiErrorLocale(locale: string): GuestLocale {
  if (isGuestLocale(locale)) return locale;
  const base = (locale || '').split(/[-_]/)[0].toLowerCase();
  if (isGuestLocale(base)) return base;
  return defaultLocale;
}

function catalogFromModule(mod: ApiErrorCatalogModule): ApiErrorCatalog {
  if (mod && typeof mod.default === 'object' && mod.default) {
    return mod.default;
  }
  return mod as unknown as ApiErrorCatalog;
}

/**
 * Load a guest apiErrors catalog. No-op for en/es/es-AR and for a catalog
 * that is already in memory. A failed load is swallowed; translateApiError
 * still falls back to English.
 */
export async function loadApiErrorCatalog(locale: string): Promise<void> {
  const resolved = resolveApiErrorLocale(locale);
  if (loadedCatalogs.has(resolved)) return;
  const loader = guestCatalogLoaders[resolved];
  if (!loader) return;
  try {
    const mod = await loader();
    loadedCatalogs.set(resolved, catalogFromModule(mod));
  } catch {
    // English fallback still works.
  }
}

export function translateApiError(
  payload: ApiErrorPayload,
  locale: string = defaultLocale,
): string {
  const resolvedLocale = resolveApiErrorLocale(locale);
  const code = payload.code;
  const active = loadedCatalogs.get(resolvedLocale);
  const english = loadedCatalogs.get(defaultLocale) ?? staticCatalogs.en;
  const template =
    (code ? active?.[code] : undefined) ??
    (code ? english[code] : undefined) ??
    payload.error ??
    code ??
    '';

  return interpolate(template, payload.params);
}

/** Test-only: load every guest catalog and return the full 21-locale map. */
export async function __loadAllApiErrorCatalogsForTests(): Promise<
  Record<GuestLocale, ApiErrorCatalog>
> {
  await Promise.all(guestLocales.map((locale) => loadApiErrorCatalog(locale)));
  const out = {} as Record<GuestLocale, ApiErrorCatalog>;
  for (const locale of guestLocales) {
    const catalog = loadedCatalogs.get(locale);
    if (!catalog) {
      throw new Error(`api error catalog failed to load: ${locale}`);
    }
    out[locale] = catalog;
  }
  return out;
}
