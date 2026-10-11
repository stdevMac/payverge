"use client";

import React, { useState, useEffect, useMemo, useContext, useCallback } from "react";
import NextImage from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import type { PublicBusiness } from "@/api/publicBusiness";
import {
  getPatternDataUri,
  isStorefrontPattern,
  sanitizeCssColor,
} from "@/lib/storefront/theme";
import { Button, Image as NextUIImage } from "@nextui-org/react";
import { parseBannerImages } from "@/utils/businessDataParsers";
import { Phone, Star, ExternalLink, ChevronRight, Pause, Play, Calendar, Navigation } from "lucide-react";
import {
  getRadiusClass,
  getShadowClass,
  getHeroTreatment,
  type HeroLayout,
} from "./designClasses";
import { Z_DROPDOWN, zStyle } from "./designLayers";
import { GuestTranslationContext } from "@/i18n/GuestTranslationProvider";
import {
  resolvePrimaryStorefrontCta,
  STOREFRONT_CTA_FALLBACKS,
  STOREFRONT_CTA_LABEL_KEYS,
} from "./storefrontCtas";

interface BusinessHeroSectionProps {
  business: PublicBusiness;
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
    shadow_intensity?: string;
    background_pattern?: string;
    pattern_opacity?: number;
    hero_layout?: HeroLayout;
    header_style?: string;
  };
  googleRating: number | null;
  operatingHours: any[];
  onViewMenu: () => void;
  isOpenNow?: boolean;
  openStatusLabel: string;
  /** Feature flags driving the primary CTA (plan 3.1). */
  hasDelivery?: boolean;
  hasReservations?: boolean;
  onViewReservations?: () => void;
  onOrderDelivery?: () => void;
}

