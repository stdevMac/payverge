'use client';

import React, { useState, useEffect } from 'react';
import { getTranslatedContent } from '../../api/currency';

interface TranslatedContentProps {
  entityType: 'menu_item' | 'category';
  entityId: number;
  fieldName: 'name' | 'description';
  languageCode: string;
  defaultLanguageCode: string;
  originalText: string;
  className?: string;
}

export default function TranslatedContent({
  entityType,
  entityId,
  fieldName,
  languageCode,
  defaultLanguageCode,
  originalText,
  className = ''
}: TranslatedContentProps) {
  const [translatedText, setTranslatedText] = useState<string>(originalText);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    // If viewing default language, show original text
    if (languageCode === defaultLanguageCode || !entityId) {
      setTranslatedText(originalText);
      return;
    }

    const loadTranslation = async () => {
      try {
        setIsLoading(true);
        const translation = await getTranslatedContent(entityType, entityId, fieldName, languageCode);
        
        if (translation && translation.trim()) {
          setTranslatedText(translation);
        } else {
          // If no translation exists, fallback to original text
          setTranslatedText(originalText);
        }
      } catch (error) {
        console.error(`Failed to load translation for ${entityType} ${entityId} ${fieldName}:`, error);
        // Fallback to original text if translation fails
        setTranslatedText(originalText);
      } finally {
        setIsLoading(false);
      }
    };

    loadTranslation();
  }, [entityType, entityId, fieldName, languageCode, defaultLanguageCode, originalText]);

  if (isLoading) {
    return (
      <span className={`${className} animate-pulse bg-gray-200 rounded`}>
        {originalText}
      </span>
    );
  }

  return (
    <span className={className}>
      {translatedText}
    </span>
  );
}
