/**
 * Browser-readable acceptance for paid Marketing AI images.
 *
 * Success is only the combination of: the asset loads, it decodes, and the
 * durable draft (URL + provenance + dimensions) is persisted. A provider URL
 * that the browser cannot read (CORS / missing ACAO) is rewritten onto the
 * same-origin media proxy before we announce "Foto generada con IA".
 */

import type { CampaignSuggestion, MarketingImageSource } from "@/api/marketing";
import { getPublicConfig } from "@/config/publicConfig";

/** Stock-photo host every deployment may proxy. */
const STATIC_PROXYABLE_IMAGE_HOSTS = ["images.unsplash.com"] as const;

/**
 * Hosts the same-origin media proxy may fetch: the static stock host plus the
 * deployment's own object-storage/CDN hosts from MEDIA_ORIGINS (https only).
 * Same-origin `/media/...` uploads never need the proxy.
 */
export function proxyableImageHosts(
  mediaOrigins: string = getPublicConfig().mediaOrigins,
): string[] {
  const hosts = new Set<string>(STATIC_PROXYABLE_IMAGE_HOSTS);
  for (const entry of mediaOrigins.split(",")) {
    const candidate = entry.trim();
    if (!candidate) continue;
    try {
      const url = new URL(
        candidate.includes("://") ? candidate : `https://${candidate}`,
      );
      if (url.protocol === "https:" && !url.username && !url.password && !url.port) {
        hosts.add(url.hostname.toLowerCase());
      }
    } catch {
      // malformed entry: ignored, never widens the allowlist
    }
  }
  return [...hosts];
}

const MARKETING_MEDIA_PROXY_PATH = "/api/marketing-media";
const GENERATED_IMAGE_DRAFT_PREFIX =
  "payverge:marketing:generated-image:";

type ImagePreflightStatus =
  | "readable"
  | "cors_blocked"
  | "decode_failed";

export interface ImagePreflightResult {
  status: ImagePreflightStatus;
  url: string;
  width?: number;
  height?: number;
}

export interface GeneratedImageDraft {
  suggestionId: string;
  businessId: string;
  url: string;
  providerUrl: string;
  imageSource: Extract<MarketingImageSource, "generated">;
  width: number;
  height: number;
  status: "ready";
  persistedAt: number;
}

export type GenerationRetryWarning = "none" | "already_ready" | "failed_delivery";

export type AcceptGeneratedImageResult =
  | { ok: true; draft: GeneratedImageDraft }
  | {
      ok: false;
      reason: "decode_failed" | "unreadable" | "persist_failed";
    };

export interface AcceptGeneratedImageArgs {
  providerUrl: string;
  businessId: string;
  suggestionId: string;
  preflight: (url: string) => Promise<ImagePreflightResult>;
  persist: (draft: GeneratedImageDraft) => void | Promise<void>;
}

function generatedImageDraftKey(
  businessId: string,
  suggestionId: string,
): string {
  return `${GENERATED_IMAGE_DRAFT_PREFIX}${businessId}:${suggestionId}`;
}

export function isProxyableImageUrl(raw: string): boolean {
  try {
    const parsed = new URL(raw);
    if (parsed.protocol !== "https:") return false;
    if (parsed.username || parsed.password) return false;
    if (parsed.port) return false;
    const host = parsed.hostname.toLowerCase();
    return proxyableImageHosts().includes(host);
  } catch {
    return false;
  }
}

export function toSameOriginMediaUrl(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith(`${MARKETING_MEDIA_PROXY_PATH}?`)) return trimmed;
  if (!isProxyableImageUrl(trimmed)) return null;
  return `${MARKETING_MEDIA_PROXY_PATH}?u=${encodeURIComponent(trimmed)}`;
}

export function browserReadablePhotoUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  return toSameOriginMediaUrl(trimmed) ?? trimmed;
}

export function generationRetryWarning(input: {
  hasReadyGeneratedDraft: boolean;
  lastAcceptanceFailed: boolean;
  imageSource: string;
}): GenerationRetryWarning {
  if (input.hasReadyGeneratedDraft || input.imageSource === "generated") {
    return "already_ready";
  }
  if (input.lastAcceptanceFailed) return "failed_delivery";
  return "none";
}

