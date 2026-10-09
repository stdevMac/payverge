// Mirror of backend internal/geodefaults. The backend remains the source of
// truth on creation (defense in depth); this powers the inline signup preview
// and the values sent in registration_data.

export interface CountryDefaults {
  currency: string;
  timezone: string;
}

export interface CountryOption {
  code: string; // ISO-3166 alpha-2
  name: string;
  currency: string;
  timezone: string;
}

export const COUNTRY_OPTIONS: CountryOption[] = [
  { code: "US", name: "United States", currency: "USD", timezone: "America/New_York" },
  { code: "CA", name: "Canada", currency: "CAD", timezone: "America/Toronto" },
  { code: "GB", name: "United Kingdom", currency: "GBP", timezone: "Europe/London" },
  { code: "IE", name: "Ireland", currency: "EUR", timezone: "Europe/Dublin" },
  { code: "AR", name: "Argentina", currency: "ARS", timezone: "America/Argentina/Buenos_Aires" },
  { code: "BR", name: "Brazil", currency: "BRL", timezone: "America/Sao_Paulo" },
  { code: "MX", name: "Mexico", currency: "MXN", timezone: "America/Mexico_City" },
  { code: "ES", name: "Spain", currency: "EUR", timezone: "Europe/Madrid" },
  { code: "FR", name: "France", currency: "EUR", timezone: "Europe/Paris" },
  { code: "DE", name: "Germany", currency: "EUR", timezone: "Europe/Berlin" },
  { code: "IT", name: "Italy", currency: "EUR", timezone: "Europe/Rome" },
  { code: "PT", name: "Portugal", currency: "EUR", timezone: "Europe/Lisbon" },
  { code: "NL", name: "Netherlands", currency: "EUR", timezone: "Europe/Amsterdam" },
  { code: "AT", name: "Austria", currency: "EUR", timezone: "Europe/Vienna" },
  { code: "BE", name: "Belgium", currency: "EUR", timezone: "Europe/Brussels" },
  { code: "GR", name: "Greece", currency: "EUR", timezone: "Europe/Athens" },
  { code: "AU", name: "Australia", currency: "AUD", timezone: "Australia/Sydney" },
  { code: "CH", name: "Switzerland", currency: "CHF", timezone: "Europe/Zurich" },
  { code: "CN", name: "China", currency: "CNY", timezone: "Asia/Shanghai" },
  { code: "AE", name: "United Arab Emirates", currency: "AED", timezone: "Asia/Dubai" },
  { code: "IN", name: "India", currency: "INR", timezone: "Asia/Kolkata" },
  { code: "JP", name: "Japan", currency: "JPY", timezone: "Asia/Tokyo" },
  { code: "KR", name: "South Korea", currency: "KRW", timezone: "Asia/Seoul" },
  { code: "SG", name: "Singapore", currency: "SGD", timezone: "Asia/Singapore" },
  { code: "HK", name: "Hong Kong", currency: "HKD", timezone: "Asia/Hong_Kong" },
  { code: "NO", name: "Norway", currency: "NOK", timezone: "Europe/Oslo" },
  { code: "SE", name: "Sweden", currency: "SEK", timezone: "Europe/Stockholm" },
  { code: "DK", name: "Denmark", currency: "DKK", timezone: "Europe/Copenhagen" },
  { code: "PL", name: "Poland", currency: "PLN", timezone: "Europe/Warsaw" },
  { code: "CL", name: "Chile", currency: "CLP", timezone: "America/Santiago" },
  { code: "CO", name: "Colombia", currency: "COP", timezone: "America/Bogota" },
  { code: "PE", name: "Peru", currency: "PEN", timezone: "America/Lima" },
  { code: "UY", name: "Uruguay", currency: "UYU", timezone: "America/Montevideo" },
  { code: "PY", name: "Paraguay", currency: "PYG", timezone: "America/Asuncion" },
  { code: "BO", name: "Bolivia", currency: "BOB", timezone: "America/La_Paz" },
  { code: "EC", name: "Ecuador", currency: "USD", timezone: "America/Guayaquil" },
  { code: "CR", name: "Costa Rica", currency: "CRC", timezone: "America/Costa_Rica" },
  { code: "PA", name: "Panama", currency: "USD", timezone: "America/Panama" },
  { code: "DO", name: "Dominican Republic", currency: "DOP", timezone: "America/Santo_Domingo" },
];

const FALLBACK: CountryDefaults = { currency: "USD", timezone: "UTC" };

const BY_CODE: Record<string, CountryDefaults> = COUNTRY_OPTIONS.reduce(
  (acc, c) => {
    acc[c.code] = { currency: c.currency, timezone: c.timezone };
    return acc;
  },
  {} as Record<string, CountryDefaults>,
);

export function getDefaultsForCountry(code: string): CountryDefaults {
  const key = (code || "").trim().toUpperCase();
  return BY_CODE[key] ?? FALLBACK;
}

/** Pre-select Argentina on the es-AR register wizard so currency/TZ are ARS/ART. */
export function defaultRegisterCountryForLocale(locale: string): string {
  const normalized = locale.replace("_", "-").toLowerCase();
  return normalized === "es-ar" ? "AR" : "";
}

// Business types. `labelKey` resolves through businessRegister.* i18n.
export const BUSINESS_TYPE_OPTIONS: { value: string; labelKey: string }[] = [
  { value: "restaurant", labelKey: "businessInfo.businessTypes.restaurant" },
  { value: "cafe", labelKey: "businessInfo.businessTypes.cafe" },
  { value: "bar", labelKey: "businessInfo.businessTypes.bar" },
  { value: "quick_service", labelKey: "businessInfo.businessTypes.quick_service" },
  { value: "food_truck", labelKey: "businessInfo.businessTypes.food_truck" },
  { value: "bakery", labelKey: "businessInfo.businessTypes.bakery" },
  { value: "fine_dining", labelKey: "businessInfo.businessTypes.fine_dining" },
  { value: "other", labelKey: "businessInfo.businessTypes.other" },
];
