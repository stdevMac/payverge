"use client";

import { getPublicConfig } from "@/config/publicConfig";
import { useState } from "react";
import {
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
  Button,
} from "@nextui-org/react";
import { Globe } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
  persistOperatorLocaleChoice,
} from "@/i18n/SimpleTranslationProvider";
import {
  locales,
  languageNames,
  languageFlags,
  type Locale,
} from "@/i18n/config";
import { getLocaleCodeLabel } from "@/i18n/localeRegistry";
import { useUserStore } from "@/store/useUserStore";

function localeFlag(locale: Locale): string {
  return languageFlags[locale] ?? "🌐";
}

// Never render an empty pill/aria-label. The provider always supplies a known
// locale at runtime (it defaults to `defaultLocale` even pre-hydration), so
// the fallbacks here are defensive depth for a registry miss — a locale code
// present in provider state that `languageNames` doesn't cover — or a broken
// caller. Order: registered display name → upper-cased raw code → "EN"
// (matching the guest SimpleLanguageSelector's terminal default).
function localeLabel(locale: Locale | string | null | undefined): string {
  // Header chrome at ~1024px cannot fit "Español (Argentina)" without clipping
  // the closing paren (#668). Dropdown items still use the full native name.
  if (locale === "es-AR") return "Español (AR)";
  const name = (languageNames as Partial<Record<string, string>>)[locale ?? ""];
  return name || (locale ? locale.toUpperCase() : "") || "EN";
}

export default function SimpleLanguageSwitcher({
  compact = false,
}: {
  /** Compact Globe + ISO-code trigger for tight spots like the dashboard
      sidebar footer, where the full "English" pill overflows the rail. */
  compact?: boolean;
} = {}) {
  const { locale, setLocale } = useSimpleLocale();
  const [isChanging, setIsChanging] = useState(false);
  const t = (key: string, fallback: string): string => {
    const v = getTranslation(`languageSwitcher.${key}`, locale);
    return typeof v === "string" && v !== `languageSwitcher.${key}` ? v : fallback;
  };

  // Use selector to minimize re-renders
  const user = useUserStore((state) => state.user);

  const handleChangeLanguage = async (newLocale: Locale) => {
    if (newLocale === locale) {
      // #617: re-picking the language already on screen is still an explicit
      // choice, and on the operator dashboard it is the ONLY record that the
      // operator wants this locale — SimpleTranslationProvider never writes
      // `locale` on operator app paths, so an untouched store reads as "no
      // opinion" and HybridAuthProvider seeds the account's saved language
      // over it at sign-in. Persist and stop: no re-render, no URL rewrite,
      // and no backend write (the pick is a session/terminal choice, not a
      // change to this account's stored preference).
      persistOperatorLocaleChoice(newLocale);
      return;
    }

    setIsChanging(true);
    // setLocale persists to the canonical `locale` key SimpleTranslationProvider
    // reads. The old redundant localStorage["language"] write only fed the
    // removed useLanguage system (OP-2/LOCALE-6) — dropped so a single key
    // governs the rendered operator language.
    setLocale(newLocale);

    // Sync with backend if user is authenticated
    // httpOnly cookies are sent automatically via credentials: "include"
    if (user) {
      try {
        const apiUrl = getPublicConfig().apiUrl;

        await fetch(`${apiUrl}/inside/set_language`, {
          method: "PUT",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            address: user.address || "",
            language: newLocale,
          }),
        });
      } catch (error) {
        console.error("Failed to sync language preference:", error);
      }
    }

    // Reset the changing state after a brief moment
    setTimeout(() => {
      setIsChanging(false);
    }, 500);
  };

  return (
    <Dropdown placement={compact ? "top-end" : "bottom-end"}>
      <DropdownTrigger aria-label={t("changeLanguage", "Change language")}>
        {compact ? (
          <Button
            variant="light"
            size="sm"
            className="h-8 min-w-0 gap-1.5 overflow-visible rounded-lg px-2 text-ink-400 hover:bg-warm-100 hover:text-ink-700"
            aria-label={`${t("changeLanguage", "Change language")} (${localeLabel(locale)})`}
            isDisabled={isChanging}
          >
            <Globe className="h-4 w-4 flex-shrink-0" strokeWidth={1.75} aria-hidden="true" />
            <span className="text-xs font-medium text-ink-600 shrink-0">
              {getLocaleCodeLabel(locale)}
            </span>
          </Button>
        ) : (
          <Button
            variant="flat"
            size="sm"
            className="shrink-0 overflow-visible whitespace-nowrap px-2.5 font-medium rounded-full border border-neutral-200 hover:bg-neutral-100 transition-colors"
            aria-label={`${t("changeLanguage", "Change language")} (${localeLabel(locale)})`}
            isDisabled={isChanging}
          >
            <span className="text-lg leading-none" aria-hidden="true">
              {localeFlag(locale)}
            </span>
            <span className="ml-1 text-sm font-medium shrink-0">
              {localeLabel(locale)}
            </span>
          </Button>
        )}
      </DropdownTrigger>
      <DropdownMenu aria-label={t("languageSelection", "Language selection")} variant="flat">
        {locales.map((l) => (
          <DropdownItem
            key={l}
            textValue={languageNames[l]}
            className={l === locale ? "text-primary font-medium" : ""}
            startContent={
              <span className="text-lg leading-none" aria-hidden="true">
                {localeFlag(l)}
              </span>
            }
            onPress={() => handleChangeLanguage(l)}
          >
            {languageNames[l]}
          </DropdownItem>
        ))}
      </DropdownMenu>
    </Dropdown>
  );
}
