'use client';

import React, { useState, useEffect } from 'react';
import { Select, SelectItem, Spinner } from '@nextui-org/react';
import { Globe } from 'lucide-react';
import { getBusinessLanguages, getSupportedLanguages } from '../../api/currency';
import type { BusinessLanguage, SupportedLanguage } from '../../api/currency';
import { useGuestTranslation, GUEST_SUPPORTED_LANGUAGES, type GuestLanguageCode } from '../../i18n/GuestTranslationProvider';

interface GuestLanguageSelectorProps {
  businessId: number;
  selectedLanguage: string;
  onLanguageChange: (languageCode: string) => void;
  className?: string;
}

export function GuestLanguageSelector({ 
  businessId, 
  selectedLanguage, 
  onLanguageChange, 
  className = '' 
}: GuestLanguageSelectorProps) {
  // Try to use guest translation, but fallback if not available
  let t: (key: string) => string;
  let currentLanguage: string;
  let setLanguage: ((lang: any) => void) | undefined;
  
  try {
    const guestTranslation = useGuestTranslation();
    t = guestTranslation.t;
    currentLanguage = guestTranslation.currentLanguage;
    setLanguage = guestTranslation.setLanguage;
  } catch {
    // Fallback if provider not available
    t = (key: string) => key;
    currentLanguage = 'en';
    setLanguage = undefined;
  }
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadLanguages = async () => {
      try {
        setLoading(true);
        setError(null);

        // Load both business languages and supported languages
        const [businessLangs, supportedLangs] = await Promise.all([
          getBusinessLanguages(businessId),
          getSupportedLanguages()
        ]);

        setBusinessLanguages(businessLangs);
        setSupportedLanguages(supportedLangs);

        // Check for saved language preference first
        const savedLanguage = localStorage.getItem(`guest-language-${businessId}`);
        const isValidSavedLanguage = savedLanguage && businessLangs.some(lang => lang.language_code === savedLanguage);

        if (!selectedLanguage && businessLangs.length > 0) {
          if (isValidSavedLanguage) {
            // Use saved language if valid
            onLanguageChange(savedLanguage);
          } else {
            // Fall back to default language
            const defaultLang = businessLangs.find(lang => lang.is_default);
            if (defaultLang) {
              onLanguageChange(defaultLang.language_code);
            }
          }
        }
      } catch (err: any) {
        console.error('Error loading languages:', err);
        setError('Failed to load languages');
      } finally {
        setLoading(false);
      }
    };

    if (businessId) {
      loadLanguages();
    }
  }, [businessId, selectedLanguage, onLanguageChange]);

  // Get language display name
  const getLanguageDisplayName = (languageCode: string): string => {
    const supportedLang = supportedLanguages.find(lang => lang.code === languageCode);
    if (supportedLang) {
      return `${supportedLang.native_name} (${supportedLang.name})`;
    }
    return languageCode.toUpperCase();
  };

  // Don't show selector if only one language or loading
  if (loading) {
    return (
      <div className={`flex items-center gap-2 ${className}`}>
        <Globe className="w-4 h-4 text-gray-400" />
        <Spinner size="sm" />
      </div>
    );
  }

  if (error || businessLanguages.length <= 1) {
    return null;
  }

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <Globe className="w-4 h-4 text-gray-600" />
      <Select
        size="sm"
        variant="bordered"
        selectedKeys={selectedLanguage ? [selectedLanguage] : []}
        onSelectionChange={(keys) => {
          const selected = Array.from(keys)[0] as string;
          if (selected) {
            // Save language preference to localStorage
            localStorage.setItem(`guest-language-${businessId}`, selected);
            
            // Update both menu language and UI language
            onLanguageChange(selected);
            
            // Update guest UI language if supported and setLanguage is available
            if (selected in GUEST_SUPPORTED_LANGUAGES && setLanguage) {
              setLanguage(selected as GuestLanguageCode);
            }
          }
        }}
        className="min-w-[140px]"
        classNames={{
          trigger: "h-8 min-h-8 bg-white/80 backdrop-blur-sm border-gray-200",
          value: "text-sm font-medium",
          popoverContent: "bg-white/95 backdrop-blur-sm"
        }}
        aria-label={t('accessibility.languageSelector')}
      >
        {businessLanguages.map((lang) => (
          <SelectItem 
            key={lang.language_code} 
            value={lang.language_code}
            className="text-sm"
          >
            {getLanguageDisplayName(lang.language_code)}
          </SelectItem>
        ))}
      </Select>
    </div>
  );
}
