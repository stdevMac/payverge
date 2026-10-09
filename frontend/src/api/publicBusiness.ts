import { axiosInstance } from './tools/instance';
import type { BusinessLanguage, SupportedLanguage } from './currency';
import type { Bundle, Offer, MenuCategory, Menu } from './business';
import type { Orderability } from './orders';
import type { HeroLayout, SectionDensity } from "@/components/business-page/designClasses";
import { getPatternDataUri, isStorefrontPattern } from "@/lib/storefront/theme";

interface PublicBusinessAddress {
  street: string;
  city: string;
  state: string;
  postal_code: string;
  country: string;
}

export interface PublicBusiness {
  id: number;
  name: string;
  description: string;
  logo: string;
  banner_images: string;
  custom_url: string;
  website: string;
  phone: string;
  address: PublicBusinessAddress;
  social_media: string;
  default_currency: string;
  display_currency: string;
  default_language?: string;
  page_enabled?: boolean;
  is_active?: boolean;
  google_business_name: string;
  google_business_url: string;
  google_place_id: string;
  google_review_link: string;
  google_reviews_enabled: boolean;
  show_reviews: boolean;
  timezone?: string;
  business_languages?: BusinessLanguage[];
  supported_languages?: SupportedLanguage[];

  // Design settings
  design_settings?: {
    primary_color: string;
    secondary_color: string;
    font_family: string;
    theme: string;
    menu_layout: string;
    show_images: boolean;
    show_descriptions: boolean;
    header_style: string;
    corner_radius: string;
    shadow_intensity: string;
    background_pattern: string;
    pattern_opacity: number;
    hero_layout?: HeroLayout;
    section_density?: SectionDensity;
  };

  // New configurable hospitality features
  welcome_message?: string;
  about_story?: string;
  gallery_images?: Array<{
    id: number;
    business_id: number;
    image_url: string;
    caption: string;
    display_order: number;
    is_active: boolean;
    created_at: string;
    updated_at: string;
  }>;
  show_gallery?: boolean;
  show_welcome_message?: boolean;
  show_about_story?: boolean;
  operating_hours?: Array<{
    id: number;
    business_id: number;
    day_of_week: number; // 0=Sunday, 1=Monday, etc.
    open_time: string;   // Format: "09:00"
    close_time: string;  // Format: "17:00"
    kitchen_close_time?: string | null;
    is_closed: boolean;
    created_at: string;
    updated_at: string;
  }>;
  operating_exceptions?: Array<{
    id: number;
    business_id: number;
    exception_date: string;
    open_time?: string | null;
    close_time?: string | null;
    kitchen_close_time?: string | null;
    is_closed: boolean;
    label?: string;
    created_at: string;
    updated_at: string;
  }>;
  show_operating_hours?: boolean;
  special_features?: Array<{
    id: number;
    business_id: number;
    title: string;
    description: string;
    icon: string;
    display_order: number;
    is_active: boolean;
    created_at: string;
    updated_at: string;
  }>;
  show_special_features?: boolean;

  // Financial rates — used in the guest checkout price breakdown estimate.
  tax_rate?: number;
  service_fee_rate?: number;

  // AI Waiter settings
  ai_settings?: {
    ai_enabled: boolean;
    ai_name: string;
    ai_priority: string;
    special_instructions?: string;
    business_page_ai_enabled?: boolean;
  };
  // Authoritative guest AI gate (instance AI + operational business + toggle).
  ai_available?: boolean;
  // "llm" when a model is configured; "basic" for set replies from the menu.
  ai_waiter_mode?: "llm" | "basic";

  created_at: string;
  updated_at: string;

  // Demo Center / seed classification. Public so storefronts can noindex
  // without guessing slugs. Real tenants omit or send false / "real".
  is_demo?: boolean;
  kind?: "real" | "demo" | "test" | string;
}

export function isNonIndexableStorefront(
  business: { is_demo?: boolean; kind?: string } | null | undefined,
): boolean {
  if (!business) return false;
  // Published demo showrooms (kind=demo) are live converting storefronts and
  // must stay indexable (#612 / #637). CI/local fixtures (kind=test) stay out.
  const kind = (business.kind || "").toLowerCase();
  return kind === "test";
}

type RawPublicBusiness = Omit<
  Partial<PublicBusiness>,
  "address" | "business_languages" | "supported_languages"
> & {
  business_page_enabled?: boolean;
  address?: unknown;
  business_languages?: unknown;
  supported_languages?: unknown;
};

