'use client';

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { getMenuTranslations } from '../api/currency';

interface TranslationData {
  [entityType: string]: {
    [entityId: string]: {
      [fieldName: string]: string;
    };
  };
}

interface TranslationContextType {
  translations: TranslationData;
  loading: boolean;
  error: string | null;
  loadTranslations: (businessId: number, languageCode: string) => Promise<void>;
  getTranslation: (entityType: string, entityId: number, fieldName: string) => string | null;
}

const TranslationContext = createContext<TranslationContextType | undefined>(undefined);

interface TranslationProviderProps {
  children: ReactNode;
}

export function TranslationProvider({ children }: TranslationProviderProps) {
  const [translations, setTranslations] = useState<TranslationData>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentLanguage, setCurrentLanguage] = useState<string>('');
  const [currentBusinessId, setCurrentBusinessId] = useState<number | null>(null);

  const loadTranslations = async (businessId: number, languageCode: string) => {
    // Skip if we already have this data
    if (currentBusinessId === businessId && currentLanguage === languageCode && Object.keys(translations).length > 0) {
      return;
    }

    setLoading(true);
    setError(null);
    
    try {
      const response = await getMenuTranslations(businessId, languageCode);
      setTranslations(response.translations);
      setCurrentLanguage(languageCode);
      setCurrentBusinessId(businessId);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load translations');
      setTranslations({});
    } finally {
      setLoading(false);
    }
  };

  const getTranslation = (entityType: string, entityId: number, fieldName: string): string | null => {
    const entityIdStr = entityId.toString();
    return translations[entityType]?.[entityIdStr]?.[fieldName] || null;
  };

  return (
    <TranslationContext.Provider value={{
      translations,
      loading,
      error,
      loadTranslations,
      getTranslation
    }}>
      {children}
    </TranslationContext.Provider>
  );
}

export function useTranslation() {
  const context = useContext(TranslationContext);
  if (context === undefined) {
    throw new Error('useTranslation must be used within a TranslationProvider');
  }
  return context;
}
