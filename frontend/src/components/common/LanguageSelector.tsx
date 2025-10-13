'use client';

import React, { useState, useEffect } from 'react';
import {
  Select,
  SelectItem,
  Button,
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
  Chip
} from '@nextui-org/react';
import { Languages, Globe, Check } from 'lucide-react';
import {
  getBusinessLanguages,
  getSupportedLanguages,
  getTranslatedContent,
  BusinessLanguage,
  SupportedLanguage,
  detectBrowserLanguage,
  getLanguageName,
  getLanguageNativeName
} from '../../api/currency';

interface LanguageSelectorProps {
  businessId?: number;
  selectedLanguage: string;
  onLanguageChange: (language: string) => void;
  availableLanguages?: SupportedLanguage[];
  showNativeNames?: boolean;
  variant?: 'select' | 'dropdown' | 'chips';
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export default function LanguageSelector({
  businessId,
  selectedLanguage,
  onLanguageChange,
  availableLanguages = [],
  showNativeNames = true,
  variant = 'dropdown',
  size = 'md',
  className = ''
}: LanguageSelectorProps) {
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (businessId) {
      loadBusinessLanguages();
    } else if (availableLanguages.length > 0) {
      setSupportedLanguages(availableLanguages);
    } else {
      loadAllSupportedLanguages();
    }
  }, [businessId, availableLanguages]);

  const loadBusinessLanguages = async () => {
    if (!businessId) return;

    try {
      setLoading(true);
      setError(null);

      const [businessLangs, allLangs] = await Promise.all([
        getBusinessLanguages(businessId),
        getSupportedLanguages()
      ]);

      setBusinessLanguages(businessLangs);
      setSupportedLanguages(allLangs);

      // Auto-select default language if none selected
      if (!selectedLanguage) {
        const defaultLang = businessLangs.find(l => l.is_default);
        if (defaultLang) {
          onLanguageChange(defaultLang.language_code);
        } else if (businessLangs.length > 0) {
          onLanguageChange(businessLangs[0].language_code);
        }
      }

    } catch (err: any) {
      console.error('Error loading business languages:', err);
      setError('Failed to load languages');
    } finally {
      setLoading(false);
    }
  };

  const loadAllSupportedLanguages = async () => {
    try {
      setLoading(true);
      const languages = await getSupportedLanguages();
      setSupportedLanguages(languages);
    } catch (err: any) {
      console.error('Error loading supported languages:', err);
      setError('Failed to load languages');
    } finally {
      setLoading(false);
    }
  };

  const getAvailableLanguages = (): SupportedLanguage[] => {
    if (businessId && businessLanguages.length > 0) {
      // Filter supported languages to only show business languages
      return supportedLanguages.filter(lang => 
        businessLanguages.some(bl => bl.language_code === lang.code)
      );
    }
    return supportedLanguages;
  };

  const getCurrentLanguage = (): SupportedLanguage | null => {
    return supportedLanguages.find(lang => lang.code === selectedLanguage) || null;
  };

  const formatLanguageName = (language: SupportedLanguage): string => {
    if (showNativeNames && language.native_name !== language.name) {
      return `${language.name} (${language.native_name})`;
    }
    return language.name;
  };

  const displayLanguages = getAvailableLanguages();
  const currentLanguage = getCurrentLanguage();

  if (loading) {
    return (
      <div className={`flex items-center gap-2 ${className}`}>
        <Languages className="w-4 h-4 text-gray-400 animate-pulse" />
        <span className="text-sm text-gray-400">Loading...</span>
      </div>
    );
  }

  if (error || displayLanguages.length <= 1) {
    return null; // Don't show selector if there's only one language or an error
  }

  // Select variant
  if (variant === 'select') {
    return (
      <Select
        size={size}
        selectedKeys={selectedLanguage ? [selectedLanguage] : []}
        onSelectionChange={(keys) => {
          const selected = Array.from(keys)[0] as string;
          onLanguageChange(selected);
        }}
        className={className}
        startContent={<Languages className="w-4 h-4" />}
        placeholder="Select language"
      >
        {displayLanguages.map((language) => (
          <SelectItem key={language.code} value={language.code}>
            {formatLanguageName(language)}
          </SelectItem>
        ))}
      </Select>
    );
  }

  // Chips variant
  if (variant === 'chips') {
    return (
      <div className={`flex flex-wrap gap-2 ${className}`}>
        {displayLanguages.map((language) => (
          <Chip
            key={language.code}
            color={language.code === selectedLanguage ? 'primary' : 'default'}
            variant={language.code === selectedLanguage ? 'solid' : 'flat'}
            onClick={() => onLanguageChange(language.code)}
            className="cursor-pointer"
            size={size}
          >
            {showNativeNames ? language.native_name : language.name}
          </Chip>
        ))}
      </div>
    );
  }

  // Dropdown variant (default)
  return (
    <Dropdown>
      <DropdownTrigger>
        <Button
          variant="flat"
          size={size}
          startContent={<Globe className="w-4 h-4" />}
          className={className}
        >
          {currentLanguage ? (
            showNativeNames ? currentLanguage.native_name : currentLanguage.name
          ) : (
            'Select Language'
          )}
        </Button>
      </DropdownTrigger>
      <DropdownMenu
        aria-label="Language selection"
        selectedKeys={selectedLanguage ? [selectedLanguage] : []}
        onSelectionChange={(keys) => {
          const selected = Array.from(keys)[0] as string;
          onLanguageChange(selected);
        }}
      >
        {displayLanguages.map((language) => (
          <DropdownItem
            key={language.code}
            startContent={
              language.code === selectedLanguage ? (
                <Check className="w-4 h-4 text-primary" />
              ) : (
                <div className="w-4 h-4" />
              )
            }
          >
            {formatLanguageName(language)}
          </DropdownItem>
        ))}
      </DropdownMenu>
    </Dropdown>
  );
}