const EMPTY_ADDRESS: PublicBusinessAddress = {
  street: "",
  city: "",
  state: "",
  postal_code: "",
  country: "",
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function normalizeAddress(value: unknown): PublicBusinessAddress {
  if (!isRecord(value)) {
    return EMPTY_ADDRESS;
  }

  return {
    street: typeof value.street === "string" ? value.street : "",
    city: typeof value.city === "string" ? value.city : "",
    state: typeof value.state === "string" ? value.state : "",
    postal_code: typeof value.postal_code === "string" ? value.postal_code : "",
    country: typeof value.country === "string" ? value.country : "",
  };
}

function normalizeSupportedLanguages(value: unknown): SupportedLanguage[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((entry) => {
    if (typeof entry === "string") {
      return [{
        id: 0,
        code: entry,
        name: entry,
        native_name: entry,
        is_active: true,
        created_at: "",
        updated_at: "",
      }];
    }

    if (!isRecord(entry) || typeof entry.code !== "string") {
      return [];
    }

    return [{
      id: typeof entry.id === "number" ? entry.id : 0,
      code: entry.code,
      name: typeof entry.name === "string" ? entry.name : entry.code,
      native_name: typeof entry.native_name === "string" ? entry.native_name : entry.code,
      is_active: typeof entry.is_active === "boolean" ? entry.is_active : true,
      created_at: typeof entry.created_at === "string" ? entry.created_at : "",
      updated_at: typeof entry.updated_at === "string" ? entry.updated_at : "",
    }];
  });
}

function normalizeBusinessLanguages(value: unknown): BusinessLanguage[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((entry) => {
    if (!isRecord(entry) || typeof entry.language_code !== "string") {
      return [];
    }

    return [{
      id: typeof entry.id === "number" ? entry.id : 0,
      business_id: typeof entry.business_id === "number" ? entry.business_id : 0,
      language_code: entry.language_code,
      is_default: typeof entry.is_default === "boolean" ? entry.is_default : false,
      display_order: typeof entry.display_order === "number" ? entry.display_order : 0,
      created_at: typeof entry.created_at === "string" ? entry.created_at : "",
      updated_at: typeof entry.updated_at === "string" ? entry.updated_at : "",
    }];
  });
}

export function normalizePublicBusiness(payload: unknown): PublicBusiness {
  const raw = (isRecord(payload) ? payload : {}) as RawPublicBusiness;
  const {
    business_page_enabled,
    address,
    business_languages,
    supported_languages,
    ...rest
  } = raw;

  return {
    ...rest,
    address: normalizeAddress(address),
    page_enabled: raw.page_enabled ?? business_page_enabled,
    business_languages: normalizeBusinessLanguages(business_languages),
    supported_languages: normalizeSupportedLanguages(supported_languages),
  } as PublicBusiness;
}

/**
 * Get public business information by custom URL
 * This endpoint is public and doesn't require authentication
 */
export const getBusinessByCustomUrl = async (
  customUrl: string,
  language?: string,
): Promise<PublicBusiness> => {
  const params = language ? { language } : {};
  const response = await axiosInstance.get(`/business/${customUrl}`, { params });
  return normalizePublicBusiness(response.data);
};

/**
 * Get public business menu by custom URL
 * This endpoint is public and doesn't require authentication
 */
export const getBusinessMenuByCustomUrl = async (customUrl: string, language?: string) => {
  const params = language ? { language } : {};
  const response = await axiosInstance.get<{
    menu?: Partial<Menu>;
    categories: MenuCategory[] | string;
    parsed_categories?: MenuCategory[];
    language?: string;
    offers?: Offer[];
    bundles?: Bundle[];
    item_orderability?: Record<string, Orderability>;
  }>(`/business/${customUrl}/menu`, { params });
  return response.data;
};

/**
 * Allow only hex colors (#rgb / #rrggbb / #rrggbbaa). Business-controlled color
 * settings reach CSS strings here; an unvalidated value like
 * `red; background-image:url(evil)` would inject arbitrary CSS. Fall back to a
 * safe opaque black on any non-conforming input.
 */
/* eslint-disable no-restricted-syntax -- CSS safety fallback hex literal, not a Tailwind class */
export const sanitizeCssColor = (color: string): string => {
  return /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(color)
    ? color
    : "#000000";
};
/* eslint-enable no-restricted-syntax */

/**
 * Generate CSS for background patterns.
 *
 * @deprecated Compatibility wrapper around the shared storefront theme module
 * (`@/lib/storefront/theme`). The SVG artwork now has a single source of truth
 * (the same defs the public page's inline `<pattern>` elements and the editor
 * swatch grid use); this wrapper only re-shapes it into a CSS declaration
 * string for legacy callers. New code should consume `getPatternDataUri` /
 * `getPatternSvgDef` directly. Unknown patterns (incl. "none") return "".
 */
export const generatePatternCSS = (pattern: string, primaryColor: string, opacity: number = 0.1): string => {
  if (!isStorefrontPattern(pattern)) return '';
  // getPatternDataUri sanitizes the color internally (sanitizeCssColor), so a
  // hostile color value can never break out of the data URI into raw CSS.
  return `
      background-image: ${getPatternDataUri(pattern, primaryColor)};
      background-repeat: repeat;
      opacity: ${opacity};
    `;
};
