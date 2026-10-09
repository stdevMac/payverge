"use client";

import React, { useState } from "react";
import { PublicBusiness } from "@/api/publicBusiness";
import { Image as NextUIImage } from "@nextui-org/react";
import GoogleReviewsSlider from "./GoogleReviewsSlider";
import GalleryLightbox from "./GalleryLightbox";
import SectionHeader from "./SectionHeader";
import { getRadiusClass, getSectionPadding } from "./designClasses";
import { pickFeatureIcon } from "./featureIcons";
import { capitalizeFirstLetter } from "@/utils/capitalizeFirstLetter";

interface BusinessAboutTabProps {
  business: PublicBusiness;
  customUrl: string;
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
    shadow_intensity?: string;
    font_family?: string;
    section_density?: string;
  };
  specialFeatures: any[];
  galleryImages: any[];
  t: (key: string) => string;
  /** Switch the storefront to the menu tab (empty-state CTA). */
  onViewMenu?: () => void;
}

// Icon resolution lives in featureIcons.tsx so the editor's stored icon keys
// (wifi, car, credit-card, …) resolve on the public page. Title heuristics are
// a fallback only when icon is missing/unknown.

export default function BusinessAboutTab({
  business,
  customUrl,
  designSettings,
  specialFeatures,
  galleryImages,
  t,
  onViewMenu,
}: BusinessAboutTabProps) {
  const radiusClass = getRadiusClass(designSettings.corner_radius);
  const sectionPadding = getSectionPadding(designSettings.section_density);
  const [failedGalleryImages, setFailedGalleryImages] = useState<Set<number>>(
    new Set(),
  );
  // Plan 1.6: index of the gallery image open in the lightbox (null = closed).
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);

  // PARITY-5: fold show_reviews into the gate so turning reviews off hides the
  // whole "What customers say" section instead of leaving an orphan header.
  const hasReviews = Boolean(
    business.show_reviews &&
    business.google_reviews_enabled &&
    business.google_place_id,
  );
  // PARITY-3: gate on show_special_features (mirror the show_gallery pattern)
  // so the "Why choose us" toggle actually hides the section.
  const hasFeatures = Boolean(
    business.show_special_features && specialFeatures.length > 0,
  );
  const hasGallery = Boolean(business.show_gallery && galleryImages.length > 0);
  // IA-1: Welcome & Story are real content sections; include them so a
  // story-only/welcome-only page is not falsely rendered as the empty state.
  const hasWelcome = Boolean(
    business.show_welcome_message && business.welcome_message?.trim(),
  );
  const hasStory = Boolean(
    business.show_about_story && business.about_story?.trim(),
  );
  // About is the default landing tab. With none of the sections present
  // (a common just-onboarded state) the panel would otherwise render empty,
  // leaving a full-viewport blank below the hero. Show a graceful default.
  const isEmpty =
    !hasReviews && !hasFeatures && !hasGallery && !hasWelcome && !hasStory;

  const addressSummary = [
    business.address?.street,
    business.address?.city,
    business.address?.state,
  ]
    .filter(Boolean)
    .join(", ");

  return (
    <div
      role="tabpanel"
      id="tabpanel-about"
      aria-labelledby="tab-about"
      tabIndex={-1}
    >
      {isEmpty && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-2xl mx-auto px-6 text-center">
            {business.description ? (
              <p dir="auto" className="text-base text-gray-700 leading-relaxed">
                {business.description}
              </p>
            ) : (
              <p className="text-base text-gray-700 leading-relaxed">
                {t("businessPage.aboutEmptyWelcome")}
              </p>
            )}
            {addressSummary && (
              <p className="mt-4 text-sm text-gray-500">{addressSummary}</p>
            )}
            {onViewMenu && (
              <button
                type="button"
                onClick={onViewMenu}
                className={`mt-8 inline-flex items-center justify-center px-6 py-3 text-sm font-semibold text-white transition-opacity hover:opacity-90 ${radiusClass}`}
                style={{ backgroundColor: designSettings.primary_color }}
              >
                {t("businessPage.aboutViewMenuCta")}
              </button>
            )}
          </div>
        </section>
      )}

      {/* Welcome lead — a warm greeting at the top of the About tab (IA-1). */}
      {hasWelcome && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-3xl mx-auto px-6 text-center">
            <p
              dir="auto"
              className="text-lg md:text-xl text-gray-700 leading-relaxed"
            >
              {business.welcome_message}
            </p>
          </div>
        </section>
      )}

      {/* Our story — a dedicated narrative section (IA-1). */}
      {hasStory && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-3xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.ourStory")}
              designSettings={designSettings}
              centered={false}
            />
            <p
              dir="auto"
              className="text-base text-gray-700 leading-relaxed whitespace-pre-line"
            >
              {business.about_story}
            </p>
          </div>
        </section>
      )}

      {/* Reviews */}
      {hasReviews && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.whatCustomersSay")}
              subtitle={t("businessPage.realReviews")}
              badge={t("businessPage.reviewsBadge")}
              designSettings={designSettings}
              centered={false}
            />
            <GoogleReviewsSlider
              businessName={business.name}
              customUrl={customUrl}
              googlePlaceId={business.google_place_id}
              googleBusinessUrl={business.google_business_url}
              googleReviewLink={business.google_review_link}
              googleReviewsEnabled={business.google_reviews_enabled}
              showReviews={business.show_reviews}
              designSettings={{
                primaryColor: designSettings.primary_color,
                secondaryColor: designSettings.secondary_color,
                cornerRadius: designSettings.corner_radius,
                shadowIntensity: designSettings.shadow_intensity,
                fontFamily: designSettings.font_family,
              }}
            />
          </div>
        </section>
      )}

      {/* Features — flat 2-col with alternating row tint */}
      {hasFeatures && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-5xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.whyChooseUs")}
              subtitle={t("businessPage.experienceBest")}
              badge={t("businessPage.featuresBadge")}
              designSettings={designSettings}
              centered={false}
            />
            <ul className="grid gap-x-12 gap-y-8 md:grid-cols-2 auto-rows-fr">
              {specialFeatures.map((feature: any, index: number) => {
                const Icon = pickFeatureIcon(feature.icon, feature.title);
                const tintAlpha = Math.floor(index / 2) % 2 === 0 ? "10" : "1A";
                const tintBg = `${designSettings.primary_color}${tintAlpha}`;
                return (
                  <li key={feature.id || index} className="flex gap-4">
                    <div
                      className={`w-10 h-10 ${radiusClass} flex items-center justify-center flex-shrink-0`}
                      style={{
                        backgroundColor: tintBg,
                        color: designSettings.primary_color,
                      }}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    <div className="flex-1">
                      <p
                        dir="auto"
                        className="font-semibold text-gray-900 mb-1"
                      >
                        {capitalizeFirstLetter(feature.title || "")}
                      </p>
                      {feature.description && (
                        <p
                          dir="auto"
                          className="text-sm text-gray-600 leading-relaxed"
                        >
                          {feature.description}
                        </p>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>
        </section>
      )}

      {/* Gallery */}
      {business.show_gallery && galleryImages.length > 0 && (
        <section className={`${sectionPadding} relative z-10`}>
          <div className="max-w-6xl mx-auto px-6">
            <SectionHeader
              title={t("businessPage.galleryTitle")}
              subtitle={t("businessPage.glimpseRestaurant")}
              badge={t("businessPage.galleryBadge")}
              designSettings={designSettings}
              centered={false}
            />
            {/* Desktop grid — switched from CSS columns to a real grid
                so an odd number of images doesn't leave a "hole" in the
                bottom-right cell. With `auto-fill` + min-max sizing the
                row reflows symmetrically regardless of image count. */}
            <div className="hidden md:grid gap-4 grid-cols-[repeat(auto-fill,minmax(280px,1fr))]">
              {galleryImages.map((img: any, index: number) =>
                failedGalleryImages.has(index) ? null : (
                  <figure
                    key={img.id || index}
                    className={`relative overflow-hidden ${radiusClass} border border-gray-200 transition-colors hover:border-gray-300 group`}
                  >
                    <button
                      type="button"
                      onClick={() => setLightboxIndex(index)}
                      aria-label={t("businessPage.gallery.viewImage").replace(
                        "{index}",
                        String(index + 1),
                      )}
                      className="block w-full text-left"
                    >
                      <div className="aspect-[4/3]">
                        <NextUIImage
                          src={img.image_url}
                          alt={
                            img.caption ||
                            t("businessPage.galleryImageAlt").replace(
                              "{index}",
                              String(index + 1),
                            ) ||
                            `Gallery ${index + 1}`
                          }
                          className="w-full h-full object-cover transition-opacity duration-300 group-hover:opacity-95"
                          removeWrapper
                          onError={() =>
                            setFailedGalleryImages((prev) =>
                              new Set(prev).add(index),
                            )
                          }
                        />
                      </div>
                    </button>
                    {/* Plan 1.6: visible caption bar (was alt-text only).
                        pointer-events-none keeps clicks on the open button. */}
                    {Boolean(img.caption?.trim()) && (
                      <figcaption className="pointer-events-none absolute inset-x-0 bottom-0 bg-white/90 px-3 py-1.5 text-xs text-ink-800 backdrop-blur-sm line-clamp-2">
                        {img.caption}
                      </figcaption>
                    )}
                  </figure>
                ),
              )}
            </div>
            {/* Mobile slider */}
            <div className="md:hidden overflow-x-auto scrollbar-hide pb-4 -mx-6">
              <div className="flex gap-3 px-6">
                {galleryImages.map((img: any, index: number) =>
                  failedGalleryImages.has(index) ? null : (
                    <figure
                      key={img.id || index}
                      className={`relative w-[260px] h-[200px] flex-shrink-0 overflow-hidden ${radiusClass} border border-gray-200`}
                    >
                      <button
                        type="button"
                        onClick={() => setLightboxIndex(index)}
                        aria-label={t(
                          "businessPage.gallery.viewImage",
                        ).replace("{index}", String(index + 1))}
                        className="block h-full w-full text-left"
                      >
                        <NextUIImage
                          src={img.image_url}
                          alt={
                            img.caption ||
                            t("businessPage.galleryImageAlt").replace(
                              "{index}",
                              String(index + 1),
                            ) ||
                            `Gallery ${index + 1}`
                          }
                          className="w-full h-full object-cover"
                          removeWrapper
                          onError={() =>
                            setFailedGalleryImages((prev) =>
                              new Set(prev).add(index),
                            )
                          }
                        />
                      </button>
                      {Boolean(img.caption?.trim()) && (
                        <figcaption className="pointer-events-none absolute inset-x-0 bottom-0 bg-white/90 px-3 py-1.5 text-xs text-ink-800 backdrop-blur-sm line-clamp-2">
                          {img.caption}
                        </figcaption>
                      )}
                    </figure>
                  ),
                )}
              </div>
            </div>
          </div>
        </section>
      )}

      {/* Plan 1.6: full-screen viewer; mounted only while open. */}
      {lightboxIndex !== null && (
        <GalleryLightbox
          images={galleryImages}
          initialIndex={lightboxIndex}
          t={t}
          onClose={() => setLightboxIndex(null)}
        />
      )}
    </div>
  );
}
