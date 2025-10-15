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

export interface BusinessCurrency {
  id: number;
  business_id: number;
  currency_code: string;
  is_preferred: boolean;
  display_order: number;
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

export interface ExchangeRate {
  id: number;
  from_currency: string;
  to_currency: string;
  rate: number;
  source: string;
  fetched_at: string;
  created_at: string;
  updated_at: string;
}

export interface Translation {
  id: number;
  entity_type: string;
  entity_id: number;
  field_name: string;
  language_code: string;
  original_text: string;
  translated_text: string;
  is_auto_translated: boolean;
  translation_source: string;
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

export interface BusinessCurrenciesResponse {
  currencies: BusinessCurrency[];
}

export interface BusinessLanguagesResponse {
  languages: BusinessLanguage[];
}

export interface ExchangeRateResponse {
  from_currency: string;
  to_currency: string;
  rate: number;
}

export interface ConversionResponse {
  original_amount: number;
  from_currency: string;
  converted_amount: number;
  to_currency: string;
}

export interface TranslationResponse {
  entity_type: string;
  entity_id: number;
  field_name: string;
  language_code: string;
  translated_text: string;
}

// Request types
export interface UpdateBusinessCurrenciesRequest {
  currency_codes: string[];
  preferred_code: string;
}

export interface UpdateBusinessLanguagesRequest {
  language_codes: string[];
  default_code: string;
}

export interface UpdateTranslationRequest {
  entity_type: string;
  entity_id: number;
  field_name: string;
  language_code: string;
  translated_text: string;
  original_text?: string;
}

// Public API functions (no authentication required)
export const getSupportedCurrencies = async (): Promise<SupportedCurrency[]> => {
  const response = await axiosInstance.get<CurrenciesResponse>('/currencies');
  return response.data.currencies;
};

export const getSupportedLanguages = async (): Promise<SupportedLanguage[]> => {
  const response = await axiosInstance.get<LanguagesResponse>('/languages');
  return response.data.languages;
};

export const getExchangeRate = async (fromCurrency: string, toCurrency: string): Promise<ExchangeRateResponse> => {
  const response = await axiosInstance.get<ExchangeRateResponse>('/exchange-rate', {
    params: { from: fromCurrency, to: toCurrency }
  });
  return response.data;
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
  
  // Calculate the converted amount
  const rate = rateResponse.data.rate;
  const convertedAmount = amount * rate;
  
  return {
    original_amount: amount,
    from_currency: fromCurrency,
    to_currency: toCurrency,
    converted_amount: convertedAmount
  };
};

// Protected API functions (require authentication)
export const getBusinessCurrencies = async (businessId: number): Promise<BusinessCurrency[]> => {
  const response = await axiosInstance.get<BusinessCurrenciesResponse>(`/inside/businesses/${businessId}/currencies`);
  return response.data.currencies;
};

export const updateBusinessCurrencies = async (
  businessId: number,
  request: UpdateBusinessCurrenciesRequest
): Promise<void> => {
  await axiosInstance.put(`/inside/businesses/${businessId}/currencies`, request);
};

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

export const getTranslatedContent = async (
  entityType: string,
  entityId: number,
  fieldName: string,
  languageCode: string
): Promise<string> => {
  const response = await axiosInstance.get<TranslationResponse>('/inside/translations', {
    params: { entity_type: entityType, entity_id: entityId, field_name: fieldName, language_code: languageCode }
  });
  return response.data.translated_text;
};

export const updateTranslation = async (request: UpdateTranslationRequest): Promise<void> => {
  await axiosInstance.put('/inside/translations', request);
};

// Get all translations for a business menu in a specific language
export const getMenuTranslations = async (
  businessId: number,
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
  const response = await axiosInstance.get('/inside/menu-translations', {
    params: { business_id: businessId, language_code: languageCode }
  });
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

// Enhanced translation functions for menu items
export const getMenuItemTranslations = async (
  businessId: number,
  itemId: number,
  languageCodes: string[]
): Promise<Record<string, { name: string; description: string }>> => {
  const translations: Record<string, { name: string; description: string }> = {};
  
  for (const langCode of languageCodes) {
    try {
      const [nameTranslation, descTranslation] = await Promise.all([
        getTranslatedContent('menu_item', itemId, 'name', langCode).catch(() => ''),
        getTranslatedContent('menu_item', itemId, 'description', langCode).catch(() => '')
      ]);
      
      translations[langCode] = {
        name: nameTranslation,
        description: descTranslation
      };
    } catch (error) {
      translations[langCode] = { name: '', description: '' };
    }
  }
  
  return translations;
};

export const getCategoryTranslations = async (
  businessId: number,
  categoryId: number,
  languageCodes: string[]
): Promise<Record<string, { name: string; description: string }>> => {
  const translations: Record<string, { name: string; description: string }> = {};
  
  for (const langCode of languageCodes) {
    try {
      const [nameTranslation, descTranslation] = await Promise.all([
        getTranslatedContent('category', categoryId, 'name', langCode).catch(() => ''),
        getTranslatedContent('category', categoryId, 'description', langCode).catch(() => '')
      ]);
      
      translations[langCode] = {
        name: nameTranslation,
        description: descTranslation
      };
    } catch (error) {
      translations[langCode] = { name: '', description: '' };
    }
  }
  
  return translations;
};

export const saveMenuItemTranslation = async (
  itemId: number,
  fieldName: 'name' | 'description',
  languageCode: string,
  translatedText: string,
  originalText: string
): Promise<void> => {
  await updateTranslation({
    entity_type: 'menu_item',
    entity_id: itemId,
    field_name: fieldName,
    language_code: languageCode,
    translated_text: translatedText,
    original_text: originalText
  });
};

export const saveCategoryTranslation = async (
  categoryId: number,
  fieldName: 'name' | 'description',
  languageCode: string,
  translatedText: string,
  originalText: string
): Promise<void> => {
  await updateTranslation({
    entity_type: 'category',
    entity_id: categoryId,
    field_name: fieldName,
    language_code: languageCode,
    translated_text: translatedText,
    original_text: originalText
  });
};

// Utility functions
export const formatCurrency = (amount: number, currencyCode: string, symbol?: string): string => {
  // Handle undefined or null currency codes
  if (!currencyCode) {
    return `${amount.toFixed(2)}`;
  }
  
  // List of known cryptocurrency and non-standard currency codes that Intl.NumberFormat doesn't support
  const nonStandardCurrencies = ['USDC', 'USDT', 'BTC', 'ETH', 'MATIC', 'BNB'];
  
  // Check if it's a non-standard currency code
  if (nonStandardCurrencies.includes(currencyCode.toUpperCase())) {
    return `${symbol || currencyCode} ${amount.toFixed(2)}`;
  }

  try {
    const formatter = new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: currencyCode,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    });
    return formatter.format(amount);
  } catch (error) {
    // Fallback for any other unsupported currency codes
    return `${symbol || currencyCode} ${amount.toFixed(2)}`;
  }
};

export const getCurrencySymbol = (currencies: SupportedCurrency[], code: string): string => {
  const currency = currencies.find(c => c.code === code);
  return currency?.symbol || code;
};

export const getLanguageName = (languages: SupportedLanguage[], code: string): string => {
  const language = languages.find(l => l.code === code);
  return language?.name || code;
};

export const getLanguageNativeName = (languages: SupportedLanguage[], code: string): string => {
  const language = languages.find(l => l.code === code);
  return language?.native_name || code;
};

// Currency conversion hook-like function
export const createCurrencyConverter = (exchangeRates: ExchangeRate[]) => {
  return (amount: number, fromCurrency: string, toCurrency: string): number => {
    if (fromCurrency === toCurrency) {
      return amount;
    }

    const rate = exchangeRates.find(
      r => r.from_currency === fromCurrency && r.to_currency === toCurrency
    );

    if (!rate) {
      throw new Error(`Exchange rate not found for ${fromCurrency} to ${toCurrency}`);
    }

    return amount * rate.rate;
  };
};

// Language detection utility
export const detectBrowserLanguage = (): string => {
  if (typeof window === 'undefined') {
    return 'en'; // Default for SSR
  }

  const browserLang = navigator.language || navigator.languages?.[0] || 'en';
  return browserLang.split('-')[0]; // Get language code without region
};

// Common currency codes for quick access
export const COMMON_CURRENCIES = [
  'USD', 'EUR', 'GBP', 'JPY', 'AUD', 'CAD', 'CHF', 'CNY',
  'ARS', 'AED', 'BRL', 'MXN', 'INR', 'KRW', 'SGD', 'HKD'
] as const;

// Common language codes for quick access
export const COMMON_LANGUAGES = [
  'en', 'es', 'fr', 'de', 'it', 'pt', 'ru', 'ja',
  'ko', 'zh', 'ar', 'hi', 'th', 'vi', 'tr', 'pl'
] as const;
