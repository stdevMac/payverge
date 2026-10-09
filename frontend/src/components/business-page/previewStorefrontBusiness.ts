/**
 * Draft → storefront payload adapter for the Business Page live preview (#591).
 *
 * The editor keeps its in-progress state in operator-shaped slices (profile /
 * page settings / hours / gallery / features / design settings). The public
 * storefront tree consumes ONE `PublicBusiness` wire object where a few fields
 * are JSON strings (`banner_images`, `social_media`). This module is the single
 * translation step between the two, so the preview can mount the real
 * `ConvertingBusinessLandingPage` instead of a hand-rolled replica that drifts.
 *
 * Everything here is pure — no React, no network — so the mapping is unit
 * testable on its own.
 */

import type {
  BusinessAddress,
  BusinessDesignSettings,
  BusinessGalleryImage,
  BusinessOperatingException,
  BusinessOperatingHours,
  BusinessSpecialFeature,
} from "@/api/business";
import type { BusinessLanguage, SupportedLanguage } from "@/api/currency";
import type { PublicBusiness } from "@/api/publicBusiness";
import {
  DEFAULT_DESIGN_SETTINGS,
  type HeroLayout,
  type SectionDensity,
} from "./designClasses";
import {
  GUEST_SUPPORTED_LANGUAGES,
  type GuestLanguageCode,
} from "@/i18n/GuestTranslationProvider";

/**
 * In-progress Business Page form state. Everything is optional except the
 * pieces the editor always has, so design-only callers keep working with an
 * (almost) empty model.
 */
export type BusinessPageLivePreviewModel = {
  /** Real business id — drives the public menu / delivery / reservation reads. */
  businessId?: number;
  name: string;
  logo?: string;
  description?: string;
  phone?: string;
  website?: string;
  address?: BusinessAddress;
  social_media?: {
    instagram?: string;
    facebook?: string;
    twitter?: string;
    linkedin?: string;
    youtube?: string;
    tiktok?: string;
  };
  banner_images?: string[];
  operatingHours: BusinessOperatingHours[];
  operatingExceptions?: BusinessOperatingException[];
  showOperatingHours?: boolean;
  timezone?: string;
  customUrl?: string;

  /** About-tab content + its per-section visibility toggles. */
  welcomeMessage?: string;
  aboutStory?: string;
  showWelcomeMessage?: boolean;
  showAboutStory?: boolean;
  showGallery?: boolean;
  showSpecialFeatures?: boolean;
  galleryImages?: BusinessGalleryImage[];
  specialFeatures?: BusinessSpecialFeature[];

  /** Reviews block. */
  showReviews?: boolean;
  googleReviewsEnabled?: boolean;
  googlePlaceId?: string;
  googleBusinessName?: string;
  googleBusinessUrl?: string;
  googleReviewLink?: string;

  /** Money + locale. */
  defaultCurrency?: string;
  displayCurrency?: string;
  defaultLanguage?: string;
  businessLanguages?: BusinessLanguage[];
  supportedLanguages?: SupportedLanguage[];
  taxRate?: number;
  serviceFeeRate?: number;
};

const EMPTY_ADDRESS: BusinessAddress = {
  street: "",
  city: "",
  state: "",
  postal_code: "",
  country: "",
};

/** Currency is not editable on this screen — fall back to the wire default. */
const FALLBACK_CURRENCY = "USD";

/**
 * Merge the draft design settings over the shipped storefront defaults so the
 * preview never renders with half a theme while the editor is still loading.
 */
export function resolvePreviewDesignSettings(
  designSettings?: Partial<BusinessDesignSettings> | null,
): NonNullable<PublicBusiness["design_settings"]> {
  const merged = {
    ...DEFAULT_DESIGN_SETTINGS,
    ...(designSettings || {}),
  } as Record<string, unknown>;

  return {
    primary_color: String(merged.primary_color ?? ""),
    secondary_color: String(merged.secondary_color ?? ""),
    font_family: String(merged.font_family ?? ""),
    theme: String(merged.theme ?? "light"),
    menu_layout: String(merged.menu_layout ?? "grid"),
    show_images: merged.show_images !== false,
    show_descriptions: merged.show_descriptions !== false,
    header_style: String(merged.header_style ?? "banner"),
    corner_radius: String(merged.corner_radius ?? "medium"),
    shadow_intensity: String(merged.shadow_intensity ?? "subtle"),
    background_pattern: String(merged.background_pattern ?? "none"),
    pattern_opacity:
      typeof merged.pattern_opacity === "number" ? merged.pattern_opacity : 0.1,
    hero_layout: merged.hero_layout as HeroLayout,
    section_density: merged.section_density as SectionDensity,
  };
}

