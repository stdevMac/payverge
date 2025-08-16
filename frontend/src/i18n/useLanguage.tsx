"use client";

import { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { useUserStore } from '@/store/useUserStore';
import { setUserLanguage } from '@/api/users/profile';

interface LanguageContextType {
  language: string;
  setLanguage: (lang: string) => void;
}

const LanguageContext = createContext<LanguageContextType>({
  language: 'en',
  setLanguage: () => {},
});

interface LanguageProviderProps {
  children: ReactNode;
}

export const LanguageProvider = ({ children }: LanguageProviderProps) => {
  const [language, setLanguageState] = useState<string>('en');
  const { user, setUser } = useUserStore();

  // Load initial language on mount
  useEffect(() => {
    const loadInitialLanguage = async () => {
      // First check user preference if it's a valid language
      if (user?.language_selected === 'en' || user?.language_selected === 'es') {
        console.log('[useLanguage] Using user preference:', user.language_selected);
        setLanguageState(user.language_selected);
        localStorage.setItem('language', user.language_selected);
        return;
      }
      
      // Handle empty string or null language case for logged in users
      if ((user?.language_selected === '' || user?.language_selected === null) && user?.address) {
        console.log('[useLanguage] User has empty/null language preference, updating...');
        
        // Prioritize language from localStorage
        const savedLanguage = localStorage.getItem('language');
        const langToUse = savedLanguage && (savedLanguage === 'en' || savedLanguage === 'es')
          ? savedLanguage
          : navigator.language.split('-')[0] === 'es' ? 'es' : 'en';
        
        try {
          // Update user's language preference in the backend
          console.log('[useLanguage] Setting empty/null language to:', langToUse);
          const updatedUser = await setUserLanguage(user.address, langToUse);
          
          if (updatedUser?.language_selected === langToUse) {
            console.log('[useLanguage] Successfully updated empty/null language preference');
            setUser({ ...user, language_selected: langToUse });
            setLanguageState(langToUse);
            localStorage.setItem('language', langToUse);
            return;
          }
        } catch (error) {
          console.error('[useLanguage] Failed to update empty/null language preference:', error);
          // Continue with local settings if API call fails
        }
      }

      // Then check localStorage
      const savedLanguage = localStorage.getItem('language');
      if (savedLanguage && (savedLanguage === 'en' || savedLanguage === 'es')) {
        console.log('[useLanguage] Using localStorage:', savedLanguage);
        setLanguageState(savedLanguage);
        return;
      }

      // Finally use browser language
      const browserLang = navigator.language.split('-')[0];
      const defaultLang = browserLang === 'es' ? 'es' : 'en';
      console.log('[useLanguage] Using browser language:', defaultLang);
      setLanguageState(defaultLang);
      localStorage.setItem('language', defaultLang);
    };

    loadInitialLanguage();
  }, [user?.language_selected, user?.address, setUser, user]); // Re-run when user preference or address changes

  const setLanguage = async (lang: string) => {
    console.log('[useLanguage] setLanguage called with:', lang);
    if (lang !== 'en' && lang !== 'es') {
      console.warn('[useLanguage] Invalid language:', lang);
      return;
    }

    try {
      // Update UI state first
      setLanguageState(lang);
      localStorage.setItem('language', lang);
      console.log('[useLanguage] Updated UI to:', lang);

      // Then update API if user is logged in
      if (user?.address) {
        try {
          console.log('[useLanguage] Updating API...');
          const updatedUser = await setUserLanguage(user.address, lang);
          
          if (updatedUser?.language_selected === lang) {
            console.log('[useLanguage] API update successful');
            setUser({ ...user, language_selected: lang });
          } else {
            console.warn('[useLanguage] API returned unexpected state:', updatedUser?.language_selected);
          }
        } catch (apiError) {
          console.error('[useLanguage] API update failed:', apiError);
          // Don't revert UI on API error
        }
      }
    } catch (error) {
      console.error('[useLanguage] Critical error:', error);
      // Only revert on critical errors
      const savedLanguage = localStorage.getItem('language');
      if (savedLanguage && (savedLanguage === 'en' || savedLanguage === 'es')) {
        setLanguageState(savedLanguage);
      }
    }
  };

  return (
    <LanguageContext.Provider value={{ language, setLanguage }}>
      {children}
    </LanguageContext.Provider>
  );
};

export const useLanguage = () => useContext(LanguageContext);
