import { axiosInstance } from './tools/instance';

// Types for currency and language management
export interface SupportedCurrency {
  id: number;
  code: string;
  name: string;
  symbol: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface SupportedLanguage {
  id: number;
  code: string;
  name: string;
  native_name: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface BusinessLanguage {
  id: number;
  business_id: number;
  language_code: string;
  is_default: boolean;
  display_order: number;
  created_at: string;
  updated_at: string;
}

// API Response types
export interface CurrenciesResponse {
  currencies: SupportedCurrency[];
}

export interface LanguagesResponse {
  languages: SupportedLanguage[];
}

export interface BusinessLanguagesResponse {
  languages: BusinessLanguage[];
}

export interface ConversionResponse {
  original_amount: number;
  from_currency: string;
  converted_amount: number;
  to_currency: string;
}

// Request types
export interface UpdateBusinessLanguagesRequest {
  language_codes: string[];
  default_code: string;
}

// Public API functions (no authentication required)
export const getSupportedCurrencies = async (): Promise<SupportedCurrency[]> => {
  // Static platform reference data (the supported-currency master list). Safe
  // to cache — it does not change within a session. Opt in explicitly since
  // GETs are uncached by default (C1). Exchange rates are NOT cached here.
  const response = await axiosInstance.get<CurrenciesResponse>('/currencies', {
    _useCache: true,
  });
  return response.data.currencies;
};

export const getSupportedLanguages = async (): Promise<SupportedLanguage[]> => {
  // Static platform reference data (the supported-language master list).
  // Safe to cache; opt in explicitly (GETs are uncached by default — C1).
  const response = await axiosInstance.get<LanguagesResponse>('/languages', {
    _useCache: true,
  });
  return response.data.languages;
};

export const convertAmount = async (
  amount: number,
  fromCurrency: string,
  toCurrency: string
): Promise<ConversionResponse> => {
  // First get the exchange rate
  const rateResponse = await axiosInstance.get('/exchange-rate', {
    params: { from: fromCurrency, to: toCurrency }
  });
  
  // Calculate the converted amount (cents precision — FIND-043 residual).
  const rate = rateResponse.data.rate;
  const convertedAmount = Math.round(amount * rate * 100) / 100;

  return {
    original_amount: amount,
    from_currency: fromCurrency,
    to_currency: toCurrency,
    converted_amount: convertedAmount
  };
};

// Protected API functions (require authentication)
export const getBusinessLanguages = async (businessId: number): Promise<BusinessLanguage[]> => {
  const response = await axiosInstance.get<BusinessLanguagesResponse>(`/inside/businesses/${businessId}/languages`);
  return response.data.languages;
};

export const updateBusinessLanguages = async (
  businessId: number,
  request: UpdateBusinessLanguagesRequest
): Promise<void> => {
  await axiosInstance.put(`/inside/businesses/${businessId}/languages`, request);
};

// Public guest menu translations — does not require authentication.
// Resolves the business from a public table code so diners don't hit a 401.
export const getGuestMenuTranslations = async (
  tableCode: string,
  languageCode: string
): Promise<{
  business_id: number;
  language_code: string;
  translations: {
    [entityType: string]: {
      [entityId: string]: {
        [fieldName: string]: string;
      };
    };
  };
}> => {
  const response = await axiosInstance.get(
    `/guest/table/${encodeURIComponent(tableCode)}/menu-translations`,
    { params: { language_code: languageCode } },
  );
  return response.data;
};

// Batch translation interfaces
export interface TranslateMenuRequest {
  language_codes: string[];
}

export interface TranslateMenuResponse {
  job_id: string;
  status: string;
  message: string;
}

export interface TranslationStatus {
  job_id: string;
  status: string;
  progress: number;
  total: number;
  message: string;
  created_at: string;
}

// Batch translation functions
export const translateEntireMenu = async (
  businessId: number,
  languageCodes: string[]
): Promise<TranslateMenuResponse> => {
  const response = await axiosInstance.post(`/inside/businesses/${businessId}/translate`, {
    language_codes: languageCodes
  });
  return response.data;
};

export const getTranslationStatus = async (jobId: string): Promise<TranslationStatus> => {
  const response = await axiosInstance.get(`/inside/translation-jobs/${jobId}/status`);
  return response.data;
};

// Utility functions
//
// `locale` lets guest-facing call sites pass the diner's active language so
// grouping/decimal separators follow their locale (e.g. de-DE -> "1.234,56 €")
// while the currency code stays correct. Defaults to "en-US" so existing
// operator/admin callers keep their current formatting.
export type FormatCurrencyOptions = {
  maximumFractionDigits?: number;
  minimumFractionDigits?: number;
  /** Force grouping for 4-digit amounts (needed for some es-ES currency runs). */
  useGrouping?: boolean | "always" | "auto" | "min2";
};

/**
 * Locale-aware decimal amount (no ISO currency style). Used for crypto/non-ISO
 * codes and the empty-code fallback so guest surfaces never mix toFixed
 * ("USDC 34.99") with Intl totals ("34,99 $").
 */
const formatLocaleDecimalAmount = (
  amount: number,
  locale: string,
  digits: number,
  useGrouping?: FormatCurrencyOptions["useGrouping"],
): string => {
  const intlOptions: Intl.NumberFormatOptions = {
    style: "decimal",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  };
  if (useGrouping !== undefined) {
    intlOptions.useGrouping =
      useGrouping as Intl.NumberFormatOptions["useGrouping"];
  }
  try {
    return new Intl.NumberFormat(locale, intlOptions).format(amount);
  } catch {
    return new Intl.NumberFormat("en-US", intlOptions).format(amount);
  }
};

export const formatCurrency = (
  amount: number,
  currencyCode: string,
  symbol?: string,
  locale: string = 'en-US',
  options?: FormatCurrencyOptions,
): string => {
  const digits = options?.maximumFractionDigits ?? 2;

  // Handle undefined or null currency codes
  if (!currencyCode) {
    return formatLocaleDecimalAmount(
      amount,
      locale,
      digits,
      options?.useGrouping,
    );
  }

  // List of known cryptocurrency and non-standard currency codes that Intl.NumberFormat doesn't support
  const nonStandardCurrencies = ['USDC', 'USDT', 'BTC', 'ETH', 'MATIC', 'BNB'];

  // Check if it's a non-standard currency code
  if (nonStandardCurrencies.includes(currencyCode.toUpperCase())) {
    return `${symbol || currencyCode} ${formatLocaleDecimalAmount(
      amount,
      locale,
      digits,
      options?.useGrouping,
    )}`;
  }

  try {
    const formatter = new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: currencyCode,
      ...(options?.maximumFractionDigits !== undefined
        ? { maximumFractionDigits: options.maximumFractionDigits }
        : {}),
      ...(options?.minimumFractionDigits !== undefined
        ? { minimumFractionDigits: options.minimumFractionDigits }
        : options?.maximumFractionDigits !== undefined
          ? { minimumFractionDigits: options.maximumFractionDigits }
          : {}),
      ...(options?.useGrouping !== undefined
        ? { useGrouping: options.useGrouping as Intl.NumberFormatOptions["useGrouping"] }
        : {}),
    });
    return formatter.format(amount);
  } catch (error) {
    // Fallback for any other unsupported currency codes — still locale-aware.
    return `${symbol || currencyCode} ${formatLocaleDecimalAmount(
      amount,
      locale,
      digits,
      options?.useGrouping,
    )}`;
  }
};