/**
 * The locale the preview should render in: the storefront's own default
 * language when it is a supported guest locale, otherwise the operator's
 * dashboard locale, otherwise English. A Spanish storefront must never be
 * previewed in English (#591).
 */
export function resolvePreviewLanguage(
  storefrontDefault?: string,
  operatorLocale?: string,
): GuestLanguageCode {
  const candidates = [storefrontDefault, operatorLocale];
  for (const candidate of candidates) {
    if (!candidate) continue;
    if (candidate in GUEST_SUPPORTED_LANGUAGES) {
      return candidate as GuestLanguageCode;
    }
    // "es-419" / "pt_BR" style tags fall back to their base language.
    const base = candidate.split(/[-_]/)[0];
    if (base && base in GUEST_SUPPORTED_LANGUAGES) {
      return base as GuestLanguageCode;
    }
  }
  return "en";
}

/**
 * Build the `PublicBusiness` payload the storefront tree expects from the
 * editor's draft state. Unsaved edits win over everything: this object is fed
 * to `ConvertingBusinessLandingPage previewMode`, which skips the localized
 * refetch precisely so the draft is not clobbered by saved server copy.
 */
export function buildPreviewStorefrontBusiness(
  model: BusinessPageLivePreviewModel,
  designSettings?: Partial<BusinessDesignSettings> | null,
): PublicBusiness {
  const banners = model.banner_images || [];

  return {
    id: model.businessId ?? 0,
    name: model.name || "",
    description: model.description || "",
    logo: model.logo || "",
    // Wire contract: banner_images and social_media are JSON strings.
    banner_images: JSON.stringify(banners),
    custom_url: model.customUrl || "",
    website: model.website || "",
    phone: model.phone || "",
    address: {
      street: model.address?.street || EMPTY_ADDRESS.street,
      city: model.address?.city || EMPTY_ADDRESS.city,
      state: model.address?.state || EMPTY_ADDRESS.state,
      postal_code: model.address?.postal_code || EMPTY_ADDRESS.postal_code,
      country: model.address?.country || EMPTY_ADDRESS.country,
    },
    social_media: JSON.stringify(model.social_media || {}),
    default_currency: model.defaultCurrency || FALLBACK_CURRENCY,
    display_currency:
      model.displayCurrency || model.defaultCurrency || FALLBACK_CURRENCY,
    default_language: model.defaultLanguage || "en",
    page_enabled: true,
    is_active: true,
    google_business_name: model.googleBusinessName || "",
    google_business_url: model.googleBusinessUrl || "",
    google_place_id: model.googlePlaceId || "",
    google_review_link: model.googleReviewLink || "",
    google_reviews_enabled: Boolean(model.googleReviewsEnabled),
    show_reviews: model.showReviews !== false,
    timezone: model.timezone || "",
    business_languages: model.businessLanguages,
    supported_languages: model.supportedLanguages,
    design_settings: resolvePreviewDesignSettings(designSettings),

    welcome_message: model.welcomeMessage || "",
    about_story: model.aboutStory || "",
    gallery_images: model.galleryImages || [],
    show_gallery: Boolean(model.showGallery),
    show_welcome_message: Boolean(model.showWelcomeMessage),
    show_about_story: Boolean(model.showAboutStory),
    operating_hours: model.operatingHours || [],
    operating_exceptions: model.operatingExceptions || [],
    show_operating_hours: Boolean(model.showOperatingHours),
    special_features: model.specialFeatures || [],
    show_special_features: Boolean(model.showSpecialFeatures),

    tax_rate: model.taxRate,
    service_fee_rate: model.serviceFeeRate,

    // The preview never mounts the AI waiter (it would open a real assistant
    // session from the dashboard) — say so in the payload too.
    ai_settings: {
      ai_enabled: false,
      ai_name: "",
      ai_priority: "",
      business_page_ai_enabled: false,
    },

    created_at: "",
    updated_at: "",
  };
}
