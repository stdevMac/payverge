import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  generateMarketingCaption,
  MarketingCaptionUnavailableError,
  type CampaignSuggestion,
  type MarketingCreativeProfile,
  type MarketingTone,
} from "@/api/marketing";
import type { Business } from "@/api/business";
import { guestLocales, type GuestLocale } from "@/i18n/localeRegistry";
import {
  canonicalMarketingCaptionLocale,
  composeLocalizedFallbackCaption,
  happyHourLiveOfferName,
  type MarketingCaptionLocale,
} from "../postContent";

type CaptionVariantTone = MarketingTone;
export type CaptionToneChoice = CaptionVariantTone;
type CaptionSource = "fallback" | "ai" | "unavailable";

const EMPTY_PROFILE: MarketingCreativeProfile = {
  audience: "",
  voice: "",
  visual_mood: "",
  cta_style: "",
  hashtag_behavior: "",
  avoid_phrases: [],
  default_language: "",
  default_tone: "",
};

function normalizedProfile(
  profile?: MarketingCreativeProfile,
): MarketingCreativeProfile {
  const defaultLanguage = profile?.default_language?.trim() ?? "";
  return {
    audience: profile?.audience?.trim() ?? "",
    voice: profile?.voice?.trim() ?? "",
    visual_mood: profile?.visual_mood ?? "",
    cta_style: profile?.cta_style ?? "",
    hashtag_behavior: profile?.hashtag_behavior ?? "",
    avoid_phrases: (profile?.avoid_phrases ?? [])
      .map((phrase) => phrase.trim())
      .filter(Boolean),
    default_language: defaultLanguage
      ? canonicalMarketingRequestLocale(defaultLanguage)
      : "",
    default_tone: profile?.default_tone ?? "",
  };
}

/** Stable cache identity for every creative-profile input used by the server. */
export function creativeProfileFingerprint(
  profile?: MarketingCreativeProfile,
): string {
  return JSON.stringify(normalizedProfile(profile ?? EMPTY_PROFILE));
}

/** Stable cache identity for profile fields that affect caption generation. */
export function captionProfileFingerprint(
  profile?: MarketingCreativeProfile,
): string {
  const normalized = normalizedProfile(profile ?? EMPTY_PROFILE);
  return JSON.stringify({
    audience: normalized.audience,
    voice: normalized.voice,
    cta_style: normalized.cta_style,
    hashtag_behavior: normalized.hashtag_behavior,
    avoid_phrases: normalized.avoid_phrases,
  });
}