type PreflightImage = {
  naturalWidth?: number;
  naturalHeight?: number;
  width?: number;
  height?: number;
  decode?: () => Promise<void>;
};

export async function preflightBrowserImage(
  url: string,
  load: (url: string) => Promise<PreflightImage | null>,
): Promise<ImagePreflightResult> {
  let image: PreflightImage | null;
  try {
    image = await load(url);
  } catch {
    return { status: "cors_blocked", url };
  }
  if (!image) return { status: "cors_blocked", url };
  if (typeof image.decode === "function") {
    try {
      await image.decode();
    } catch {
      return { status: "decode_failed", url };
    }
  }
  const width = image.naturalWidth || image.width || 0;
  const height = image.naturalHeight || image.height || 0;
  if (width <= 0 || height <= 0) {
    return { status: "decode_failed", url };
  }
  return { status: "readable", url, width, height };
}

export async function acceptGeneratedMarketingImage(
  args: AcceptGeneratedImageArgs,
): Promise<AcceptGeneratedImageResult> {
  const providerUrl = args.providerUrl.trim();
  if (!providerUrl) return { ok: false, reason: "unreadable" };

  let result = await args.preflight(providerUrl);
  let url = providerUrl;

  if (result.status === "cors_blocked") {
    const proxied = toSameOriginMediaUrl(providerUrl);
    if (proxied && proxied !== providerUrl) {
      url = proxied;
      result = await args.preflight(url);
    }
  }

  if (result.status === "decode_failed") {
    return { ok: false, reason: "decode_failed" };
  }
  if (result.status !== "readable" || !result.width || !result.height) {
    return { ok: false, reason: "unreadable" };
  }

  const draft: GeneratedImageDraft = {
    suggestionId: args.suggestionId,
    businessId: args.businessId,
    url,
    providerUrl,
    imageSource: "generated",
    width: result.width,
    height: result.height,
    status: "ready",
    persistedAt: Date.now(),
  };

  try {
    await args.persist(draft);
  } catch {
    return { ok: false, reason: "persist_failed" };
  }
  return { ok: true, draft };
}

function parseDraft(raw: string | null): GeneratedImageDraft | null {
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as Partial<GeneratedImageDraft>;
    if (
      typeof parsed.suggestionId !== "string" ||
      typeof parsed.businessId !== "string" ||
      typeof parsed.url !== "string" ||
      typeof parsed.providerUrl !== "string" ||
      parsed.imageSource !== "generated" ||
      parsed.status !== "ready" ||
      typeof parsed.width !== "number" ||
      typeof parsed.height !== "number" ||
      parsed.width <= 0 ||
      parsed.height <= 0
    ) {
      return null;
    }
    return {
      suggestionId: parsed.suggestionId,
      businessId: parsed.businessId,
      url: parsed.url,
      providerUrl: parsed.providerUrl,
      imageSource: "generated",
      width: parsed.width,
      height: parsed.height,
      status: "ready",
      persistedAt:
        typeof parsed.persistedAt === "number" ? parsed.persistedAt : 0,
    };
  } catch {
    return null;
  }
}

export function persistGeneratedImageDraft(draft: GeneratedImageDraft): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(
    generatedImageDraftKey(draft.businessId, draft.suggestionId),
    JSON.stringify(draft),
  );
}

export function loadGeneratedImageDraft(
  businessId: string,
  suggestionId: string,
): GeneratedImageDraft | null {
  if (typeof window === "undefined") return null;
  if (!businessId || !suggestionId) return null;
  return parseDraft(
    window.localStorage.getItem(
      generatedImageDraftKey(businessId, suggestionId),
    ),
  );
}

export function applyGeneratedImageDrafts(
  suggestions: CampaignSuggestion[],
  businessId: string,
): CampaignSuggestion[] {
  if (!suggestions.length) return suggestions;
  return suggestions.map((suggestion) => {
    const draft = loadGeneratedImageDraft(businessId, suggestion.id);
    if (!draft) return suggestion;
    return {
      ...suggestion,
      image_url: draft.url,
      image_source: "generated",
    };
  });
}
