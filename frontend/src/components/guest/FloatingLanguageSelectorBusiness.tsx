"use client";

import React, { useState, useEffect, useRef, useCallback } from "react";
import { createPortal } from "react-dom";
import { Globe } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { BusinessLanguage, SupportedLanguage } from "../../api/currency";
import {
  useGuestTranslation,
  GUEST_SUPPORTED_LANGUAGES,
  type GuestLanguageCode,
} from "../../i18n/GuestTranslationProvider";
import {
  buildGuestLangHref,
  guestPathLocaleFromPathname,
  readGuestLocaleCookie,
} from "../../i18n/guestLocaleResolver";
import {
  normalizeGuestLangParam,
  resolveGuestInitialLanguage,
  storefrontLocales,
} from "../../i18n/localeRegistry";

const ALL_STOREFRONT_CODES = storefrontLocales as readonly string[];

interface FloatingLanguageSelectorBusinessProps {
  businessId: number;
  businessLanguages: BusinessLanguage[];
  supportedLanguages: SupportedLanguage[];
  onClose?: () => void;
  /**
   * "floating" — mobile-style fixed pill (default). "inline" — embedded
   * in a parent layout (e.g. the desktop tab bar) with no fixed positioning
   * and no auto-show delay.
   */
  variant?: "floating" | "inline";
  /**
   * Rendered inside the operator's Business Page live preview (#591). The
   * selector still switches the previewed locale, but it must not resolve the
   * language from — or write `?lang=` back into — the host dashboard URL.
   */
  previewMode?: boolean;
}

// Flags were removed: flags != languages, and emojis are banned in the design
// system (see SimpleLanguageSelector). The selector uses ISO-code chips
// instead, matching the sibling guest language picker.

