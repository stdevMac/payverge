'use client';

import React from 'react';
import { useTranslation } from '../../contexts/TranslationContext';

interface SmartTranslatedTextProps {
  entityType: string;
  entityId: string | number;
  fieldName: string;
  originalText: string;
  languageCode: string;
  defaultLanguageCode?: string;
  className?: string;
  fallback?: React.ReactNode;
}

export function SmartTranslatedText({
  entityType,
  entityId,
  fieldName,
  originalText,
  languageCode,
  defaultLanguageCode = 'en',
  className = '',
  fallback
}: SmartTranslatedTextProps) {
  const { getTranslation, loading } = useTranslation();

  // If viewing default language, show original text
  if (languageCode === defaultLanguageCode) {
    return <span className={className}>{originalText}</span>;
  }

  // Get translation from bulk loaded data
  const translatedText = getTranslation(entityType, Number(entityId), fieldName);

  // Show loading state if translations are still loading
  if (loading && !translatedText) {
    return (
      <span className={`${className} animate-pulse`}>
        {fallback || originalText}
      </span>
    );
  }

  // Use translated text if available, otherwise fallback to original
  const displayText = translatedText || originalText;

  return <span className={className}>{displayText}</span>;
}
