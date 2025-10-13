'use client';

import { useState, useEffect, useCallback } from 'react';
import { getMenuItemTranslations, getCategoryTranslations } from '../api/currency';

interface TranslationCache {
  [key: string]: {
    name: string;
    description: string;
  };
}

interface UseTranslationsProps {
  businessId: number;
  languageCode: string;
  defaultLanguageCode: string;
}

export function useTranslations({ businessId, languageCode, defaultLanguageCode }: UseTranslationsProps) {
  const [categoryTranslations, setCategoryTranslations] = useState<TranslationCache>({});
  const [itemTranslations, setItemTranslations] = useState<TranslationCache>({});
  const [isLoading, setIsLoading] = useState(false);
  const [loadedLanguages, setLoadedLanguages] = useState<Set<string>>(new Set());

  // If viewing default language, don't load translations
  const shouldLoadTranslations = languageCode !== defaultLanguageCode;

  const loadTranslationsForLanguage = useCallback(async (langCode: string) => {
    if (!shouldLoadTranslations || loadedLanguages.has(langCode)) {
      return;
    }

    try {
      setIsLoading(true);
      
      // We'll load translations on-demand when needed
      // This hook just manages the cache
      
      setLoadedLanguages(prev => new Set(Array.from(prev).concat(langCode)));
    } catch (error) {
      console.error('Failed to load translations:', error);
    } finally {
      setIsLoading(false);
    }
  }, [shouldLoadTranslations, loadedLanguages]);

  const getCategoryTranslation = useCallback(async (
    categoryId: number, 
    field: 'name' | 'description', 
    originalText: string
  ): Promise<string> => {
    if (!shouldLoadTranslations) {
      return originalText;
    }

    const cacheKey = `category_${categoryId}_${languageCode}`;
    
    // Check cache first
    if (categoryTranslations[cacheKey]) {
      return categoryTranslations[cacheKey][field] || originalText;
    }

    // Load translation if not in cache
    try {
      const translations = await getCategoryTranslations(businessId, categoryId, [languageCode]);
      const translation = translations[languageCode];
      
      if (translation) {
        setCategoryTranslations(prev => ({
          ...prev,
          [cacheKey]: translation
        }));
        return translation[field] || originalText;
      }
    } catch (error) {
      console.error('Failed to load category translation:', error);
    }

    return originalText;
  }, [businessId, languageCode, shouldLoadTranslations, categoryTranslations]);

  const getItemTranslation = useCallback(async (
    itemId: number, 
    field: 'name' | 'description', 
    originalText: string
  ): Promise<string> => {
    if (!shouldLoadTranslations) {
      return originalText;
    }

    const cacheKey = `item_${itemId}_${languageCode}`;
    
    // Check cache first
    if (itemTranslations[cacheKey]) {
      return itemTranslations[cacheKey][field] || originalText;
    }

    // Load translation if not in cache
    try {
      const translations = await getMenuItemTranslations(businessId, itemId, [languageCode]);
      const translation = translations[languageCode];
      
      if (translation) {
        setItemTranslations(prev => ({
          ...prev,
          [cacheKey]: translation
        }));
        return translation[field] || originalText;
      }
    } catch (error) {
      console.error('Failed to load item translation:', error);
    }

    return originalText;
  }, [businessId, languageCode, shouldLoadTranslations, itemTranslations]);

  // Clear cache when language changes
  useEffect(() => {
    if (shouldLoadTranslations) {
      setCategoryTranslations({});
      setItemTranslations({});
      setLoadedLanguages(new Set());
    }
  }, [languageCode, shouldLoadTranslations]);

  return {
    getCategoryTranslation,
    getItemTranslation,
    isLoading,
    shouldLoadTranslations
  };
}