export function FloatingLanguageSelectorBusiness({
  businessId,
  businessLanguages,
  supportedLanguages,
  variant = "floating",
  previewMode = false,
}: FloatingLanguageSelectorBusinessProps) {
  const { t, currentLanguage, setLanguage } = useGuestTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [isChanging, setIsChanging] = useState(false);
  // PG-6: controlled open state so outside-click / Escape reliably close.
  const [isOpen, setIsOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const selectedOptionRef = useRef<HTMLButtonElement | null>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  // Roving tabindex: tabIndex 0 follows the focused option, not only the
  // selected language, so Tab leaves the list from wherever the arrows stopped.
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const focusedIndexRef = useRef(0);
  const [focusedIndex, setFocusedIndex] = useState(0);
  // #595: the dialog is portaled to <body> (see render), so an ancestor's
  // overflow clipping (the sticky tab bar keeps overflow-hidden for #442)
  // cannot cut it off. It is fixed-positioned from the trigger's rect.
  const [menuPos, setMenuPos] = useState<{ top: number; right: number } | null>(
    null,
  );

  const updateMenuPos = useCallback(() => {
    const rect = triggerRef.current?.getBoundingClientRect();
    if (!rect || typeof window === "undefined") return;
    setMenuPos({
      top: rect.bottom + 8,
      right: Math.max(window.innerWidth - rect.right, 8),
    });
  }, []);
  // PG-5: only auto-resolve once; after a user pick (or first apply) do not
  // re-apply sticky ?lang= / browser detection on every render.
  const resolvedOnceRef = useRef(false);

  // Plan 1.11: the pill mounts immediately. The old 1s setTimeout
  // materialization popped the control in with no transition; the entrance is
  // now a CSS keyframe fade/slide (see the scoped <style> at render). The
  // global prefers-reduced-motion rule in globals.css collapses it to an
  // instant mount for reduced-motion users. Visibility no longer depends on
  // the locale-resolve effect, so a remount after a language change (#474)
  // shows the pill right away whether or not ?lang= is set.

  useEffect(() => {
    if (resolvedOnceRef.current) return;
    // In the editor preview the provider is seeded with the storefront's own
    // language; browser/cookie/?lang= resolution belongs to real guests only.
    if (previewMode) return;
    resolvedOnceRef.current = true;

    // Resolve against the full storefront set so a saved/cookie es-AR pick
    // is not discarded when the business only enabled en/es.
    const enabled = ALL_STOREFRONT_CODES;

    // PG-4: honor an explicit ?lang= for the pill + provider when valid.
    // UI chrome can switch even if menu translations are sparse; menu rows
    // fall back es-AR → es on the backend.
    const urlLang =
      normalizeGuestLangParam(searchParams?.get("lang")) ||
      guestPathLocaleFromPathname(pathname ?? "");
    if (urlLang && urlLang in GUEST_SUPPORTED_LANGUAGES) {
      setLanguage(urlLang as GuestLanguageCode);
    } else {
      const browserLanguages =
        typeof navigator !== "undefined"
          ? [navigator.language, ...(navigator.languages ?? [])].filter(Boolean)
          : [];
      const businessDefault =
        businessLanguages.find((lang: BusinessLanguage) => lang.is_default)
          ?.language_code ?? null;

      const resolved = resolveGuestInitialLanguage({
        saved:
          localStorage.getItem(`guest-language-${businessId}`) ??
          readGuestLocaleCookie(),
        browserLanguages,
        enabled,
        businessDefault,
      });

      if (resolved in GUEST_SUPPORTED_LANGUAGES) {
        setLanguage(resolved as GuestLanguageCode);
      }
    }
  }, [
    businessId,
    businessLanguages,
    setLanguage,
    variant,
    searchParams,
    pathname,
    previewMode,
  ]);

  // #595: keep the fixed-positioned dialog glued to the trigger while the
  // page scrolls or resizes (the tab bar is sticky, but pre-stick it moves).
  useEffect(() => {
    if (!isOpen) return;
    updateMenuPos();
    window.addEventListener("resize", updateMenuPos);
    window.addEventListener("scroll", updateMenuPos, true);
    return () => {
      window.removeEventListener("resize", updateMenuPos);
      window.removeEventListener("scroll", updateMenuPos, true);
    };
  }, [isOpen, updateMenuPos]);

  // PG-6: close on outside mousedown. The locale dialog is portaled to
  // <body> (#595), so `rootRef.contains` no longer covers it — check the
  // portal node explicitly. Treat only real menu/listbox/dialog portal nodes
  // as "inside" — never generic NextUI roots (`[data-slot="base"]` appears on
  // storefront cards/buttons site-wide and would block outside-close).
  useEffect(() => {
    if (!isOpen) return;
    const onPointerDown = (event: MouseEvent | TouchEvent) => {
      const target = event.target as Element | null;
      if (!target) return;
      if (rootRef.current?.contains(target)) return;
      if (dialogRef.current?.contains(target)) return;
      if (
        target.closest?.('[role="menu"], [role="listbox"], [role="dialog"]')
      ) {
        return;
      }
      setIsOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("touchstart", onPointerDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("touchstart", onPointerDown);
    };
  }, [isOpen]);

  const dialogLabel = t("languageSelector.changeLanguage") || "Change language";

  // Name the dialog and park focus on the active locale (not a middle item).
  useEffect(() => {
    if (!isOpen) return;
    const focusId = window.requestAnimationFrame(() => {
      selectedOptionRef.current?.focus();
    });
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setIsOpen(false);
      triggerRef.current?.focus();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      window.cancelAnimationFrame(focusId);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [isOpen, currentLanguage]);

  const syncLangToUrl = useCallback(
    (languageCode: string) => {
      // PG-5: update ?lang= to the user pick so a sticky old param cannot
      // re-defeat the selection on remount / soft navigation.
      // #401: keep #menu / #delivery / #reservations on the replaced URL.
      // The preview is embedded in the operator dashboard; rewriting the host
      // route's query would navigate the dashboard out from under the editor.
      if (previewMode) return;
      if (!pathname) return;
      const href = buildGuestLangHref(
        pathname,
        searchParams?.toString() ?? "",
        languageCode,
        typeof window !== "undefined" ? window.location.hash : "",
      );
      router.replace(href, { scroll: false });
    },
    [pathname, router, searchParams, previewMode],
  );

  const handleLanguageChange = (languageCode: string) => {
    if (languageCode === currentLanguage) {
      setIsOpen(false);
      triggerRef.current?.focus();
      return;
    }

    setIsChanging(true);

    if (businessId) {
      localStorage.setItem(`guest-language-${businessId}`, languageCode);
    }

    if (languageCode in GUEST_SUPPORTED_LANGUAGES) {
      setLanguage(languageCode as GuestLanguageCode);
    }

    syncLangToUrl(languageCode);

    window.dispatchEvent(
      new CustomEvent("guestLanguageChange", {
        detail: { language: languageCode, businessId },
      }),
    );

    setIsOpen(false);
    setTimeout(() => {
      setIsChanging(false);
    }, 500);
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

  // PG-14 parity with /t: always offer the full guest storefront locale set
  // so a saved/cookie es-AR pick remains selectable and the pill can show
  // ES-AR instead of collapsing to EN. Business languages still mark default.
  const defaultCodes = new Set(
    businessLanguages
      .filter((lang) => lang.is_default)
      .map((lang) => lang.language_code.toLowerCase()),
  );
  const menuLanguages: BusinessLanguage[] = ALL_STOREFRONT_CODES.map(
    (code) =>
      ({
        language_code: code,
        is_default:
          defaultCodes.has(code.toLowerCase()) ||
          (defaultCodes.size === 0 && code === "en"),
      }) as BusinessLanguage,
  );

  if (menuLanguages.length < 2) {
    return null;
  }

  const focusOptionAt = (index: number) => {
    const count = menuLanguages.length;
    if (count === 0) return;
    const next = ((index % count) + count) % count;
    focusedIndexRef.current = next;
    setFocusedIndex(next);
    optionRefs.current[next]?.focus();
  };

  const onListboxKeyDown = (event: React.KeyboardEvent<HTMLUListElement>) => {
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        focusOptionAt(focusedIndexRef.current + 1);
        break;
      case "ArrowUp":
        event.preventDefault();
        focusOptionAt(focusedIndexRef.current - 1);
        break;
      case "Home":
        event.preventDefault();
        focusOptionAt(0);
        break;
      case "End":
        event.preventDefault();
        focusOptionAt(menuLanguages.length - 1);
        break;
      default:
        break;
    }
  };

  // PG-4: pill tracks the provider language, not a separate local state.
  const pillCode = (currentLanguage || "en").toUpperCase();

  const wrapperClass =
    variant === "inline"
      ? "relative inline-flex"
      : "fixed top-6 right-6 z-50 storefront-lang-pill-in";

  return (
    <div className={wrapperClass} ref={rootRef}>
      {variant === "floating" && (
        <style>{`
          @keyframes storefront-lang-pill-in {
            from { opacity: 0; transform: translateY(-0.5rem); }
            to { opacity: 1; transform: translateY(0); }
          }
          .storefront-lang-pill-in {
            animation: storefront-lang-pill-in 300ms ease-out;
          }
        `}</style>
      )}
      <button
        ref={triggerRef}
        type="button"
        aria-label={dialogLabel}
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        disabled={isChanging}
        onClick={() => {
          // Position before the open render so the portaled dialog never
          // flashes at (0, 0). Seed roving focus on the selected language
          // in the same update so the first open paint has tabIndex 0 there.
          if (!isOpen) {
            updateMenuPos();
            const selectedIndex = menuLanguages.findIndex(
              (lang) => lang.language_code === currentLanguage,
            );
            const next = selectedIndex >= 0 ? selectedIndex : 0;
            focusedIndexRef.current = next;
            setFocusedIndex(next);
          }
          setIsOpen(!isOpen);
        }}
        className="inline-flex min-w-0 items-center gap-1 rounded-full border border-neutral-200 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-neutral-100 disabled:opacity-60"
      >
        <Globe className="h-4 w-4 text-ink-600" strokeWidth={1.75} />
        <span className="font-mono text-xs uppercase tracking-wider">
          {pillCode}
        </span>
      </button>
      {/* #595: portal the dialog to <body>. Inline (desktop) mounts inside the
          sticky tab bar whose overflow-hidden / overflow-x-clip ancestors (kept
          for #442) would clip an in-flow absolute panel to a sliver. A fixed,
          body-level dialog floats over the page for both variants. */}
      {isOpen && typeof document !== "undefined"
        ? createPortal(
            <div
              ref={dialogRef}
              role="dialog"
              aria-label={dialogLabel}
              className="fixed z-50 max-h-[min(24rem,70vh)] w-72 overflow-y-auto rounded-xl border border-warm-200 bg-white p-1 shadow-lg"
              style={{ top: menuPos?.top ?? 0, right: menuPos?.right ?? 0 }}
            >
              <ul
                role="listbox"
                aria-label={
                  t("languageSelector.dropdownAria") || "Language selection"
                }
                onKeyDown={onListboxKeyDown}
              >
                {menuLanguages.map((lang, index) => {
                  const selected = lang.language_code === currentLanguage;
                  return (
                    <li key={lang.language_code} role="none">
                      <button
                        ref={(node) => {
                          optionRefs.current[index] = node;
                          if (selected) selectedOptionRef.current = node;
                        }}
                        type="button"
                        role="option"
                        data-key={lang.language_code}
                        aria-selected={selected}
                        tabIndex={index === focusedIndex ? 0 : -1}
                        onClick={() => handleLanguageChange(lang.language_code)}
                        className={`flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm ${
                          selected
                            ? "bg-warm-50 font-medium text-primary"
                            : "hover:bg-warm-50"
                        }`}
                      >
                        <span className="inline-flex h-7 min-w-[2.25rem] items-center justify-center rounded-md border border-warm-200 bg-warm-50 px-2 font-mono text-[11px] font-semibold uppercase tracking-wider text-ink-700">
                          {lang.language_code}
                        </span>
                        <span className="flex-1">
                          {getLanguageDisplayName(lang.language_code)}
                        </span>
                        {lang.is_default && (
                          <span className="text-xs font-medium text-brand">
                            {t("languageSelector.default")}
                          </span>
                        )}
                      </button>
                    </li>
                  );
                })}
              </ul>
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}
