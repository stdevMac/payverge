"use client";

import React, { createContext, useContext, useState, ReactNode } from 'react';
import { locales, defaultLocale, type Locale } from './config';

// Import translation messages
import enMessages from './messages/en.json';
import esMessages from './messages/es.json';

const messages = {
  en: enMessages,
  es: esMessages
};

interface SimpleTranslationContextType {
  locale: Locale;
  setLocale: (locale: Locale) => void;
}

const SimpleTranslationContext = createContext<SimpleTranslationContextType | undefined>(undefined);

export function SimpleTranslationProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(defaultLocale);

  // Initialize locale from localStorage on client side
  React.useEffect(() => {
    if (typeof window !== 'undefined') {
      try {
        const savedLocale = localStorage.getItem('locale') as Locale;
        if (savedLocale && locales.includes(savedLocale)) {
          setLocaleState(savedLocale);
        }
      } catch (error) {
        console.warn('Error reading locale from localStorage:', error);
      }
    }
  }, []);

  const setLocale = (newLocale: Locale) => {
    setLocaleState(newLocale);
    if (typeof window !== 'undefined') {
      try {
        localStorage.setItem('locale', newLocale);
      } catch (error) {
        console.warn('Error saving locale to localStorage:', error);
      }
    }
  };

  return (
    <SimpleTranslationContext.Provider value={{ locale, setLocale }}>
      {children}
    </SimpleTranslationContext.Provider>
  );
}

// Simple translation function that doesn't use hooks
export function getTranslation(key: string, locale: Locale = defaultLocale): string | string[] {
  try {
    const keys = key.split('.');
    let value: any = messages[locale];
    
    for (const k of keys) {
      value = value?.[k];
    }
    
    if (typeof value === 'string' || Array.isArray(value)) {
      return value;
    }
    
    // Fallback to English
    value = messages.en;
    for (const k of keys) {
      value = value?.[k];
    }
    
    return (typeof value === 'string' || Array.isArray(value)) ? value : key;
  } catch (error) {
    return key;
  }
}

export function useSimpleLocale() {
  const context = useContext(SimpleTranslationContext);
  if (context === undefined) {
    return { locale: defaultLocale, setLocale: () => {} };
  }
  return context;
}
