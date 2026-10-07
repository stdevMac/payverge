"use client";

import React, { useState, useEffect, useCallback } from "react";
import Image from "next/image";
import {
  ChevronLeft,
  ChevronRight,
  Star,
  ExternalLink,
} from "lucide-react";
import { Link } from "@nextui-org/react";
import { getBusinessGoogleReviews, GoogleReview } from "@/api/googleReviews";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { getLocaleDirection } from "@/i18n/localeRegistry";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import { getRadiusClass, getShadowClass } from "./designClasses";
import ReviewsEmptyState from "./ReviewsEmptyState";

// ReviewText component for handling long text with read more functionality
const ReviewText: React.FC<{
  text: string;
  authorUrl?: string;
  maxLines: number;
  compact?: boolean;
  t: (key: string) => string;
}> = ({ text, authorUrl, maxLines, compact = false, t }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [showReadMore, setShowReadMore] = useState(false);
  const textRef = React.useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const el = textRef.current;
    if (!el) return;
    // While clamped, clientHeight is the clamped box and scrollHeight the
    // full content — offer the toggle only when the text actually overflows
    // (plan 1.10). The CSS line-clamp does the truncation; there is no
    // substring cut, so words are never sliced mid-word.
    setShowReadMore(el.scrollHeight > el.clientHeight);
  }, [text, maxLines]);

  return (
    <div className="space-y-2">
      <p
        ref={textRef}
        dir="auto"
        className={`text-gray-700 leading-relaxed ${compact ? "text-sm" : ""}`}
        style={{
          // Line clamping is applied via WebkitLineClamp below (a runtime
          // value); a `line-clamp-${maxLines}` Tailwind class would never
          // compile and is redundant with this style.
          display: "-webkit-box",
          WebkitLineClamp: !isExpanded ? maxLines : "none",
          WebkitBoxOrient: "vertical",
          overflow: "hidden",
        }}
      >
        {text}
      </p>

      <div
        className={`flex items-center gap-3 ${compact ? "text-xs" : "text-sm"}`}
      >
        {showReadMore && (
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="text-brand hover:text-brand-dark font-medium transition-colors"
          >
            {isExpanded
              ? t("businessPage.reviews.showLess")
              : t("businessPage.reviews.readMore")}
          </button>
        )}

        {authorUrl && (
          <Link
            href={authorUrl}
            isExternal
            className="text-gray-500 hover:text-gray-700 font-medium"
          >
            {t("businessPage.reviews.viewOriginal")}
            <ExternalLink className="w-3 h-3 ms-1 inline" />
          </Link>
        )}
      </div>
    </div>
  );
};

interface GoogleReviewsSliderProps {
  businessName: string;
  customUrl: string;
  googlePlaceId?: string;
  googleBusinessUrl?: string;
  googleReviewLink?: string;
  googleReviewsEnabled?: boolean;
  showReviews?: boolean;
  designSettings: {
    primaryColor: string;
    secondaryColor: string;
    cornerRadius?: string;
    shadowIntensity?: string;
    fontFamily?: string;
  };
}

