"use client";

import { useState } from 'react';
import { Dropdown, DropdownTrigger, DropdownMenu, DropdownItem, Button } from "@nextui-org/react";
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
import { locales, languageNames, languageFlags, type Locale } from '@/i18n/config';
import { FaGlobe } from "react-icons/fa";

export default function SimpleLanguageSwitcher() {
  const { locale, setLocale } = useSimpleLocale();
  const [isChanging, setIsChanging] = useState(false);

  const handleChangeLanguage = (newLocale: Locale) => {
    if (newLocale === locale) return;
    
    setIsChanging(true);
    setLocale(newLocale);
    
    // Reset the changing state after a brief moment
    setTimeout(() => {
      setIsChanging(false);
    }, 500);
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
              <span className="text-base">{languageFlags[locale]}</span>
              <FaGlobe className="text-primary text-lg" />
            </div>
          }
          isDisabled={isChanging}
        >
          {locale.toUpperCase()}
        </Button>
      </DropdownTrigger>
      <DropdownMenu aria-label="Language selection">
        {locales.map((l) => (
          <DropdownItem
            key={l}
            textValue={languageNames[l]}
            className={l === locale ? "text-primary font-medium" : ""}
            startContent={<span className="text-lg">{languageFlags[l]}</span>}
            onClick={() => handleChangeLanguage(l)}
          >
            {languageNames[l]}
          </DropdownItem>
        ))}
      </DropdownMenu>
    </Dropdown>
  );
}
