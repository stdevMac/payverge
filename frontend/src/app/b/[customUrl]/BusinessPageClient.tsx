"use client";

import React, { useCallback, useState, useEffect, useContext } from "react";
import { useRouter } from "next/navigation";
import ConvertingBusinessLandingPage from "@/components/business-page/ConvertingBusinessLandingPage";
import HeroSkeleton from "@/components/business-page/HeroSkeleton";
import {
  GuestTranslationProvider,
  GuestTranslationContext,
} from "@/i18n/GuestTranslationProvider";
import type { StorefrontLocale } from "@/i18n/localeRegistry";
import { Compass, CloudOff } from "lucide-react";
import { getBusinessByCustomUrl, PublicBusiness } from "@/api/publicBusiness";
import type { MenuCategory } from "@/api/business";
import { formatStorefrontSeoPrice } from "@/lib/storefront/menuPayload";

interface BusinessPageClientProps {
  customUrl: string;
  /**
   * PERF-1: the server component already fetched + normalized this business
   * (page.tsx). Seeding it here lets the storefront first-paint instantly with
   * real content (crawlers + LCP) instead of a skeleton + duplicate cold fetch.
   * Server fetch is default-language; locale-specific copy re-translates at
   * runtime via GuestTranslationProvider (no refetch on locale toggle).
   */
  initialBusiness?: PublicBusiness | null;
  /** Server-known failure reason when initialBusiness is null. */
  initialReason?: "not_found" | "unavailable";
  initialLanguage?: StorefrontLocale;
  initialMessages?: Record<string, unknown>;
  preferInitialLanguage?: boolean;
  /**
   * SEO-0.1: server-normalized menu categories (same payload seeded into the
   * React Query cache by page.tsx). When present, a visually hidden but
   * crawlable menu section is rendered so the menu — the highest-intent
   * keyword content a restaurant has — exists in the SSR HTML even though the
   * visible menu tab only mounts for the active tab client-side.
   */
  initialMenuCategories?: MenuCategory[] | null;
}

