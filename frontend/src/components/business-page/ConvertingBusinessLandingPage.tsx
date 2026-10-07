"use client";

import React, { useMemo, useEffect, useCallback, useState, useRef } from "react";
import dynamic from "next/dynamic";
import { useQuery } from "@tanstack/react-query";
import { PublicBusiness, getBusinessByCustomUrl } from "@/api/publicBusiness";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import {
  getPatternSvgDef,
  isStorefrontPattern,
  resolveStorefrontFontClass,
} from "@/lib/storefront/theme";
import { FloatingLanguageSelectorBusiness } from "@/components/guest/FloatingLanguageSelectorBusiness";
import { shouldShowAiWaiter } from "@/app/t/[tableCode]/menu/aiGate";
import PublicMenuDisplay from "./PublicMenuDisplay";
import BusinessHeroSection from "./BusinessHeroSection";
import BusinessTabNavigation from "./BusinessTabNavigation";
import BusinessAboutTab from "./BusinessAboutTab";
import BusinessContactTab from "./BusinessContactTab";
import BusinessDeliveryTab from "./BusinessDeliveryTab";
import BusinessReservationsTab from "./BusinessReservationsTab";
import BusinessFooter from "./BusinessFooter";
import StorefrontAnnouncementBar from "./StorefrontAnnouncementBar";
import StorefrontThemeStyle from "./StorefrontThemeStyle";
import type { GuestDeliveryQuoteContext } from "@/components/delivery/GuestDeliveryOrder";
import { useHashTabs } from "@/hooks/useHashTabs";
import {
  shouldPollStorefrontMenu,
  useBusinessPageData,
} from "@/hooks/useBusinessPageData";
import { useFulfillmentContext } from "@/hooks/useFulfillmentContext";
import { useBusinessOpenStatus } from "@/hooks/useBusinessOpenStatus";
import { DEFAULT_DESIGN_SETTINGS } from "./designClasses";
import { resolveStorefrontDisplayBusiness } from "./storefrontDisplayBusiness";
import { findUpcomingException } from "./storefrontAnnouncement";
import {
  resolvePrimaryStorefrontCta,
  STOREFRONT_CTA_FALLBACKS,
  STOREFRONT_CTA_LABEL_KEYS,
} from "./storefrontCtas";

const AiWaiter = dynamic(
  () =>
    import("@/components/guest/AiWaiter").then((mod) => ({
      default: mod.AiWaiter,
    })),
  { ssr: false, loading: () => null },
);

interface ConvertingBusinessLandingPageProps {
  business: PublicBusiness;
  customUrl: string;
  /** Locale of the server-seeded `business` payload (may be Spanish SSR). */
  seedLanguage?: string;
  /**
   * Embedded render inside the operator's Business Page editor (#591). The
   * SAME component tree paints the preview — there is no second storefront
   * implementation to drift. Preview mode only strips the things an embedded
   * copy must not do to its host document:
   *   - no `#hash` tab routing / URL rewrites (`useHashTabs syncLocation`),
   *   - no `id="main-content"` / `id="tab-content"` duplicates and no
   *     document-level scrollIntoView + focus moves,
   *   - no `?language=` refetch (the draft payload IS the source of truth,
   *     and the localized copy would clobber unsaved edits),
   *   - no AI Waiter session (it would open a real assistant session and
   *     emit analytics from the dashboard).
   */
  previewMode?: boolean;
}