export default function BusinessHeroSection({
  business,
  designSettings,
  googleRating,
  operatingHours,
  onViewMenu,
  isOpenNow = false,
  openStatusLabel,
  hasDelivery = false,
  hasReservations = false,
  onViewReservations,
  onOrderDelivery,
}: BusinessHeroSectionProps) {
  const bannerImages = useMemo(
    () => parseBannerImages(business.banner_images),
    [business.banner_images],
  );
  const [currentBannerIndex, setCurrentBannerIndex] = useState(0);
  const [failedBanners, setFailedBanners] = useState<Set<number>>(new Set());
  const [loadedBanners, setLoadedBanners] = useState<Set<number>>(new Set());
  const [logoError, setLogoError] = useState(false);
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);
  const [motionPreferenceReady, setMotionPreferenceReady] = useState(false);
  const [bannerPaused, setBannerPaused] = useState(false);
  const [bannerHoverPause, setBannerHoverPause] = useState(false);
  const [bannerFocusPause, setBannerFocusPause] = useState(false);

  const handleBannerError = useCallback((index: number) => {
    setFailedBanners((prev) => {
      if (prev.has(index)) return prev;
      const next = new Set(prev);
      next.add(index);
      return next;
    });
  }, []);

  const handleBannerLoaded = useCallback((index: number) => {
    setLoadedBanners((prev) => {
      if (prev.has(index)) return prev;
      const next = new Set(prev);
      next.add(index);
      return next;
    });
  }, []);

  // PG-15.4: Next/Image optimizer can hang (complete=false, naturalWidth=0)
  // without ever firing onError for a dead upstream CDN URL. After this
  // timeout, treat the active slide as failed so rotation never dwells blank.
  // Cancelled when onLoad reports a non-zero natural size.
  const BANNER_LOAD_TIMEOUT_MS = 8000;
  useEffect(() => {
    if (bannerImages.length === 0) return;
    if (failedBanners.has(currentBannerIndex)) return;
    if (loadedBanners.has(currentBannerIndex)) return;
    const timer = window.setTimeout(() => {
      handleBannerError(currentBannerIndex);
    }, BANNER_LOAD_TIMEOUT_MS);
    return () => window.clearTimeout(timer);
  }, [
    bannerImages,
    currentBannerIndex,
    failedBanners,
    loadedBanners,
    handleBannerError,
  ]);

  const loadableBanners = useMemo(
    () => bannerImages.filter((_, i) => !failedBanners.has(i)),
    [bannerImages, failedBanners],
  );

  // Original indices of the loadable banners — the indicator dots map back to
  // these so clicking a dot lands on the right slide even after failures.
  const loadableIndices = useMemo(
    () =>
      bannerImages
        .map((_, i) => i)
        .filter((i) => !failedBanners.has(i)),
    [bannerImages, failedBanners],
  );

  // If the currently-shown banner just failed, snap to the next loadable one so
  // the hero never displays a blank slide (including the first banner failing).
  useEffect(() => {
    if (
      failedBanners.has(currentBannerIndex) &&
      loadableBanners.length > 0
    ) {
      const total = bannerImages.length;
      for (let step = 1; step <= total; step++) {
        const candidate = (currentBannerIndex + step) % total;
        if (!failedBanners.has(candidate)) {
          setCurrentBannerIndex(candidate);
          break;
        }
      }
    }
  }, [failedBanners, currentBannerIndex, loadableBanners.length, bannerImages.length]);

  // Use the guest-i18n context directly so the hero CTAs render in the
  // visitor's selected language. Defensive fallback keeps tests rendering
  // outside the provider safe.
  const guestCtx = useContext(GuestTranslationContext);
  const t = (key: string, params?: Record<string, string | number>): string => {
    const result = guestCtx?.t?.(key, params);
    return typeof result === "string" ? result : "";
  };

  const radiusClass = getRadiusClass(designSettings.corner_radius);
  const shadowClass = getShadowClass(designSettings.shadow_intensity);
  const heroLayout: HeroLayout = designSettings.hero_layout || "centered";
  const treatment = getHeroTreatment(designSettings.header_style);
  const safePrimaryColor = sanitizeCssColor(designSettings.primary_color);
  const safeSecondaryColor = sanitizeCssColor(designSettings.secondary_color);

  // Designed no-banner fallback (plan 1.5): a saturated diagonal gradient
  // between the merchant's primary/secondary with the shared storefront
  // pattern overlaid in white (data-URI background, so no SVG <pattern> id
  // collisions with the page-level pattern layer) plus a readability floor.
  const fallbackPatternImage = useMemo(() => {
    if (!isStorefrontPattern(designSettings.background_pattern)) {
      return undefined;
    }
    // eslint-disable-next-line no-restricted-syntax -- white pattern tint over the brand gradient (functional overlay, not a palette color)
    return getPatternDataUri(designSettings.background_pattern, "#ffffff");
  }, [designSettings.background_pattern]);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setPrefersReducedMotion(mq.matches);
    update();
    setMotionPreferenceReady(true);
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);

  // WCAG 2.2.2: auto-rotation pauses on hover, on keyboard focus anywhere
  // inside the hero, and via the explicit pause/play toggle.
  const rotationPaused = bannerPaused || bannerHoverPause || bannerFocusPause;

  const handleHeroBlur = useCallback((e: React.FocusEvent<HTMLElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      setBannerFocusPause(false);
    }
  }, []);

  useEffect(() => {
    if (!motionPreferenceReady) return;
    if (prefersReducedMotion || rotationPaused || loadableBanners.length <= 1) {
      return;
    }
    const interval = setInterval(() => {
      // Advance to the next banner that has not failed to load. Cycling over
      // the raw bannerImages length would dwell on a failed (null) slide for
      // the full interval; skip failed indices so rotation never lands blank.
      setCurrentBannerIndex((prev) => {
        const total = bannerImages.length;
        for (let step = 1; step <= total; step++) {
          const candidate = (prev + step) % total;
          if (!failedBanners.has(candidate)) return candidate;
        }
        return prev;
      });
    }, 6000);
    return () => clearInterval(interval);
  }, [
    loadableBanners.length,
    bannerImages.length,
    failedBanners,
    prefersReducedMotion,
    rotationPaused,
    motionPreferenceReady,
  ]);

  // The banner background sits behind a dark scrim, and the no-banner
  // fallback is a saturated brand gradient with a black/25 floor — both are
  // dark surfaces, so hero copy is always light.
  const hasBannerBackground = treatment.showBanner && loadableBanners.length > 0;
  // Banner treatment is always a dark surface (photo + scrim, or the designed
  // no-photo fallback). Minimal/classic sit on a light wash so H1 stays charcoal.
  const onDarkOverlay = treatment.showBanner;

  const effectiveLayout: HeroLayout =
    treatment.forceCentered || (heroLayout !== "centered" && !business.logo)
      ? "centered"
      : heroLayout;
  const isSplit = effectiveLayout !== "centered";
  const splitOrder =
    effectiveLayout === "split-right" ? mdFlexRowReverse() : "md:flex-row";
  function mdFlexRowReverse() {
    return "md:flex-row-reverse";
  }

  // Logo tiles: object-contain on a padded white/95 tile so wide wordmark
  // logos are never cropped (plan 1.1). The fallback gradient gets a larger
  // treatment so the brand still carries the hero without photography.
  const splitLogoSize = hasBannerBackground
    ? "w-44 h-44 lg:w-56 lg:h-56"
    : "w-52 h-52 lg:w-64 lg:h-64";
  const centeredLogoSize = hasBannerBackground
    ? "w-28 h-28 md:w-32 md:h-32"
    : "w-36 h-36 md:w-44 md:h-44";

  // CTA hierarchy (plan 3.1): primary from enabled features, "View menu"
  // secondary when it isn't already the primary, Call stays last.
  const primaryKind = resolvePrimaryStorefrontCta({ hasReservations, hasDelivery });
  const primaryAction =
    primaryKind === "reservations"
      ? onViewReservations ?? onViewMenu
      : primaryKind === "delivery"
        ? onOrderDelivery ?? onViewMenu
        : onViewMenu;
  const primaryLabel =
    t(STOREFRONT_CTA_LABEL_KEYS[primaryKind]) ||
    STOREFRONT_CTA_FALLBACKS[primaryKind];
  const viewMenuLabel =
    t("businessPage.hero.viewMenuCta") || STOREFRONT_CTA_FALLBACKS.menu;

  const outlineButtonClass = onDarkOverlay
    ? `font-semibold px-8 py-6 text-base ${radiusClass} border-white/40 text-white bg-white/5 hover:bg-white/10`
    : `font-semibold px-8 py-6 text-base ${radiusClass} border-ink-200 text-ink-900 bg-white hover:bg-warm-50`;

  return (
    // Hover/focus pause is required by WCAG 2.2.2 for auto-rotating banners.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <section
      className={`relative ${treatment.minHeightClass} flex items-center overflow-hidden`}
      data-hero-layout={effectiveLayout}
      data-header-style={designSettings.header_style || "banner"}
      onMouseEnter={() => setBannerHoverPause(true)}
      onMouseLeave={() => setBannerHoverPause(false)}
      onFocus={() => setBannerFocusPause(true)}
      onBlur={handleHeroBlur}
    >
      {treatment.bannerAsStrip && loadableBanners.length > 0 && (
        <div className="absolute top-0 inset-x-0 h-2 z-10" style={{ backgroundColor: safePrimaryColor }} />
      )}
      {/* Background */}
      {hasBannerBackground ? (
        <div className="absolute inset-0 z-0 bg-warm-100">
          {bannerImages.map((img: string, idx: number) => {
            if (failedBanners.has(idx)) return null;
            const isActive = idx === currentBannerIndex;
            if (prefersReducedMotion && !isActive) return null;
            return (
            <div
              key={idx}
              data-hero-slide={isActive ? "active" : "inactive"}
              ref={(el) => {
                if (!el) return;
                if (isActive) el.removeAttribute("inert");
                else el.setAttribute("inert", "");
              }}
              aria-hidden="true"
              className={`absolute inset-0 ${
                prefersReducedMotion ? "" : "transition-opacity duration-1000"
              } ${isActive ? "opacity-100" : "opacity-0"}`}
            >
              <NextImage
                src={img}
                unoptimized={!canOptimizeImageSrc(img)}
                alt=""
                fill
                className="object-cover"
                sizes="100vw"
                priority={idx === 0}
                onError={() => handleBannerError(idx)}
                onLoad={(e) => {
                  // Guard against zero-size "loaded" placeholders from the
                  // optimizer when the upstream CDN returned nothing usable.
                  const el = e.currentTarget;
                  if (
                    el.naturalWidth === 0 ||
                    el.naturalHeight === 0
                  ) {
                    handleBannerError(idx);
                    return;
                  }
                  handleBannerLoaded(idx);
                }}
              />
            </div>
            );
          })}
          {/* PG-15.4: contrast floor must be unconditional. Gating scrim
              opacity on image onLoad left text on the photo with opacity:0
              when a banner failed or load never fired (WCAG AA failure). */}
          <div
            data-testid="hero-scrim"
            className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/30 to-black/10 opacity-100"
          />
          {isSplit && (
            <div
              className={`absolute inset-y-0 w-full md:w-1/2 from-black/55 to-transparent ${
                heroLayout === "split-right"
                  ? "right-0 bg-gradient-to-l"
                  : "left-0 bg-gradient-to-r"
              }`}
            />
          )}
        </div>
      ) : (
        <div className="absolute inset-0 z-0" data-testid="hero-fallback-bg">
          {onDarkOverlay ? (
            <>
              <div
                className="absolute inset-0"
                style={{
                  background: `linear-gradient(135deg, ${safePrimaryColor} 0%, ${safeSecondaryColor} 100%)`,
                }}
              />
              {fallbackPatternImage && (
                <div
                  aria-hidden="true"
                  className="absolute inset-0"
                  style={{
                    backgroundImage: fallbackPatternImage,
                    opacity: designSettings.pattern_opacity ?? 0.15,
                  }}
                />
              )}
              <div aria-hidden="true" className="absolute inset-0 bg-black/25" />
            </>
          ) : (
            <div
              className="absolute inset-0 bg-warm-50"
              style={{
                backgroundImage: `linear-gradient(180deg, ${safePrimaryColor}18 0%, transparent 55%)`,
              }}
            />
          )}
        </div>
      )}

      {/* Indicator dots — clickable, aria-labelled, hidden under reduced
          motion (static first image) and for single/failed carousels. */}
      {!prefersReducedMotion && loadableIndices.length > 1 && (
        <div
          data-testid="hero-banner-dots"
          className="absolute bottom-4 left-1/2 -translate-x-1/2 flex items-center gap-2"
          style={zStyle(Z_DROPDOWN)}
        >
          {loadableIndices.map((bannerIdx, dotIdx) => {
            const isActive = bannerIdx === currentBannerIndex;
            return (
              <button
                key={bannerIdx}
                type="button"
                aria-label={
                  t("businessPage.hero.slideIndicator", {
                    index: dotIdx + 1,
                    total: loadableIndices.length,
                  }) ||
                  `Go to slide ${dotIdx + 1} of ${loadableIndices.length}`
                }
                aria-current={isActive}
                onClick={() => setCurrentBannerIndex(bannerIdx)}
                className={`w-2.5 h-2.5 rounded-full transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white ${
                  isActive ? "bg-white" : "bg-white/40 hover:bg-white/70"
                }`}
              />
            );
          })}
        </div>
      )}

      {!prefersReducedMotion && loadableBanners.length > 1 && (
        <button
          type="button"
          data-testid="hero-motion-toggle"
          onClick={() => setBannerPaused((prev) => !prev)}
          className={`absolute bottom-4 right-4 inline-flex items-center gap-2 px-3 py-2 text-sm font-semibold rounded-md bg-white/90 text-ink-900 border border-warm-200 hover:bg-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-600 ${radiusClass}`}
          style={zStyle(Z_DROPDOWN)}
          aria-label={
            bannerPaused
              ? t("businessPage.hero.playBanner") || "Play banner"
              : t("businessPage.hero.pauseBanner") || "Pause banner"
          }
        >
          {bannerPaused ? (
            <Play className="w-4 h-4" aria-hidden />
          ) : (
            <Pause className="w-4 h-4" aria-hidden />
          )}
          <span>
            {bannerPaused
              ? t("businessPage.hero.playBanner") || "Play banner"
              : t("businessPage.hero.pauseBanner") || "Pause banner"}
          </span>
        </button>
      )}

      {/* Content */}
      <div className="relative z-10 w-full">
        <div className="max-w-7xl mx-auto px-6 py-16 md:py-24">
          <div
            className={`flex flex-col gap-8 md:gap-12 ${
              isSplit
                ? splitOrder + " md:items-center"
                : "items-center text-center"
            }`}
          >
            {/* Logo column (split variants only) */}
            {isSplit && business.logo && !logoError && (
              <div className="hidden md:flex md:w-1/2 justify-center">
                <div
                  className={`${splitLogoSize} ${radiusClass} overflow-hidden ring-1 ring-white/25 ${shadowClass} bg-white/95 p-4`}
                >
                  <NextUIImage
                    src={business.logo}
                    alt={business.name}
                    className="w-full h-full object-contain"
                    onError={() => setLogoError(true)}
                  />
                </div>
              </div>
            )}

            <div
              className={`flex flex-col gap-6 ${
                isSplit ? "md:w-1/2" : "max-w-3xl"
              } ${
                isSplit
                  ? "items-start text-start"
                  : "items-center text-center"
              }`}
            >
              {/* Centered-only logo (hidden on mobile to avoid duplicating the H1) */}
              {!isSplit && business.logo && !logoError && (
                <div
                  className={`hidden md:block ${centeredLogoSize} ${radiusClass} overflow-hidden ring-1 ring-white/25 ${shadowClass} bg-white/95 p-3`}
                >
                  <NextUIImage
                    src={business.logo}
                    alt={business.name}
                    className="w-full h-full object-contain"
                    onError={() => setLogoError(true)}
                  />
                </div>
              )}

              <h1
                dir="auto"
                className={`font-title tracking-tight leading-[1.05] text-4xl md:text-5xl lg:text-6xl ${onDarkOverlay ? "text-white" : "text-ink-900"}`}
              >
                {business.name}
              </h1>

              {business.description && (
                <p
                  dir="auto"
                  className={`text-base md:text-lg leading-relaxed max-w-2xl ${onDarkOverlay ? "text-white/85" : "text-ink-600"}`}
                >
                  {business.description}
                </p>
              )}

              {/* Quiet status row */}
              <div
                className={`flex flex-wrap gap-3 ${
                  isSplit ? "" : "justify-center"
                }`}
              >
                {business.show_operating_hours && operatingHours.length > 0 && (
                  <span
                    role="status"
                    className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-semibold uppercase tracking-[0.18em] border ${
                      onDarkOverlay
                        ? "border-white/25 bg-white/10 text-white"
                        : "border-warm-200 bg-white text-ink-800"
                    }`}
                  >
                    {/* eslint-disable no-restricted-syntax -- functional indicator colors (green-600/red-600), intentionally not the merchant primary */}
                    <span
                      aria-hidden
                      className="w-1.5 h-1.5 rounded-full"
                      style={{
                        backgroundColor: isOpenNow ? "#16a34a" : "#dc2626",
                      }}
                    />
                    {/* eslint-enable no-restricted-syntax */}
                    {openStatusLabel}
                  </span>
                )}
                {business.show_reviews &&
                  business.google_reviews_enabled &&
                  googleRating && (
                    <a
                      href={
                        business.google_review_link ||
                        business.google_business_url ||
                        `https://www.google.com/search?q=${encodeURIComponent(business.name)}`
                      }
                      target="_blank"
                      rel="noopener noreferrer"
                      className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-xs border transition-colors ${
                        onDarkOverlay
                          ? "border-white/25 bg-white/10 text-white hover:bg-white/15"
                          : "border-warm-200 bg-white text-ink-800 hover:bg-warm-50"
                      }`}
                    >
                      <span className="flex items-center gap-0.5">
                        {[0, 1, 2, 3, 4].map((i) => (
                          <Star
                            key={i}
                            className={`w-3.5 h-3.5 ${
                              i < Math.floor(googleRating)
                                ? "fill-amber-400 text-amber-400"
                                : i < googleRating
                                ? "fill-amber-400/50 text-amber-400"
                                : "fill-transparent text-current opacity-30"
                            }`}
                          />
                        ))}
                      </span>
                      <span className="font-semibold tracking-wide">
                        {googleRating.toFixed(1)}
                      </span>
                      <span className="opacity-70">
                        {t("businessPage.onGoogle")}
                      </span>
                      <ExternalLink className="w-3 h-3 opacity-70" />
                    </a>
                  )}
              </div>

              {/* CTAs */}
              <div
                className={`flex flex-col sm:flex-row gap-3 ${
                  isSplit ? "" : "justify-center"
                } w-full sm:w-auto`}
              >
                <Button
                  size="lg"
                  className={`text-white font-semibold px-8 py-6 text-base ring-1 ring-white/20 transition-transform active:translate-y-[1px] ${radiusClass} ${shadowClass}`}
                  style={{ backgroundColor: safePrimaryColor }}
                  startContent={
                    primaryKind === "reservations" ? (
                      <Calendar className="w-5 h-5" />
                    ) : primaryKind === "delivery" ? (
                      <Navigation className="w-5 h-5" />
                    ) : undefined
                  }
                  endContent={
                    primaryKind === "menu" ? (
                      <ChevronRight className="w-5 h-5" />
                    ) : undefined
                  }
                  onPress={primaryAction}
                >
                  {primaryLabel}
                </Button>
                {primaryKind !== "menu" && (
                  <Button
                    size="lg"
                    variant="bordered"
                    className={outlineButtonClass}
                    onPress={onViewMenu}
                  >
                    {viewMenuLabel}
                  </Button>
                )}
                {business.phone && (
                  <Button
                    size="lg"
                    variant="bordered"
                    className={outlineButtonClass}
                    startContent={<Phone className="w-4 h-4" />}
                    as="a"
                    href={`tel:${business.phone}`}
                  >
                    {t("businessPage.hero.callCta") || "Call"}
                  </Button>
                )}
              </div>

            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
