"use client";

import React, { createContext, useContext, ReactNode } from 'react';
import { translations, Locale } from './translations';
import { useLanguage } from './useLanguage';
import { useUserStore } from '@/store/useUserStore';

interface TranslationContextType {
  language: Locale;
  t: (key: string, params?: Record<string, any> | string, defaultValue?: string) => string;
  changeLanguage: (newLanguage: Locale) => void;
}

const TranslationContext = createContext<TranslationContextType | undefined>(undefined);

// Get the browser language or default to English
const getBrowserLanguage = (): Locale => {
  if (typeof window === 'undefined') return 'en';
  
  const browserLang = navigator.language.split('-')[0];
  return (browserLang === 'es' ? 'es' : 'en') as Locale;
};

// Get the stored language preference or use browser language
const getInitialLanguage = (): Locale => {
  if (typeof window === 'undefined') return 'en';
  
  const storedLang = localStorage.getItem('language') as Locale;
  if (storedLang === 'en' || storedLang === 'es') {
    return storedLang;
  }
  return getBrowserLanguage();
};

export const TranslationProvider = ({ children }: { children: ReactNode }) => {
  const { language: userLanguage, setLanguage } = useLanguage();
  const { user } = useUserStore();
  
  console.log('[TranslationProvider] State:', {
    userLanguage,
    userPreference: user?.language_selected,
    localStorage: typeof window !== 'undefined' ? localStorage.getItem('language') : null
  });
  
  // First check user's selected language
  const language = (() => {
    // If user has a valid preference, use it
    if (user?.language_selected && 
        (user.language_selected === 'en' || user.language_selected === 'es')) {
      return user.language_selected as Locale;
    }
    
    // Otherwise use current language if valid
    if (userLanguage === 'en' || userLanguage === 'es') {
      return userLanguage as Locale;
    }
    
    // Finally fall back to initial language
    return getInitialLanguage();
  })();
  
  console.log('[TranslationProvider] Using language:', language);
  
  // Define changeLanguage function that will update the language in LanguageProvider
  const changeLanguage = (newLanguage: Locale) => {
    console.log('[TranslationProvider] Changing language to:', newLanguage);
    if (newLanguage === 'en' || newLanguage === 'es') {
      setLanguage(newLanguage);
    }
  };
  
  // Function to get a translation by key with optional parameters and default value
  const t = (key: string, params?: Record<string, any> | string, defaultValue?: string): string => {
    try {
      // Handle case where params is omitted but defaultValue is provided
      if (typeof params === 'string' && defaultValue === undefined) {
        defaultValue = params;
        params = undefined;
      }

      // Special handling for onboarding steps
      if (key.startsWith('onboarding.steps.step')) {
        const match = key.match(/onboarding\.steps\.step(\d+)\.(title|description)/);
        if (match) {
          const [, stepNumber, property] = match;
          const stepKey = `step${stepNumber}`;
          const translationObj = translations[language] as any;
          
          if (translationObj?.onboarding?.steps?.[stepKey]?.[property]) {
            return translationObj.onboarding.steps[stepKey][property];
          }
        }
      }
      
      // Regular translation lookup
      const keys = key.split('.');
      let result: any = translations[language];
      
      for (const k of keys) {
        if (result === undefined) return defaultValue || key;
        result = result[k];
      }
      
      let translatedText = result === undefined ? (defaultValue || key) : result;
      
      // Apply parameter interpolation if params are provided
      if (params && typeof translatedText === 'string') {
        Object.entries(params).forEach(([paramKey, paramValue]) => {
          translatedText = translatedText.replace(new RegExp(`\{\{${paramKey}\}\}`, 'g'), String(paramValue));
        });
      }
      
      return translatedText;
    } catch (error) {
      console.error(`Translation error for key: ${key}`, error);
      return defaultValue || key;
    }
  };
  
  return (
    <TranslationContext.Provider value={{ language, t, changeLanguage }}>
      {children}
    </TranslationContext.Provider>
  );
};

export const useTranslation = () => {
  const context = useContext(TranslationContext);
  
  // Provide a fallback for admin views or other areas without TranslationProvider
  if (context === undefined) {
    // Return a dummy implementation that won't break the app
    return {
      language: 'en',
      t: (key: string, params?: Record<string, any> | string, defaultValue?: string) => {
        // Handle case where params is a string (defaultValue)
        if (typeof params === 'string') {
          return params || key;
        }
        // Handle case where params is an object (interpolation params)
        return defaultValue || key;
      },
      changeLanguage: (newLanguage: string) => {}
    };
  }
  
  return context;
};
