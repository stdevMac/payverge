"use client";

import { useTransition } from 'react';
import { Dropdown, DropdownTrigger, DropdownMenu, DropdownItem, Button } from "@nextui-org/react";
import { useTranslation } from '@/i18n/useTranslation';
import { locales, Locale } from '@/i18n/translations';
import { FaGlobe } from "react-icons/fa";

export default function LanguageSwitcher() {
  const [isPending, startTransition] = useTransition();
  const { language, changeLanguage } = useTranslation();

  // Function to change the language
  const handleChangeLanguage = (newLocale: Locale) => {
    if (newLocale === language) return;
    
    startTransition(() => {
      changeLanguage(newLocale);
    });
  };

  // Map of language codes to full names
  const languageNames = {
    en: "English",
    es: "Español"
  };

  return (
    <Dropdown placement="bottom-end">
      <DropdownTrigger>
        <Button 
          variant="flat" 
          size="sm" 
          className="min-w-0 px-3 font-medium rounded-full border border-neutral-200 hover:bg-neutral-100 transition-colors"
          startContent={
            <div className="flex items-center gap-1.5">
              <span className="text-base">{language === 'en' ? '🇺🇸' : '🇪🇸'}</span>
              <FaGlobe className="text-primary text-lg" />
            </div>
          }
        >
          {language.toUpperCase()}
        </Button>
      </DropdownTrigger>
      <DropdownMenu aria-label="Language selection">
        {locales.map((l) => (
          <DropdownItem
            key={l}
            textValue={languageNames[l as keyof typeof languageNames]}
            className={l === language ? "text-primary font-medium" : ""}
            startContent={<span className="text-lg">{l === 'en' ? '🇺🇸' : '🇪🇸'}</span>}
            onClick={() => handleChangeLanguage(l)}
          >
            {languageNames[l as keyof typeof languageNames]}
          </DropdownItem>
        ))}
      </DropdownMenu>
    </Dropdown>
  );
}
