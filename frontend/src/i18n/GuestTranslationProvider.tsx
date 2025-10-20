'use client';

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';

// Extended language support for guest interface
export const GUEST_SUPPORTED_LANGUAGES = {
  en: { name: 'English', flag: '🇺🇸' },
  es: { name: 'Español', flag: '🇪🇸' },
  fr: { name: 'Français', flag: '🇫🇷' },
  de: { name: 'Deutsch', flag: '🇩🇪' },
  it: { name: 'Italiano', flag: '🇮🇹' },
  pt: { name: 'Português', flag: '🇵🇹' },
  zh: { name: '中文', flag: '🇨🇳' },
  ja: { name: '日本語', flag: '🇯🇵' },
  ko: { name: '한국어', flag: '🇰🇷' },
  ar: { name: 'العربية', flag: '🇸🇦' },
  ru: { name: 'Русский', flag: '🇷🇺' },
  hi: { name: 'हिन्दी', flag: '🇮🇳' }
} as const;

export type GuestLanguageCode = keyof typeof GUEST_SUPPORTED_LANGUAGES;

interface GuestTranslationContextType {
  currentLanguage: GuestLanguageCode;
  setLanguage: (language: GuestLanguageCode) => void;
  t: (key: string, params?: Record<string, any>) => string;
  availableLanguages: typeof GUEST_SUPPORTED_LANGUAGES;
  businessId?: number;
  setBusinessId: (id: number) => void;
}

const GuestTranslationContext = createContext<GuestTranslationContextType | undefined>(undefined);

interface GuestTranslationProviderProps {
  children: ReactNode;
  businessId?: number;
  initialLanguage?: GuestLanguageCode;
}

// Translation storage - will be loaded dynamically
const translations: Record<GuestLanguageCode, Record<string, any>> = {} as any;

// Load translation function
const loadTranslations = async (language: GuestLanguageCode): Promise<Record<string, any>> => {
  if (translations[language]) {
    return translations[language];
  }

  try {
    const response = await import(`./guest-messages/${language}.json`);
    translations[language] = response.default;
    return translations[language];
  } catch (error) {
    console.warn(`Failed to load translations for ${language}, falling back to English`);
    if (language !== 'en') {
      return loadTranslations('en');
    }
    return {};
  }
};

// Translation function with parameter support
const translateKey = (translations: Record<string, any>, key: string, params?: Record<string, any>): string => {
  const keys = key.split('.');
  let value: any = translations;
  
  for (const k of keys) {
    if (value && typeof value === 'object' && k in value) {
      value = value[k];
    } else {
      return key; // Return key if translation not found
    }
  }
  
  if (typeof value !== 'string') {
    return key;
  }
  
  // Replace parameters in the translation
  if (params) {
    return value.replace(/\{\{(\w+)\}\}/g, (match: string, paramKey: string) => {
      return params[paramKey]?.toString() || match;
    });
  }
  
  return value;
};

export function GuestTranslationProvider({ 
  children, 
  businessId: initialBusinessId, 
  initialLanguage = 'en' 
}: GuestTranslationProviderProps) {
  const [currentLanguage, setCurrentLanguage] = useState<GuestLanguageCode>(initialLanguage);
  const [currentTranslations, setCurrentTranslations] = useState<Record<string, any>>({});
  const [businessId, setBusinessId] = useState<number | undefined>(initialBusinessId);

  // Load translations when language changes
  useEffect(() => {
    loadTranslations(currentLanguage).then(setCurrentTranslations);
  }, [currentLanguage]);

  // Sync with business language preference if available
  useEffect(() => {
    if (businessId) {
      const savedLanguage = localStorage.getItem(`guest-language-${businessId}`) as GuestLanguageCode;
      if (savedLanguage && savedLanguage in GUEST_SUPPORTED_LANGUAGES && savedLanguage !== currentLanguage) {
        setCurrentLanguage(savedLanguage);
      }
    }
  }, [businessId, currentLanguage]);

  const setLanguage = (language: GuestLanguageCode) => {
    setCurrentLanguage(language);
    if (businessId) {
      localStorage.setItem(`guest-language-${businessId}`, language);
      // Also trigger business menu language change
      window.dispatchEvent(new CustomEvent('guestLanguageChange', { 
        detail: { language, businessId } 
      }));
    }
  };

  const t = (key: string, params?: Record<string, any>): string => {
    return translateKey(currentTranslations, key, params);
  };

  return (
    <GuestTranslationContext.Provider
      value={{
        currentLanguage,
        setLanguage,
        t,
        availableLanguages: GUEST_SUPPORTED_LANGUAGES,
        businessId,
        setBusinessId
      }}
    >
      {children}
    </GuestTranslationContext.Provider>
  );
}

export function useGuestTranslation() {
  const context = useContext(GuestTranslationContext);
  if (context === undefined) {
    throw new Error('useGuestTranslation must be used within a GuestTranslationProvider');
  }
  return context;
}
