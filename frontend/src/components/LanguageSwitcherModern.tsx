"use client";

import { useTransition } from 'react';
import { Dropdown, DropdownTrigger, DropdownMenu, DropdownItem, Button } from "@nextui-org/react";
import { useLocale, useTranslations } from '@/i18n/TranslationProvider';
import { locales, languageNames, languageFlags, type Locale } from '@/i18n/config';
import { FaGlobe } from "react-icons/fa";

export default function LanguageSwitcherModern() {
  const [isPending, startTransition] = useTransition();
  const locale = useLocale();
  const { setLocale } = useTranslations();

  // Function to change the language
  const handleChangeLanguage = (newLocale: Locale) => {
    if (newLocale === locale) return;
    
    startTransition(() => {
      setLocale(newLocale);
    });
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
          isDisabled={isPending}
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