// Hook for managing translated content
export function useTranslatedContent(
  entityType: string,
  entityId: number,
  fieldName: string,
  originalText: string,
  selectedLanguage: string,
  defaultLanguage: string = 'en'
) {
  const [translatedText, setTranslatedText] = useState<string>(originalText);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (selectedLanguage === defaultLanguage) {
      setTranslatedText(originalText);
      return;
    }

    const loadTranslation = async () => {
      try {
        setLoading(true);
        const translation = await getTranslatedContent(
          entityType,
          entityId,
          fieldName,
          selectedLanguage
        );
        setTranslatedText(translation || originalText);
      } catch (error) {
        console.warn(`Translation not found for ${entityType}:${entityId}:${fieldName}:${selectedLanguage}`);
        setTranslatedText(originalText); // Fallback to original
      } finally {
        setLoading(false);
      }
    };

    loadTranslation();
  }, [entityType, entityId, fieldName, originalText, selectedLanguage, defaultLanguage]);

  return { translatedText, loading };
}

// Auto-detect and set browser language
export function useAutoLanguageDetection(
  availableLanguages: string[],
  onLanguageChange: (language: string) => void,
  defaultLanguage: string = 'en'
) {
  useEffect(() => {
    const browserLang = detectBrowserLanguage();
    
    // Check if browser language is available
    if (availableLanguages.includes(browserLang)) {
      onLanguageChange(browserLang);
    } else {
      // Check for language family match (e.g., 'en' for 'en-US')
      const familyMatch = availableLanguages.find(lang => 
        lang.startsWith(browserLang) || browserLang.startsWith(lang)
      );
      
      if (familyMatch) {
        onLanguageChange(familyMatch);
      } else {
        onLanguageChange(defaultLanguage);
      }
    }
  }, [availableLanguages, onLanguageChange, defaultLanguage]);
}

// Component for displaying text with language fallback
interface TranslatedTextProps {
  entityType: string;
  entityId: number;
  fieldName: string;
  originalText: string;
  selectedLanguage: string;
  defaultLanguage?: string;
  className?: string;
  fallbackComponent?: React.ReactNode;
}

export function TranslatedText({
  entityType,
  entityId,
  fieldName,
  originalText,
  selectedLanguage,
  defaultLanguage = 'en',
  className = '',
  fallbackComponent
}: TranslatedTextProps) {
  const { translatedText, loading } = useTranslatedContent(
    entityType,
    entityId,
    fieldName,
    originalText,
    selectedLanguage,
    defaultLanguage
  );

  if (loading && fallbackComponent) {
    return <>{fallbackComponent}</>;
  }

  return <span className={className}>{translatedText}</span>;
}
