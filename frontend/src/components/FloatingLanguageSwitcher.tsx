"use client";

import { useTransition, useEffect, useState } from 'react';
import { Button } from "@nextui-org/react";
import { useTranslation } from '@/i18n/useTranslation';
import { locales, Locale } from '@/i18n/translations';
import { FaGlobe } from "react-icons/fa";
import { useShareBarStore } from '@/store/useShareBarStore';

export default function FloatingLanguageSwitcher() {
  const [isPending, startTransition] = useTransition();
  const { language, changeLanguage } = useTranslation();
  const { isExpanded, expandedHeight } = useShareBarStore();
  const [position, setPosition] = useState("bottom-24");

  const languageNames = {
    en: "English",
    es: "Español"
  };

  // Function to toggle between languages
  const toggleLanguage = () => {
    const newLocale = language === 'en' ? 'es' : 'en';
    startTransition(() => {
      changeLanguage(newLocale);
    });
  };

  // Listen for custom event from ShareBar
  useEffect(() => {
    const handleShareBarChange = (event: any) => {
      if (event.detail.isExpanded) {
        // Move just slightly above the ShareBar component
        setPosition("bottom-[580px]");
      } else {
        setPosition("bottom-24");
      }
    };

    // Add event listener
    window.addEventListener('shareBarExpandedChange', handleShareBarChange);
    
    // Initial position based on current state
    if (isExpanded) {
      setPosition("bottom-[580px]");
    }
    
    // Clean up
    return () => {
      window.removeEventListener('shareBarExpandedChange', handleShareBarChange);
    };
  }, [isExpanded]); // Add isExpanded to the dependency array

  return (
    <div className={`fixed ${position} right-5 md:hidden z-[100] transition-all duration-300 ease-in-out`}>
      <Button
        variant="flat"
        size="sm"
        className={
          `min-w-0 px-3 py-2 font-medium rounded-full border border-neutral-200 bg-white shadow-lg ` +
          `hover:bg-neutral-100 active:scale-95 transition-all flex items-center gap-1.5`
        }
        startContent={
          <>
            <span className="text-base">{language === 'en' ? '🇺🇸' : '🇪🇸'}</span>
            <FaGlobe className="text-primary text-lg" />
          </>
        }
        onClick={toggleLanguage}
        aria-label={
          language === 'en'
            ? `Switch language to Español`
            : `Cambiar idioma a English`
        }
        disabled={isPending}
      >
        {language.toUpperCase()}
      </Button>
    </div>
  );
}
