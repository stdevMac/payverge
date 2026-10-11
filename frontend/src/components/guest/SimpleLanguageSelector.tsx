"use client";

import React, { useState, useEffect, useMemo, useRef } from "react";
import { Modal, ModalContent, ModalHeader, ModalBody } from "@nextui-org/react";
import { Globe, Check } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { getBusinessByTableCode } from "../../api/bills";
import type { BusinessLanguage, SupportedLanguage } from "../../api/currency";
import {
  useGuestTranslation,
  GUEST_SUPPORTED_LANGUAGES,
  type GuestLanguageCode,
} from "../../i18n/GuestTranslationProvider";
import {
  buildGuestLangHref,
  readGuestLocaleCookie,
} from "../../i18n/guestLocaleResolver";
import {
  normalizeGuestLangParam,
  resolveGuestInitialLanguage,
} from "../../i18n/localeRegistry";

interface SimpleLanguageSelectorProps {
  tableCode: string;
}

// Country flags removed: flags ≠ languages, and emojis are banned in the design
// system. The selector uses ISO codes inline.

const ALL_STOREFRONT_CODES = Object.keys(
  GUEST_SUPPORTED_LANGUAGES,
) as GuestLanguageCode[];

export function SimpleLanguageSelector({
  tableCode,
}: SimpleLanguageSelectorProps) {
  const { t, currentLanguage, setLanguage } = useGuestTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [businessLanguages, setBusinessLanguages] = useState<
    BusinessLanguage[]
  >([]);
  const [supportedLanguages, setSupportedLanguages] = useState<
    SupportedLanguage[]
  >([]);
  const [businessId, setBusinessId] = useState<number | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [loading, setLoading] = useState(true);
  // PG-5: GuestTranslationProvider.setLanguage is not useCallback-stable. If it
  // is in the effect deps, every diner pick re-runs loadLanguages, re-reads
  // sticky ?lang=, and re-applies the URL locale — defeating the pick. Resolve
  // the initial language exactly once per tableCode (same pattern as the /b
  // FloatingLanguageSelectorBusiness resolvedOnceRef).
  const resolvedOnceRef = useRef(false);
  const setLanguageRef = useRef(setLanguage);
  setLanguageRef.current = setLanguage;
  const searchParamsRef = useRef(searchParams);
  searchParamsRef.current = searchParams;
  const selectedOptionRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!tableCode) return;

    // New table → allow one initial resolve for that table.
    resolvedOnceRef.current = false;
    let cancelled = false;

    const loadLanguages = async () => {
      try {
        setLoading(true);
        const businessData = await getBusinessByTableCode(tableCode);
        if (cancelled) return;
        const businessLangs = businessData.business_languages || [];
        const supportedLangs = businessData.supported_languages || [];
        const loadedBusinessId = businessData.business?.id;

        setBusinessId(loadedBusinessId);
        setBusinessLanguages(businessLangs);
        setSupportedLanguages(supportedLangs);

        // PG-5: only seed language once. Subsequent effect re-runs (or a late
        // resolve after the diner already picked) must not re-apply sticky ?lang=.
        if (resolvedOnceRef.current) return;
        resolvedOnceRef.current = true;

        // PG-4: prefer explicit ?lang= for first paint agreement with URL.
        const urlLang = normalizeGuestLangParam(
          searchParamsRef.current?.get("lang"),
        );
        if (urlLang && urlLang in GUEST_SUPPORTED_LANGUAGES) {
          setLanguageRef.current(urlLang as GuestLanguageCode);
        } else {
          const browserLanguages =
            typeof navigator !== "undefined"
              ? [navigator.language, ...(navigator.languages ?? [])].filter(
                  Boolean,
                )
              : [];
          const businessDefault =
            businessLangs.find((lang: BusinessLanguage) => lang.is_default)
              ?.language_code ?? null;

          // PG-14: resolve against the full guest storefront set (not only
          // the business's menu languages) so /t matches /b's 21-locale surface.
          const resolved = resolveGuestInitialLanguage({
            saved: loadedBusinessId
              ? localStorage.getItem(`guest-language-${loadedBusinessId}`) ??
                readGuestLocaleCookie()
              : readGuestLocaleCookie(),
            browserLanguages,
            enabled: ALL_STOREFRONT_CODES,
            businessDefault,
          });

          if (resolved in GUEST_SUPPORTED_LANGUAGES) {
            setLanguageRef.current(resolved as GuestLanguageCode);
          }
        }
      } catch (err) {
        console.error("Error loading languages:", err);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void loadLanguages();
    return () => {
      cancelled = true;
    };
    // Intentionally only tableCode: setLanguage is unstable; searchParams are
    // snapshotted for the first resolve only (PG-5).
  }, [tableCode]);

  useEffect(() => {
    if (!showModal) return;
    const id = window.requestAnimationFrame(() => {
      selectedOptionRef.current?.focus();
    });
    return () => window.cancelAnimationFrame(id);
  }, [showModal, currentLanguage]);

  // PG-14: always offer the full guest storefront locale set for UI chrome.
  // Business languages still inform display names when present.
  const localeOptions = useMemo(() => {
    const byCode = new Map(
      businessLanguages.map((l) => [l.language_code.toLowerCase(), l]),
    );
    return ALL_STOREFRONT_CODES.map((code) => {
      const biz = byCode.get(code.toLowerCase());
      return {
        language_code: code,
        is_default: biz?.is_default ?? code === "en",
      };
    });
  }, [businessLanguages]);

  const handleLanguageChange = (languageCode: string) => {
    if (businessId) {
      localStorage.setItem(`guest-language-${businessId}`, languageCode);
    }

    if (languageCode in GUEST_SUPPORTED_LANGUAGES) {
      setLanguage(languageCode as GuestLanguageCode);
    }

    // PG-5: keep URL in sync so sticky ?lang= cannot re-defeat the pick.
    // #401: preserve #menu / #bill hashes on the table route too.
    if (pathname) {
      router.replace(
        buildGuestLangHref(
          pathname,
          searchParams?.toString() ?? "",
          languageCode,
          typeof window !== "undefined" ? window.location.hash : "",
        ),
        { scroll: false },
      );
    }

    setTimeout(() => {
      setShowModal(false);
    }, 300);
  };

  const getLanguageDisplayName = (languageCode: string): string => {
    const supportedLang = supportedLanguages.find(
      (lang) => lang.code === languageCode,
    );
    if (supportedLang) {
      return supportedLang.native_name || supportedLang.name;
    }
    const guestMeta =
      languageCode in GUEST_SUPPORTED_LANGUAGES
        ? GUEST_SUPPORTED_LANGUAGES[languageCode as GuestLanguageCode]
        : null;
    if (guestMeta) return guestMeta.name;
    return languageCode.toUpperCase();
  };

  if (loading) {
    return null;
  }

  // PG-4: pill tracks provider language.
  const pillCode = (currentLanguage || "en").toUpperCase();

  return (
    <>
      <button
        type="button"
        onClick={() => setShowModal(true)}
        className="group flex flex-1 basis-0 flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2.5 min-h-12 text-[10px] font-semibold uppercase tracking-[0.18em] text-ink-500 transition-colors hover:bg-ink-50 hover:text-ink-900"
        aria-label={t("languageSelector.changeLanguage") || "Change language"}
      >
        <span className="flex h-6 w-6 items-center justify-center">
          <Globe className="h-5 w-5" strokeWidth={1.75} />
        </span>
        <span className="font-mono leading-none">{pillCode}</span>
      </button>

      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        size="sm"
        placement="center"
        aria-label={t("languageSelector.changeLanguage") || "Change language"}
        classNames={{
          base: "m-4",
          header: "border-b border-gray-100",
          body: "py-4",
        }}
      >
        <ModalContent>
          <ModalHeader>
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-100 rounded-xl flex items-center justify-center">
                <Globe className="w-5 h-5 text-gray-700" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900">
                {t("languageSelector.title")}
              </h3>
            </div>
          </ModalHeader>
          <ModalBody>
            <div
              className="space-y-2 max-h-[60vh] overflow-y-auto"
              role="listbox"
              aria-label={t("languageSelector.title") || "Language"}
            >
              {localeOptions.map((lang) => (
                <button
                  key={lang.language_code}
                  ref={
                    lang.language_code === currentLanguage
                      ? selectedOptionRef
                      : undefined
                  }
                  type="button"
                  role="option"
                  aria-selected={lang.language_code === currentLanguage}
                  onClick={() => handleLanguageChange(lang.language_code)}
                  className={`flex w-full items-center justify-between gap-3 rounded-xl border px-3 py-3 transition-colors ${
                    lang.language_code === currentLanguage
                      ? "border-ink-900 bg-warm-50"
                      : "border-warm-200 bg-white hover:border-ink-300"
                  }`}
                >
                  <div className="flex items-center gap-3">
                    <span className="inline-flex h-9 min-w-[2.5rem] items-center justify-center rounded-lg border border-warm-200 bg-warm-50 px-2 font-mono text-xs font-semibold uppercase tracking-wider text-ink-700">
                      {lang.language_code}
                    </span>
                    <div className="text-left">
                      <p className="text-sm font-semibold text-ink-900">
                        {getLanguageDisplayName(lang.language_code)}
                      </p>
                      {lang.is_default && (
                        <p className="text-xs text-ink-500">
                          {t("languageSelector.default")}
                        </p>
                      )}
                    </div>
                  </div>
                  {lang.language_code === currentLanguage && (
                    <Check className="h-5 w-5 text-ink-900" strokeWidth={2} />
                  )}
                </button>
              ))}
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>
    </>
  );
}
