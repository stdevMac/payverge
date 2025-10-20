'use client';

import React, { useState, useEffect } from 'react';
import { Dropdown, DropdownTrigger, DropdownMenu, DropdownItem, Button } from '@nextui-org/react';
import { FaGlobe } from "react-icons/fa";
import { getBusinessByTableCode } from '../../api/bills';
import type { BusinessLanguage, SupportedLanguage } from '../../api/currency';
import { useGuestTranslation, GUEST_SUPPORTED_LANGUAGES, type GuestLanguageCode } from '../../i18n/GuestTranslationProvider';

interface FloatingLanguageSelectorProps {
  tableCode: string;
  onClose?: () => void;
}

// Language flags mapping (matching common language codes)
const languageFlags: Record<string, string> = {
  'en': '🇺🇸',
  'es': '🇪🇸',
  'fr': '🇫🇷',
  'de': '🇩🇪',
  'it': '🇮🇹',
  'pt': '🇵🇹',
  'ru': '🇷🇺',
  'ja': '🇯🇵',
  'ko': '🇰🇷',
  'zh': '🇨🇳',
  'ar': '🇸🇦',
  'hi': '🇮🇳',
  'th': '🇹🇭',
  'vi': '🇻🇳',
  'tr': '🇹🇷',
  'pl': '🇵🇱',
  'nl': '🇳🇱',
  'sv': '🇸🇪',
  'da': '🇩🇰',
  'no': '🇳🇴'
};

export function FloatingLanguageSelector({ 
  tableCode, 
  onClose 
}: FloatingLanguageSelectorProps) {
  const { t, currentLanguage, setLanguage } = useGuestTranslation();
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedLanguage, setSelectedLanguage] = useState<string>('');
  const [isVisible, setIsVisible] = useState(false);
  const [businessId, setBusinessId] = useState<number | null>(null);
  const [isChanging, setIsChanging] = useState(false);

  useEffect(() => {
    const loadLanguages = async () => {
      try {
        setLoading(true);
        setError(null);

        // Load business data with languages from guest API
        const businessData = await getBusinessByTableCode(tableCode);
        
        const businessLangs = businessData.business_languages || [];
        const supportedLangs = businessData.supported_languages || [];
        const loadedBusinessId = businessData.business?.id;
        
        setBusinessId(loadedBusinessId);

        console.log('FloatingLanguageSelector: Loaded languages for table', tableCode, 'business', loadedBusinessId, ':', businessLangs);
        console.log('FloatingLanguageSelector: Business has', businessLangs.length, 'languages configured');
        
        setBusinessLanguages(businessLangs);
        setSupportedLanguages(supportedLangs);

        // Check for saved language preference first
        const savedLanguage = localStorage.getItem(`guest-language-${loadedBusinessId}`);
        const isValidSavedLanguage = savedLanguage && businessLangs.some((lang: BusinessLanguage) => lang.language_code === savedLanguage);

        if (isValidSavedLanguage) {
          setSelectedLanguage(savedLanguage);
          // Update guest UI language if supported
          if (savedLanguage in GUEST_SUPPORTED_LANGUAGES) {
            setLanguage(savedLanguage as GuestLanguageCode);
          }
        } else {
          // Fall back to default language
          const defaultLang = businessLangs.find((lang: BusinessLanguage) => lang.is_default);
          if (defaultLang) {
            setSelectedLanguage(defaultLang.language_code);
            // Update guest UI language if supported
            if (defaultLang.language_code in GUEST_SUPPORTED_LANGUAGES) {
              setLanguage(defaultLang.language_code as GuestLanguageCode);
            }
          }
        }

        // Show the widget after a short delay if there are multiple languages
        // TEMP: Show even with one language for testing
        if (businessLangs.length >= 1) {
          console.log('FloatingLanguageSelector: Will show widget in 1 second');
          setTimeout(() => {
            console.log('FloatingLanguageSelector: Showing widget now');
            setIsVisible(true);
          }, 1000);
        } else {
          console.log('FloatingLanguageSelector: Not showing widget - no languages configured');
        }
      } catch (err: any) {
        console.error('FloatingLanguageSelector: Detailed error loading languages:', {
          error: err,
          message: err.message,
          response: err.response?.data,
          status: err.response?.status,
          tableCode
        });
        setError('Failed to load languages');
      } finally {
        setLoading(false);
      }
    };

    if (tableCode) {
      loadLanguages();
    }
  }, [tableCode, setLanguage]);

  const handleLanguageChange = (languageCode: string) => {
    if (languageCode === selectedLanguage) return;
    
    setIsChanging(true);
    setSelectedLanguage(languageCode);
    
    // Save to localStorage (use businessId from the loaded data)
    if (businessId) {
      localStorage.setItem(`guest-language-${businessId}`, languageCode);
    }
    
    // Update guest UI language if supported
    if (languageCode in GUEST_SUPPORTED_LANGUAGES) {
      setLanguage(languageCode as GuestLanguageCode);
    }

    // Reset the changing state and auto-hide after selection
    setTimeout(() => {
      setIsChanging(false);
      setIsVisible(false);
      onClose?.();
    }, 500);
  };

  // Get language display name
  const getLanguageDisplayName = (languageCode: string): string => {
    const supportedLang = supportedLanguages.find(lang => lang.code === languageCode);
    if (supportedLang) {
      return supportedLang.native_name || supportedLang.name;
    }
    return languageCode.toUpperCase();
  };

  // Debug: Show current state
  console.log('FloatingLanguageSelector render check:', {
    loading,
    error,
    businessLanguagesCount: businessLanguages.length,
    isVisible,
    tableCode
  });

  // Don't show if loading, error, or no languages
  // TEMP: Show even with one language for testing
  if (loading || error || businessLanguages.length < 1 || !isVisible) {
    return null;
  }

  return (
    <div className="fixed top-6 right-6 z-50 animate-in slide-in-from-top-2 duration-300">
      <Dropdown placement="bottom-end">
        <DropdownTrigger>
          <Button 
            variant="flat" 
            size="sm" 
            className="min-w-0 px-3 font-medium rounded-full border border-neutral-200 hover:bg-neutral-100 transition-colors"
            startContent={
              <div className="flex items-center gap-1.5">
                <span className="text-base">{languageFlags[selectedLanguage] || '🌐'}</span>
                <FaGlobe className="text-primary text-lg" />
              </div>
            }
            isDisabled={isChanging}
          >
            {selectedLanguage.toUpperCase()}
          </Button>
        </DropdownTrigger>
        <DropdownMenu aria-label="Language selection">
          {businessLanguages.map((lang) => (
            <DropdownItem
              key={lang.language_code}
              textValue={getLanguageDisplayName(lang.language_code)}
              className={lang.language_code === selectedLanguage ? "text-primary font-medium" : ""}
              startContent={<span className="text-lg">{languageFlags[lang.language_code] || '🌐'}</span>}
              onClick={() => handleLanguageChange(lang.language_code)}
            >
              <div className="flex items-center justify-between w-full">
                <span>{getLanguageDisplayName(lang.language_code)}</span>
                {lang.is_default && (
                  <span className="text-xs text-blue-600 font-medium ml-2">Default</span>
                )}
              </div>
            </DropdownItem>
          ))}
        </DropdownMenu>
      </Dropdown>
    </div>
  );
}