export default function BusinessPageClient({
  customUrl,
  initialBusiness = null,
  initialReason,
  initialLanguage: serverInitialLanguage,
  initialMessages,
  preferInitialLanguage = false,
  initialMenuCategories = null,
}: BusinessPageClientProps) {
  const router = useRouter();

  // I18N-1: resolve loading/error copy from the GUEST tier (21 locales) like
  // MenuSkeleton — read the context directly so it works even before a business
  // id mounts a provider. When a provider IS mounted, trust the resolved value:
  // every businessPage.error.* / loadingAria / errors.* key is ported into ALL
  // 21 bundles, so the key is guaranteed present (and the guest t() never returns
  // the raw key on a miss — it returns sentenceCaseLeaf(key), so a
  // `resolved !== key` guard would be ALWAYS true and emit humanized garbage,
  // verified GuestTranslationProvider.tsx:222-250). The English `fallback` is used
  // ONLY when no provider is in scope (guestCtx === undefined).
  const guestCtx = useContext(GuestTranslationContext);
  const tBusiness = useCallback(
    (key: string, fallback: string): string => {
      if (!guestCtx) return fallback;
      return guestCtx.t(`businessPage.${key}`);
    },
    [guestCtx],
  );
  // The active guest locale drives the first-paint fetch language (audit E1 —
  // no source-language flash). Defaults to the business default / en when no
  // guest provider is mounted on the cold-fetch path.
  const locale = guestCtx?.currentLanguage ?? "en";

  // Seed the published/active gate on the server-fetched object so a seeded
  // but unpublished/inactive business is treated as not-found at first paint.
  const seededPublished =
    initialBusiness != null &&
    initialBusiness.page_enabled !== false &&
    initialBusiness.is_active !== false;
  const seededOk = initialBusiness != null && seededPublished;

  const [business, setBusiness] = useState<PublicBusiness | null>(
    seededOk ? initialBusiness : null,
  );
  // Seeded success, a confirmed 404, or an unpublished page skip the skeleton.
  // Transient SSR "unavailable" must still retry on the client (#685).
  const [loading, setLoading] = useState(
    !(
      seededOk ||
      initialReason === "not_found" ||
      (initialBusiness != null && !seededPublished)
    ),
  );
  const [error, setError] = useState<string | null>(
    initialReason === "not_found"
      ? tBusiness("errors.notFound", "Business not found")
      : initialBusiness != null && !seededPublished
        ? tBusiness("errors.pageNotAvailable", "Business page not available")
        : null,
  );
  const [isNotFound, setIsNotFound] = useState(initialReason === "not_found");

  useEffect(() => {
    // PERF-1: when the server already seeded a (published) business OR a
    // confirmed not-found, skip the duplicate client fetch. A transient
    // unavailable seed must refetch so a live venue can recover (#685).
    if (initialBusiness != null || initialReason === "not_found") {
      return;
    }
    const fetchBusiness = async () => {
      try {
        setLoading(true);
        setError(null);
        setIsNotFound(false);

        const businessData = await getBusinessByCustomUrl(customUrl, locale);

        // Check if business page is enabled and business is active
        // The public payload omits these flags for an enabled, active page.
        const pageEnabled = businessData.page_enabled !== false; // Default to true if undefined
        const isActive = businessData.is_active !== false; // Default to true if undefined

        if (!pageEnabled || !isActive) {
          setError(
            tBusiness("errors.pageNotAvailable", "Business page not available"),
          );
          return;
        }

        setBusiness(businessData);
      } catch (err: unknown) {
        console.error("Error fetching business:", err);

        // The shared axiosInstance interceptor rejects with a SANITIZED plain
        // Error (so axios.isAxiosError(err) is always false here). Read the
        // status off the sanitized shape it carries instead, so a real 404
        // shows the permanent "not found" UI rather than a retryable
        // "temporarily unavailable / Try again". (L-1)
        const status =
          (err as { response?: { status?: number }; status?: number })?.response
            ?.status ?? (err as { status?: number })?.status;
        if (status === 404) {
          setIsNotFound(true);
          setError(tBusiness("errors.notFound", "Business not found"));
        } else {
          setError(tBusiness("errors.loadFailed", "Failed to load business"));
        }
      } finally {
        setLoading(false);
      }
    };

    fetchBusiness().catch((err) => console.error("fetchBusiness failed:", err));
    // The active locale participates in the request and the dependencies. This
    // keeps an unseeded storefront from retaining locale-specific API data from
    // the language that happened to be active on mount.
  }, [customUrl, initialBusiness, initialReason, locale, tBusiness]);

  if (loading) {
    return (
      <HeroSkeleton
        label={tBusiness("loadingAria", "Loading business page")}
      />
    );
  }

  if (error || !business) {
    const eyebrow = isNotFound
      ? tBusiness("error.notFoundEyebrow", "Not found")
      : tBusiness("error.unavailableEyebrow", "Unavailable");
    const title = isNotFound
      ? tBusiness("error.notFoundTitle", "We couldn't find that page")
      : tBusiness("error.unavailableTitle", "Business temporarily unavailable");
    const bodyText = isNotFound
      ? tBusiness(
          "error.notFoundBody",
          "We couldn't find a business with that custom URL. It may have been removed, or the URL may be misspelled.",
        )
      : tBusiness(
          "error.unavailableBody",
          "We're having trouble loading this page. Please try again.",
        );

    return (
      <div className="min-h-screen flex flex-col justify-center items-center bg-warm-50 px-6">
        <div className="max-w-lg w-full text-center flex flex-col items-center gap-4">
          <div
            aria-hidden
            className="w-14 h-14 rounded-2xl bg-warm-100 border border-warm-200 flex items-center justify-center"
          >
            {isNotFound ? (
              <Compass className="w-6 h-6 text-ink-500" />
            ) : (
              <CloudOff className="w-6 h-6 text-ink-500" />
            )}
          </div>
          <p className="text-label uppercase text-ink-500">{eyebrow}</p>
          <h1 className="font-title text-display-md text-ink-950">{title}</h1>
          <p className="text-body text-ink-600 max-w-md">{bodyText}</p>
          <div className="flex flex-col sm:flex-row gap-3 justify-center mt-2">
            <button
              onClick={() => router.push("/")}
              className="inline-flex items-center justify-center rounded-full bg-brand px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-brand-dark cursor-pointer"
            >
              {tBusiness("error.notFoundCta", "Go to homepage")}
            </button>
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center justify-center rounded-full border border-ink-200 px-6 py-3 text-sm font-semibold text-ink-800 transition-colors hover:bg-ink-50 cursor-pointer"
            >
              {tBusiness("error.retry", "Try again")}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // I18N-2: honor the advertised ?lang= hreflang alternate on first paint,
  // validated against the guest locale set; else the business default.
  // (Initial language is otherwise determined by FloatingLanguageSelectorBusiness
  // based on localStorage preference or business default language.)
  // LOCALE-2: normalize the ?lang= value case-insensitively so a natural
  // lowercase "es-ar" resolves to the canonical "es-AR" (and agrees with the
  // server generateMetadata / SERP snippet) instead of falling back to default.
  const initialLanguage =
    serverInitialLanguage ?? (business.default_language || "en");

  // SEO-0.1: crawlable menu content. Rendered only from the server-seeded
  // payload (never client-fetched data), visually hidden via sr-only, and
  // aria-hidden so assistive tech ignores the duplicate of the visible menu.
  // Text comes entirely from the business's own menu data (no UI copy).
  const seoCurrency = business.display_currency || business.default_currency;
  const seoMenu =
    initialMenuCategories && initialMenuCategories.length > 0
      ? initialMenuCategories
      : null;

  return (
    <GuestTranslationProvider
      businessId={business.id}
      initialLanguage={initialLanguage as any}
      initialMessages={initialMessages}
      preferInitialLanguage={preferInitialLanguage}
    >
      <ConvertingBusinessLandingPage
        business={business}
        customUrl={customUrl}
        seedLanguage={
          preferInitialLanguage && serverInitialLanguage
            ? serverInitialLanguage
            : business.default_language || "en"
        }
      />
      {seoMenu && (
        <section
          aria-hidden="true"
          className="sr-only"
          data-testid="storefront-seo-menu"
        >
          {seoMenu.map((category, categoryIndex) => (
            <div key={category?.id ?? `seo-category-${categoryIndex}`}>
              {category?.name ? <h2>{category.name}</h2> : null}
              {category?.description ? <p>{category.description}</p> : null}
              <ul>
                {(Array.isArray(category?.items) ? category.items : []).map(
                  (item, itemIndex) => {
                    const price = formatStorefrontSeoPrice(
                      item?.price,
                      seoCurrency,
                    );
                    return (
                      <li
                        key={
                          item?.id ??
                          `seo-item-${categoryIndex}-${itemIndex}`
                        }
                      >
                        {item?.name ? <h3>{item.name}</h3> : null}
                        {item?.description ? <p>{item.description}</p> : null}
                        {price ? <span>{price}</span> : null}
                      </li>
                    );
                  },
                )}
              </ul>
            </div>
          ))}
        </section>
      )}
    </GuestTranslationProvider>
  );
}