const GoogleReviewsSlider: React.FC<GoogleReviewsSliderProps> = ({
  businessName: _businessName,
  customUrl,
  googlePlaceId,
  googleBusinessUrl,
  googleReviewLink,
  googleReviewsEnabled = false,
  showReviews = true,
  designSettings,
}) => {
  const { t, currentLanguage } = useGuestTranslation();
  // RTL: the flex track lays out end-to-start under dir=rtl, so the slide
  // transform must be positive there (registry-driven, same source of truth
  // as the provider's document.dir).
  const isRtl = getLocaleDirection(currentLanguage) === "rtl";
  const [currentIndex, setCurrentIndex] = useState(0);
  const [reviews, setReviews] = useState<GoogleReview[]>([]);
  const [loading, setLoading] = useState(true);
  const [_error, setError] = useState<string | null>(null);
  // Plan 1.8: broken avatar URLs tracked in state so the fallback initials
  // tile renders declaratively (replacing the old document.createElement +
  // replaceChildren DOM mutation, which breaks under concurrent rendering).
  const [failedAvatars, setFailedAvatars] = useState<Set<string>>(new Set());

  const radiusClass = getRadiusClass(designSettings.cornerRadius || "medium");
  const shadowClass = getShadowClass(designSettings.shadowIntensity || "subtle");
  // Depend on the translated value rather than the provider's function
  // identity. Some providers/tests return a new `t` closure each render; the
  // localized fallback only needs to refresh when its resulting copy changes.
  const loadErrorFallback = String(t("googleReviews.loadError"));

  // Ensure reviews is always an array
  const safeReviews = Array.isArray(reviews) ? reviews : [];

  // Determine how many reviews to show based on screen size
  const getVisibleReviews = useCallback(() => {
    if (typeof window === "undefined") return 1;
    const width = window.innerWidth;
    if (width >= 1024) return 3;
    if (width >= 768) return 2;
    return 1;
  }, []);

  const [visibleReviews, setVisibleReviews] = useState(1); // Start with 1 for SSR
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setPrefersReducedMotion(mq.matches);
    const listener = (e: MediaQueryListEvent) =>
      setPrefersReducedMotion(e.matches);
    mq.addEventListener?.("change", listener);
    return () => mq.removeEventListener?.("change", listener);
  }, []);

  // Update visible reviews on window resize and initial mount
  useEffect(() => {
    // Set initial value after component mounts
    setVisibleReviews(getVisibleReviews());

    const handleResize = () => {
      const newVisibleReviews = getVisibleReviews();
      setVisibleReviews(newVisibleReviews);

      // Reset currentIndex if it's out of bounds
      setCurrentIndex((prevIndex) => {
        const maxIndex = Math.max(0, safeReviews.length - newVisibleReviews);
        return prevIndex > maxIndex ? 0 : prevIndex;
      });
    };

    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, [safeReviews.length, getVisibleReviews]);

  // Auto-slide functionality - adjusted for multiple reviews
  useEffect(() => {
    if (prefersReducedMotion) return;
    if (safeReviews.length > visibleReviews) {
      const interval = setInterval(() => {
        setCurrentIndex((prevIndex) => {
          const maxIndex = Math.max(0, safeReviews.length - visibleReviews);
          return prevIndex >= maxIndex ? 0 : prevIndex + 1;
        });
      }, 7000); // Change slide every 7 seconds (longer for multiple reviews)

      return () => clearInterval(interval);
    }
  }, [safeReviews.length, visibleReviews, prefersReducedMotion]);

  // Load reviews from real API
  useEffect(() => {
    const loadReviews = async () => {
      if (!googleReviewsEnabled || !googlePlaceId || !customUrl) {
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const response = await getBusinessGoogleReviews(
          customUrl,
          currentLanguage,
        );
        // Ensure we always set a valid array
        const reviewsData = response?.reviews;
        setReviews(Array.isArray(reviewsData) ? reviewsData : []);
      } catch (err) {
        console.error("Error fetching Google reviews:", err);
        // FIND-058: never surface axios transport / gin dumps to guests.
        setError(
          getSafeApiErrorMessage(err, loadErrorFallback),
        );
        setReviews([]);
      } finally {
        setLoading(false);
      }
    };

    void loadReviews();
  }, [
    customUrl,
    googlePlaceId,
    googleReviewsEnabled,
    currentLanguage,
    loadErrorFallback,
  ]);

  const nextSlide = () => {
    if (safeReviews.length === 0) return;
    const maxIndex = Math.max(0, safeReviews.length - visibleReviews);
    setCurrentIndex((prevIndex) => (prevIndex >= maxIndex ? 0 : prevIndex + 1));
  };

  const prevSlide = () => {
    if (safeReviews.length === 0) return;
    const maxIndex = Math.max(0, safeReviews.length - visibleReviews);
    setCurrentIndex((prevIndex) => (prevIndex <= 0 ? maxIndex : prevIndex - 1));
  };

  const renderStars = (rating: number) => {
    return Array.from({ length: 5 }, (_, index) => (
      <Star
        key={index}
        className={`w-4 h-4 ${index < rating ? "text-amber-400 fill-amber-400" : "text-gray-400"
          }`}
      />
    ));
  };

  // Don't render if reviews are disabled or no Google integration
  if (!showReviews || !googleReviewsEnabled || !googlePlaceId) {
    return null;
  }

  if (loading) {
    return (
      <div className={`bg-white p-8 ${radiusClass} ${shadowClass} border border-gray-200`}>
        <div className="mb-6">
          <h3 className="font-title text-2xl md:text-3xl text-gray-900">
            {t("businessPage.reviews.customerReviews")}
          </h3>
        </div>

        <div className="flex items-center justify-center py-12">
          <div
            className="animate-spin rounded-full h-8 w-8 border-b-2"
            style={{ borderColor: designSettings.primaryColor }}
          ></div>
          <span className="ms-3 text-gray-600">
            {t("businessPage.reviews.loadingReviews")}
          </span>
        </div>
      </div>
    );
  }

  if (reviews.length === 0) {
    return (
      <div className={`bg-white p-8 ${radiusClass} ${shadowClass} border border-gray-200`}>
        <div className="mb-6">
          <h3 className="font-title text-2xl md:text-3xl text-gray-900">
            {t("businessPage.reviews.customerReviews")}
          </h3>
        </div>

        <ReviewsEmptyState
          primaryColor={designSettings.primaryColor}
          radiusClass={radiusClass}
          reviewLink={googleReviewLink}
          ctaLabel={t("businessPage.reviews.writeReview")}
          bodyText={t("businessPage.reviews.beFirstReview")}
        />
      </div>
    );
  }

  return (
    <div className={`bg-white p-8 ${radiusClass} ${shadowClass} border border-gray-200 relative`}>
      {/* Header */}
      <div className="flex flex-wrap items-baseline justify-between gap-3 mb-6">
        <div className="flex items-baseline gap-3">
          <h3 className="font-title text-2xl md:text-3xl text-gray-900">
            {t("businessPage.reviews.customerReviews")}
          </h3>
          <span
            aria-hidden
            className="hidden sm:block h-px w-10"
            style={{ backgroundColor: `${designSettings.primaryColor}66` }}
          />
        </div>
        {googleBusinessUrl ? (
          <Link
            href={googleBusinessUrl}
            isExternal
            className="text-sm text-gray-500 hover:text-gray-900 inline-flex items-center gap-1"
          >
            {t("businessPage.reviews.viewOnGoogle")}
            <ExternalLink className="w-3 h-3" />
          </Link>
        ) : null}
      </div>

      {/* Multi-Review Carousel - Shows 2-3 reviews at once */}
      <div className="relative overflow-hidden">
        <div
          className={`flex gap-6 ${prefersReducedMotion ? "" : "transition-transform duration-700 ease-in-out"}`}
          style={{
            transform: `translateX(${isRtl ? "" : "-"}${visibleReviews > 0 ? currentIndex * (100 / visibleReviews) : 0}%)`,
          }}
        >
          {safeReviews.map((review, index) => (
            <div
              key={index}
              className={`flex-shrink-0 bg-white ${radiusClass} p-6 border border-gray-200 hover:border-gray-300 transition-colors ${visibleReviews === 1
                ? "w-full"
                : visibleReviews === 2
                  ? "w-[calc(50%-12px)]"
                  : "w-[calc(33.333%-16px)]"
                }`}
            >
              {/* Profile Photo with Placeholder */}
              <div className="flex items-center gap-4 mb-4">
                <div className="flex-shrink-0">
                  <div
                    className="w-12 h-12 rounded-full overflow-hidden shadow-md flex items-center justify-center text-white font-bold text-sm"
                    style={{ backgroundColor: designSettings.primaryColor }}
                  >
                    {review.profile_photo_url &&
                    !failedAvatars.has(review.profile_photo_url) ? (
                      <Image
                        src={review.profile_photo_url}
                        alt={review.author_name}
                        width={48}
                        height={48}
                        unoptimized
                        className="w-full h-full object-cover"
                        onError={() =>
                          setFailedAvatars((prev) =>
                            new Set(prev).add(review.profile_photo_url!),
                          )
                        }
                      />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-white font-bold text-sm">
                        {review.author_name.charAt(0).toUpperCase()}
                      </div>
                    )}
                  </div>
                </div>

                {/* Header Info */}
                <div className="flex-1 min-w-0">
                  <h4 className="font-semibold text-gray-900 truncate text-sm">
                    {review.author_name}
                  </h4>
                  <div className="flex items-center gap-2 mt-1">
                    <div className="flex items-center gap-1">
                      {renderStars(review.rating)}
                    </div>
                    <span className="text-xs text-gray-500">
                      {review.relative_time_description}
                    </span>
                  </div>
                </div>
              </div>

              {/* Review Text */}
              <div className="flex-1">
                <ReviewText
                  text={review.text}
                  authorUrl={review.author_url}
                  maxLines={6}
                  compact={true}
                  t={t}
                />
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Navigation Controls */}
      {safeReviews.length > visibleReviews && (
        <div className="flex items-center justify-between mt-6">
          <button
            onClick={prevSlide}
            className="p-2 rounded-full bg-gray-100 hover:bg-gray-200 transition-colors duration-200"
          >
            <ChevronLeft className="w-5 h-5 text-gray-600 rtl:rotate-180" />
          </button>

          {/* Dots Indicator - Show dots for each possible position */}
          <div className="flex items-center gap-2">
            {safeReviews.length > 0 &&
              visibleReviews > 0 &&
              Array.from(
                {
                  length: Math.max(
                    0,
                    safeReviews.length - visibleReviews + 1,
                  ),
                },
                (_, index) => (
                  <button
                    key={index}
                    onClick={() => setCurrentIndex(index)}
                    // Fixed-size pill (w-6) whose inactive width is achieved by a
                    // GPU-composited scaleX transform, not by animating the
                    // layout `width` property (which forces reflow per frame).
                    className={`h-2 w-6 origin-center rounded-full transition-[transform,background-color] duration-200 ${index === currentIndex ? "scale-x-100" : "scale-x-[0.333] hover:bg-gray-400"
                      }`}
                    style={{
                      // eslint-disable-next-line no-restricted-syntax -- dynamic user primaryColor / CSS gray fallback (#d1d5db)
                      backgroundColor: index === currentIndex ? designSettings.primaryColor : "#d1d5db",
                    }}
                  />
                ),
              )}
          </div>

          <button
            onClick={nextSlide}
            className="p-2 rounded-full bg-gray-100 hover:bg-gray-200 transition-colors duration-200"
          >
            <ChevronRight className="w-5 h-5 text-gray-600 rtl:rotate-180" />
          </button>
        </div>
      )}

      {/* Action Buttons */}
      <div className="flex items-center justify-center gap-4 mt-6 pt-6 border-t border-gray-200/60">
        {googleReviewLink && (
          <Link
            href={googleReviewLink}
            isExternal
            className={`inline-flex items-center gap-2 px-5 py-2.5 ${radiusClass} text-sm font-medium text-white transition-colors active:translate-y-[1px]`}
            style={{ backgroundColor: designSettings.primaryColor }}
          >
            <Star className="w-4 h-4" />
            {t("businessPage.reviews.writeReview")}
            <ExternalLink className="w-3.5 h-3.5" />
          </Link>
        )}

        {googleBusinessUrl && (
          <Link
            href={googleBusinessUrl}
            isExternal
            className={`inline-flex items-center gap-2 px-5 py-2.5 ${radiusClass} font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 transition-colors`}
          >
            {t("businessPage.reviews.viewAllReviews")}
            <ExternalLink className="w-4 h-4" />
          </Link>
        )}
      </div>
    </div>
  );
};

export default GoogleReviewsSlider;