export default function ConvertingBusinessLandingPage({
  business,
  customUrl,
  seedLanguage,
  previewMode = false,
}: ConvertingBusinessLandingPageProps) {
  const { t, currentLanguage, setBusinessId } = useGuestTranslation();

  const designSettings = useMemo(() => ({
    ...DEFAULT_DESIGN_SETTINGS,
    ...(business.design_settings || {}),
  }), [business.design_settings]);

  useEffect(() => {
    setBusinessId(business.id);
  }, [business.id, setBusinessId]);

  const [storefrontTab, setStorefrontTab] = useState<string | null>(() => {
    if (typeof window === "undefined") return null;
    if (previewMode) return "about";
    const hash = window.location.hash.replace(/^#/, "");
    const queryTab = new URLSearchParams(window.location.search).get("tab");
    return hash || queryTab || "about";
  });
  // Preview passes 0 so the operator's own guest delivery context (same
  // business, same browser) never leaks into the editor preview.
  const { context: fulfillmentContext, setContext: setFulfillmentContext, clearContext } =
    useFulfillmentContext(previewMode ? 0 : business.id);

  const {
    menu,
    offers,
    bundles,
    itemOrderability,
    menuSnapshotAuthoritative,
    menuLoading,
    googleRating,
    deliveryEnabled,
    deliveryPartnerLinks,
    deliverySettings,
    deliverySettingsLoading,
    reservationsEnabled,
    reservationPartnerLinks,
    reservationSettingsLoading,
    featureTabsReady,
  } = useBusinessPageData(
    business.id,
    customUrl,
    currentLanguage,
    business.google_reviews_enabled,
    business.google_place_id,
    {
      pollMenu: shouldPollStorefrontMenu(
        storefrontTab,
        Boolean(fulfillmentContext),
      ),
    },
  );

  // Re-fetch the public business for every active locale, including the
  // business default. A Spanish SSR seed must not be reused when the guest
  // switches back to English (`wantsTranslatedContent` used to go false and
  // pin the stale Spanish hero/about/gallery copy).
  const defaultLanguage = business.default_language || "en";
  const contentSeedLanguage = seedLanguage || defaultLanguage;
  const { data: translatedBusiness } = useQuery({
    queryKey: ["public-business-translated", customUrl, currentLanguage],
    queryFn: () => getBusinessByCustomUrl(customUrl, currentLanguage),
    enabled: !previewMode && !!customUrl && !!currentLanguage,
    staleTime: 10 * 60 * 1000,
  });
  // Preview: the draft payload is the only truth. Neither the saved server
  // copy nor the "wait for the localized fetch" blanking may override the
  // operator's unsaved edits.
  const displayBusiness = previewMode
    ? business
    : resolveStorefrontDisplayBusiness({
        seed: business,
        localized: translatedBusiness,
        currentLanguage,
        seedLanguage: contentSeedLanguage,
      });

  const operatingHours = useMemo(() => business.operating_hours || [], [business.operating_hours]);
  const operatingExceptions = useMemo(
    () => business.operating_exceptions || [],
    [business.operating_exceptions],
  );
  const businessTimezone = business.timezone || (business as any).timezone;
  const { isOpen: isBusinessOpen, businessDate } = useBusinessOpenStatus(
    operatingHours,
    businessTimezone,
    operatingExceptions,
  );
  const upcomingException = useMemo(
    () => findUpcomingException(operatingExceptions, businessTimezone),
    [operatingExceptions, businessTimezone],
  );
  // Per-feature visibility only: inactive features are dropped here so the
  // SpecialFeaturesEditor "Visible" toggle is meaningful. SECTION visibility is
  // owned by BusinessAboutTab's show_special_features gate (PARITY-3).
  const specialFeatures = displayBusiness.special_features?.filter((f) => f.is_active) || [];
  // D2: read the gallery from displayBusiness (the ?language= refetch) so
  // translated captions reach alt text; falls back to the seed payload until
  // the translated copy loads — same pattern as special_features above.
  const galleryImages = useMemo(() => {
    try {
      return Array.isArray(displayBusiness.gallery_images)
        ? displayBusiness.gallery_images
        : [];
    } catch {
      return [];
    }
  }, [displayBusiness.gallery_images]);

  const hasDeliveryTab = deliveryEnabled || deliveryPartnerLinks.length > 0;
  const hasReservationsTab = reservationsEnabled || reservationPartnerLinks.length > 0;
  const showDeliveryTab = hasDeliveryTab || deliverySettingsLoading;
  const showReservationsTab = hasReservationsTab || reservationSettingsLoading;

  const validTabs = useMemo(() => {
    const tabs = ["about", "menu"];
    if (showDeliveryTab) tabs.push("delivery");
    if (showReservationsTab) tabs.push("reservations");
    tabs.push("contact");
    return tabs;
  }, [showDeliveryTab, showReservationsTab]);

  const { activeTab, changeTab, locationReady } = useHashTabs({
    validTabs,
    defaultTab: "about",
    tabsReady: featureTabsReady,
    syncLocation: !previewMode,
  });
  const visibleTab = locationReady ? activeTab : null;

  useEffect(() => {
    if (visibleTab) setStorefrontTab(visibleTab);
  }, [visibleTab]);
  const pendingFocusTabRef = useRef<string | null>(null);

  const hiddenPanel = (key: string) => (
    <div
      key={key}
      role="tabpanel"
      id={`tabpanel-${key}`}
      aria-labelledby={`tab-${key}`}
      hidden
      tabIndex={-1}
    />
  );

  const handleDeliveryQuoteReady = (context: GuestDeliveryQuoteContext) => {
    setFulfillmentContext({
      ...context,
      business_id: business.id,
      custom_url: customUrl,
    });
    changeTab("menu");
  };

  // Switch to the menu tab and bring the content into view. Shared by the hero
  // "View Menu" CTA and the reservation confirmation "Check Out Menu" CTA.
  const goToMenu = useCallback(() => {
    pendingFocusTabRef.current = "menu";
    changeTab("menu");
  }, [changeTab]);

  useEffect(() => {
    if (pendingFocusTabRef.current !== activeTab) return;
    pendingFocusTabRef.current = null;
    // Preview is aria-hidden inside the dashboard: scrolling the operator's
    // document or stealing focus into a decorative subtree is never right.
    if (previewMode) return;
    const reduceMotion =
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    document.getElementById("tab-content")?.scrollIntoView({
      behavior: reduceMotion ? "auto" : "smooth",
      block: "start",
    });
    const panel = document.getElementById(`tabpanel-${activeTab}`);
    const tab = document.getElementById(`tab-${activeTab}`);
    const target = panel ?? tab;
    if (target instanceof HTMLElement) {
      if (!target.hasAttribute("tabindex")) target.tabIndex = -1;
      target.focus({ preventScroll: true });
    }
  }, [activeTab, previewMode]);

  const goToDelivery = useCallback(() => {
    pendingFocusTabRef.current = "delivery";
    changeTab("delivery");
  }, [changeTab]);

  const goToReservations = useCallback(() => {
    pendingFocusTabRef.current = "reservations";
    changeTab("reservations");
  }, [changeTab]);

  const [addressEditorToken, setAddressEditorToken] = useState(0);
  const handleEditFulfillment = useCallback(() => {
    setAddressEditorToken((n) => n + 1);
    goToDelivery();
  }, [goToDelivery]);

  const fontFamilyClass = resolveStorefrontFontClass(designSettings.font_family);
  const patternId = isStorefrontPattern(designSettings.background_pattern)
    ? designSettings.background_pattern
    : null;
  const patternSvg = patternId ? getPatternSvgDef(patternId) : null;

  const primaryCtaKind = resolvePrimaryStorefrontCta({
    hasReservations: hasReservationsTab,
    hasDelivery: hasDeliveryTab,
  });
  const primaryCtaAction =
    primaryCtaKind === "reservations"
      ? goToReservations
      : primaryCtaKind === "delivery"
        ? goToDelivery
        : goToMenu;
  const primaryCtaLabel =
    t(STOREFRONT_CTA_LABEL_KEYS[primaryCtaKind]) ||
    STOREFRONT_CTA_FALLBACKS[primaryCtaKind];
  const stickyPrimaryCta =
    primaryCtaKind === activeTab
      ? null
      : { label: primaryCtaLabel, onClick: primaryCtaAction };

  const languageSelectorEnabled = Boolean(
    business.business_languages &&
      business.business_languages.length > 1 &&
      business.supported_languages,
  );

  const rootClassName = `storefront-theme ${previewMode ? "min-h-full" : "min-h-screen"} relative transition-colors duration-300 bg-warm-50 text-ink-950 ${fontFamilyClass}`;

  const storefrontBody = (
    <>
      <StorefrontThemeStyle designSettings={designSettings} />

      {/* Live page pins the pattern to the viewport; the embedded preview must
          pin it to its own frame instead of the dashboard viewport. */}
      {patternSvg && patternId && (
        <div className={`${previewMode ? "absolute" : "fixed"} inset-0 z-0 pointer-events-none opacity-[var(--pattern-opacity)]`} style={{ "--pattern-opacity": designSettings.pattern_opacity || 0.1 } as React.CSSProperties}>
          <svg width="100%" height="100%" className="text-ink-900">
            <defs dangerouslySetInnerHTML={{ __html: patternSvg }} />
            <rect width="100%" height="100%" fill={`url(#${patternId})`} />
          </svg>
        </div>
      )}

      <StorefrontAnnouncementBar
        businessId={business.id}
        exception={upcomingException}
        t={t}
      />

      <BusinessHeroSection
        business={displayBusiness}
        designSettings={designSettings}
        googleRating={googleRating}
        operatingHours={operatingHours}
        onViewMenu={goToMenu}
        onViewReservations={goToReservations}
        onOrderDelivery={goToDelivery}
        hasDelivery={hasDeliveryTab}
        hasReservations={hasReservationsTab}
        isOpenNow={isBusinessOpen}
        openStatusLabel={isBusinessOpen ? t("businessPage.openNow") : t("businessPage.closed")}
      />

      <BusinessTabNavigation
        activeTab={activeTab}
        onChangeTab={changeTab}
        hasDeliveryTab={showDeliveryTab}
        hasReservationsTab={showReservationsTab}
        designSettings={designSettings}
        t={t}
        businessName={displayBusiness.name}
        logoUrl={displayBusiness.logo}
        primaryCta={stickyPrimaryCta}
        rightSlot={
          languageSelectorEnabled ? (
            <FloatingLanguageSelectorBusiness
              variant="inline"
              businessId={business.id}
              businessLanguages={business.business_languages!}
              supportedLanguages={business.supported_languages!}
              previewMode={previewMode}
            />
          ) : null
        }
      />

      {/* Tab Content */}
      <div id={previewMode ? undefined : "tab-content"}>
        {visibleTab === "about" ? (
          <BusinessAboutTab
            business={displayBusiness}
            customUrl={customUrl}
            designSettings={designSettings}
            specialFeatures={specialFeatures}
            galleryImages={galleryImages}
            t={t}
            onViewMenu={goToMenu}
          />
        ) : (
          hiddenPanel("about")
        )}

        {visibleTab === "menu" ? (
          <div role="tabpanel" id="tabpanel-menu" aria-labelledby="tab-menu" tabIndex={-1}>
            <PublicMenuDisplay
              customUrl={customUrl}
              businessId={business.id}
              businessName={business.name}
              categories={menu}
              offers={offers}
              bundles={bundles}
              itemOrderability={itemOrderability}
              menuSnapshotAuthoritative={menuSnapshotAuthoritative}
              loading={menuLoading}
              isOpen={isBusinessOpen}
              designSettings={designSettings}
              fulfillmentContext={fulfillmentContext}
              onEditFulfillment={handleEditFulfillment}
              onClearFulfillment={clearContext}
              deliveryAvailable={deliveryEnabled}
              businessCurrencies={{
                default_currency: business.default_currency || "USD",
                display_currency: business.display_currency || "USD",
              }}
              taxRate={business.tax_rate}
              serviceFeeRate={business.service_fee_rate}
            />
          </div>
        ) : (
          hiddenPanel("menu")
        )}

        {visibleTab === "delivery" ? (
          <BusinessDeliveryTab
            businessId={business.id}
            businessName={business.name}
            customUrl={customUrl}
            deliveryEnabled={deliveryEnabled}
            deliverySettings={deliverySettings}
            deliveryLoading={deliverySettingsLoading}
            deliveryPartnerLinks={deliveryPartnerLinks}
            designSettings={designSettings}
            onQuoteReady={handleDeliveryQuoteReady}
            currency={business.default_currency || "USD"}
            defaultCountry={business.address?.country || ""}
            isBusinessOpen={isBusinessOpen}
            fulfillmentContext={fulfillmentContext}
            openAddressEditorToken={addressEditorToken}
            t={t}
          />
        ) : (
          hiddenPanel("delivery")
        )}

        {visibleTab === "reservations" ? (
          <BusinessReservationsTab
            customUrl={customUrl}
            businessName={business.name}
            timezone={business.timezone}
            reservationsEnabled={reservationsEnabled}
            reservationPartnerLinks={reservationPartnerLinks}
            reservationLoading={reservationSettingsLoading}
            designSettings={designSettings}
            t={t}
            onViewMenu={goToMenu}
          />
        ) : (
          hiddenPanel("reservations")
        )}

        {visibleTab === "contact" ? (
          <BusinessContactTab
            business={displayBusiness}
            designSettings={designSettings}
            operatingHours={operatingHours}
            t={t}
          />
        ) : (
          hiddenPanel("contact")
        )}
      </div>

      <BusinessFooter
        business={displayBusiness}
        operatingHours={operatingHours}
        businessDate={businessDate}
        designSettings={designSettings}
        t={t}
      />

      {/* Floating Language Selector — mobile only; desktop renders inline in
          the tab bar. The preview already renders the inline copy in its tab
          bar; a second `position: fixed` pill would escape the device frame. */}
      {languageSelectorEnabled && !previewMode && (
        <div className="md:hidden">
          <FloatingLanguageSelectorBusiness
            businessId={business.id}
            businessLanguages={business.business_languages!}
            supportedLanguages={business.supported_languages!}
          />
        </div>
      )}

      {/* AI Waiter Concierge — never in preview: mounting it opens a real
          assistant session and emits assistant analytics from the dashboard.
          Gate on backend ai_available (plan + toggle), not leftover settings. */}
      {!previewMode && shouldShowAiWaiter(business) && (
        <AiWaiter
          businessId={business.id}
          businessName={business.name}
          aiName={business.ai_settings?.ai_name}
          waiterMode={business.ai_waiter_mode}
          menuData={menu}
          bundles={bundles}
          language={currentLanguage}
          onAddToCart={() => {}}
          mode="concierge"
        />
      )}
    </>
  );

  return previewMode ? (
    <div data-storefront-preview="true" className={rootClassName}>
      {storefrontBody}
    </div>
  ) : (
    <main id="main-content" tabIndex={-1} className={rootClassName}>
      {storefrontBody}
    </main>
  );
}
