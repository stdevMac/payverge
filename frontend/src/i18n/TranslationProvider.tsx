"use client";

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { locales, defaultLocale, type Locale } from './config';

// Import translation messages
import enMessages from './messages/en.json';
import esMessages from './messages/es.json';

const messages = {
  en: enMessages,
  es: esMessages
};

interface TranslationContextType {
  locale: Locale;
  t: (key: string, params?: Record<string, any>) => string;
  setLocale: (locale: Locale) => void;
}

const TranslationContext = createContext<TranslationContextType | undefined>(undefined);

// Get nested value from object using dot notation
function getNestedValue(obj: any, path: string): any {
  return path.split('.').reduce((current, key) => current?.[key], obj);
}

// Replace parameters in translation string
function interpolate(template: string, params: Record<string, any> = {}): string {
  return template.replace(/\{\{(\w+)\}\}/g, (match, key) => {
    return params[key] !== undefined ? String(params[key]) : match;
  });
}

export function TranslationProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(defaultLocale);

  // Initialize locale from localStorage or browser
  useEffect(() => {
    // Only run on client side
    if (typeof window === 'undefined') return;
    
    try {
      const savedLocale = localStorage.getItem('locale') as Locale;
      if (savedLocale && locales.includes(savedLocale)) {
        setLocaleState(savedLocale);
      } else {
        // Detect browser language
        const browserLang = navigator.language.split('-')[0];
        const detectedLocale = locales.includes(browserLang as Locale) ? (browserLang as Locale) : defaultLocale;
        setLocaleState(detectedLocale);
        localStorage.setItem('locale', detectedLocale);
      }
    } catch (error) {
      console.warn('Error accessing localStorage for locale:', error);
      setLocaleState(defaultLocale);
    }
  }, []);

  const setLocale = (newLocale: Locale) => {
    setLocaleState(newLocale);
    try {
      if (typeof window !== 'undefined') {
        localStorage.setItem('locale', newLocale);
      }
    } catch (error) {
      console.warn('Error saving locale to localStorage:', error);
    }
  };

  const t = (key: string, params?: Record<string, any>): string => {
    try {
      const message = getNestedValue(messages[locale], key);
      if (message === undefined) {
        // Fallback to English if key not found in current locale
        const fallbackMessage = getNestedValue(messages.en, key);
        if (fallbackMessage === undefined) {
          console.warn(`Translation key not found: ${key}`);
          return key;
        }
        return params ? interpolate(fallbackMessage, params) : fallbackMessage;
      }
      return params ? interpolate(message, params) : message;
    } catch (error) {
      console.error(`Translation error for key: ${key}`, error);
      return key;
    }
  };

  return (
    <TranslationContext.Provider value={{ locale, t, setLocale }}>
      {children}
    </TranslationContext.Provider>
  );
}

export function useTranslations(namespace?: string) {
  const context = useContext(TranslationContext);
  
  if (context === undefined) {
    throw new Error('useTranslations must be used within a TranslationProvider');
  }

  const { t: originalT, locale, setLocale } = context;

  // Create namespaced translation function
  const t = (key: string, params?: Record<string, any>): string => {
    const fullKey = namespace ? `${namespace}.${key}` : key;
    return originalT(fullKey, params);
  };

  return { t, locale, setLocale };
}

// Hook for getting current locale
export function useLocale(): Locale {
  const context = useContext(TranslationContext);
  if (context === undefined) {
    throw new Error('useLocale must be used within a TranslationProvider');
  }
  return context.locale;
}