const MARKETING_SOCIAL_PLATFORMS = [
  "instagram",
  "tiktok",
  "twitter",
  "x",
  "facebook",
] as const;
const MARKETING_SOCIAL_HOSTS = new Set([
  "instagram.com",
  "www.instagram.com",
  "tiktok.com",
  "www.tiktok.com",
  "twitter.com",
  "www.twitter.com",
  "x.com",
  "www.x.com",
  "facebook.com",
  "www.facebook.com",
]);
const MARKETING_SOCIAL_HANDLE = /^[A-Za-z0-9._-]{1,30}$/;
const MARKETING_SOCIAL_URL_SHAPE = /^https?:\/\/([^/?#]+)(?:[/?#]|$)/i;
const MARKETING_SOCIAL_URL_FORBIDDEN = /[\\\u0000-\u001f\u007f]/;

function hasStrictMarketingSocialURLSyntax(value: string): boolean {
  if (MARKETING_SOCIAL_URL_FORBIDDEN.test(value)) return false;
  const authority = MARKETING_SOCIAL_URL_SHAPE.exec(value)?.[1];
  return !!authority && !/\s/.test(authority);
}

/** Match the backend's selected, validated social handle exactly. */
function normalizedMarketingSocialHandle(value?: string): string {
  let selected = value?.trim() ?? "";
  if (!selected) return "";
  if (selected.startsWith("{")) {
    let links: Record<string, unknown>;
    try {
      links = JSON.parse(selected) as Record<string, unknown>;
    } catch {
      return "";
    }
    selected = "";
    for (const platform of MARKETING_SOCIAL_PLATFORMS) {
      const candidate = links[platform];
      if (typeof candidate === "string" && candidate.trim()) {
        selected = candidate;
        break;
      }
    }
    if (!selected) return "";
  }

  selected = selected.trim();
  if (selected.includes("://")) {
    if (!hasStrictMarketingSocialURLSyntax(selected)) return "";
    let parsed: URL;
    try {
      parsed = new URL(selected);
    } catch {
      return "";
    }
    if (
      (parsed.protocol !== "http:" && parsed.protocol !== "https:") ||
      !MARKETING_SOCIAL_HOSTS.has(parsed.hostname.toLowerCase())
    ) {
      return "";
    }
    const path = parsed.pathname.replace(/^\/+|\/+$/g, "");
    if (!path) return "";
    try {
      selected = decodeURIComponent(path.split("/")[0]);
    } catch {
      return "";
    }
  } else if (selected.includes("/")) {
    return "";
  }

  selected = selected.replace(/^[@/ ]+|[@/ ]+$/g, "");
  return MARKETING_SOCIAL_HANDLE.test(selected) ? `@${selected}` : "";
}

/** Stable cache identity for server-owned business facts used by captions. */
export function businessGroundingFingerprint(business: Business): string {
  return JSON.stringify({
    name: business.name?.trim() ?? "",
    business_type: business.business_type?.trim() ?? "",
    description: business.description?.trim() ?? "",
    city: business.address?.city?.trim() ?? "",
    social_handle: normalizedMarketingSocialHandle(business.social_media),
  });
}

export function marketingCaptionQueryKey(
  businessId: string | number,
  suggestionId: string,
  locale: GuestLocale,
  tone: CaptionToneChoice,
  creativeProfile?: MarketingCreativeProfile,
  requestFingerprint = "",
  businessFingerprint = "",
) {
  return [
    "business",
    String(businessId),
    "marketing",
    "caption",
    suggestionId,
    locale,
    tone,
    captionProfileFingerprint(creativeProfile),
    requestFingerprint,
    businessFingerprint,
  ] as const;
}

const guestLocaleByLower = new Map<string, GuestLocale>(
  guestLocales.map((locale) => [locale.toLowerCase(), locale]),
);

/** Preserve any backend-supported guest locale while normalizing casing. */
export function canonicalMarketingRequestLocale(locale?: string): GuestLocale {
  const normalized = (locale || "en").trim().replace(/_/g, "-").toLowerCase();
  return guestLocaleByLower.get(normalized) ?? "en";
}

/** Never use promotional titles as factual caption subjects. */
export function resolveCaptionSubject(
  suggestion: CampaignSuggestion,
  business: Business,
): string {
  const subject =
    (suggestion.play === "happy_hour"
      ? happyHourLiveOfferName(suggestion)
      : suggestion.target_name?.trim()) ||
    business.name?.trim() ||
    "";
  const runes = Array.from(subject);
  if (runes.length <= 160) return subject;
  return `${runes.slice(0, 159).join("").trimEnd()}…`;
}

/** Stable identity for every client fact sent to caption generation. */
export function captionRequestFingerprint(
  suggestion: CampaignSuggestion,
  business: Business,
): string {
  return JSON.stringify({
    subject: resolveCaptionSubject(suggestion, business),
    play: suggestion.play,
    angle: suggestion.copy_angle?.trim() ?? "",
    why_data: suggestion.why_data?.trim() ?? "",
  });
}

export interface CaptionResource {
  caption: string;
  source: CaptionSource;
  tone: CaptionToneChoice;
  generating: boolean;
  error: Error | null;
  retry: () => void;
  selectTone: (tone: CaptionToneChoice) => void;
  observeRef: (node: HTMLElement | null) => void;
  canGenerate: boolean;
}

interface UseSuggestionCaptionOptions {
  businessId: string | number | undefined;
  business: Business;
  suggestion: CampaignSuggestion;
  locale: string;
  creativeProfile?: MarketingCreativeProfile;
  enabled: boolean;
  /** The first card fetches immediately; later cards wait for near-viewport observation. */
  priority?: boolean;
}

function asError(value: unknown): Error | null {
  if (!value) return null;
  return value instanceof Error ? value : new Error(String(value));
}

/** One progressive, independently retryable caption resource for one feed card. */
export function useSuggestionCaption({
  businessId,
  business,
  suggestion,
  locale,
  creativeProfile,
  enabled,
  priority = false,
}: UseSuggestionCaptionOptions): CaptionResource {
  const id = businessId == null ? "" : String(businessId);
  const profile = React.useMemo(
    () => normalizedProfile(creativeProfile),
    [creativeProfile],
  );
  const requestLocale = canonicalMarketingRequestLocale(
    profile.default_language || locale,
  );
  const fallbackLocale: MarketingCaptionLocale =
    canonicalMarketingCaptionLocale(requestLocale);
  const profileDefaultTone: CaptionToneChoice = profile.default_tone || "warm";
  const [toneOverride, setToneOverride] =
    React.useState<CaptionToneChoice | null>(null);
  const tone = toneOverride ?? profileDefaultTone;
  const [target, setTarget] = React.useState<HTMLElement | null>(null);
  const [nearViewport, setNearViewport] = React.useState(priority);
  const subject = resolveCaptionSubject(suggestion, business);
  const requestFingerprint = captionRequestFingerprint(suggestion, business);
  const businessFingerprint = businessGroundingFingerprint(business);

  React.useEffect(() => {
    if (priority) setNearViewport(true);
  }, [priority]);

  const observeRef = React.useCallback((node: HTMLElement | null) => {
    setTarget(node);
  }, []);

  React.useEffect(() => {
    if (!enabled || priority || nearViewport || !target) return;
    if (typeof IntersectionObserver === "undefined") {
      setNearViewport(true);
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        setNearViewport(true);
        observer.disconnect();
      },
      { rootMargin: "320px 0px", threshold: 0.01 },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [enabled, nearViewport, priority, target]);

  const canFetch = enabled && !!id && !!subject && (priority || nearViewport);
  const query = useQuery({
    queryKey: marketingCaptionQueryKey(
      id,
      suggestion.id,
      requestLocale,
      tone,
      profile,
      requestFingerprint,
      businessFingerprint,
    ),
    queryFn: ({ signal }) =>
      generateMarketingCaption(
        id,
        {
          item_name: subject,
          play: suggestion.play,
          tone,
          language: requestLocale,
          angle: suggestion.copy_angle?.trim() ?? "",
          why_data: suggestion.why_data?.trim() ?? "",
        },
        signal,
      ),
    enabled: canFetch,
    staleTime: 30 * 60 * 1000,
    retry: false,
  });

  const fallback = React.useMemo(
    () =>
      composeLocalizedFallbackCaption({
        suggestion,
        business,
        locale: fallbackLocale,
      }),
    [business, fallbackLocale, suggestion],
  );
  const aiCaption = enabled ? query.data?.trim() : "";
  const unavailable =
    query.error instanceof MarketingCaptionUnavailableError ||
    (query.error instanceof Error &&
      query.error.message === "item_unavailable");

  const retry = React.useCallback(() => {
    if (canFetch) void query.refetch();
  }, [canFetch, query]);

  const selectTone = React.useCallback((nextTone: CaptionToneChoice) => {
    setToneOverride(nextTone);
  }, []);

  return {
    caption: unavailable ? "" : aiCaption || fallback,
    source: unavailable ? "unavailable" : aiCaption ? "ai" : "fallback",
    tone,
    generating: enabled && query.isFetching,
    error: enabled ? asError(query.error) : null,
    retry,
    selectTone,
    observeRef,
    canGenerate: enabled && !!subject,
  };
}
