import type { PublicBusiness } from "@/api/publicBusiness";
import { imageCspOrigins } from "@/config/imageCspOrigins";
import { getPublicConfig, getSiteUrl } from "@/config/publicConfig";
import { isOwnMediaUrl, isRelativeOwnMediaUrl } from "@/lib/media/ownMedia";
import { getServerApiUrl } from "@/lib/serverApiUrl";
import { parseMediaOrigins } from "@/lib/security/csp";
import { parseBannerImages } from "@/utils/businessDataParsers";

/** Bytes the OG card will inline. Larger bodies are dropped, not buffered. */
const MAX_OG_IMAGE_BYTES = 5_000_000;

/**
 * Data helpers for the dynamic storefront OG card
 * (`app/b/[customUrl]/opengraph-image.tsx`). Kept separate from the route file
 * because Next only allows a fixed set of exports from image conventions —
 * and so the pure parts stay unit-testable without rendering ImageResponse.
 */

export interface OgCardModel {
  /** Business name, or the Payverge fallback wordmark for the default card. */
  name: string;
  /** Single-letter initial for the no-logo tile. */
  initial: string;
  /** Truncated description line (null when empty). */
  description: string | null;
  /** Google rating, when available. */
  rating: { value: number; count: number } | null;
  /** Footer path line, e.g. "pos.example.com/b/aurora". */
  slugPath: string;
  bannerUrl: string | null;
  logoUrl: string | null;
}

/** Collapse whitespace and clip to `max` chars on a word boundary. */
export function truncateOgText(text: string, max = 120): string {
  const collapsed = text.replace(/\s+/g, " ").trim();
  if (collapsed.length <= max) return collapsed;
  const sliced = collapsed.slice(0, max);
  const lastSpace = sliced.lastIndexOf(" ");
  return `${sliced.slice(0, lastSpace > 40 ? lastSpace : max).trimEnd()}…`;
}

export function buildOgCardModel(input: {
  business: PublicBusiness | null;
  customUrl: string;
  baseUrl: string;
  googleRating?: { ratingValue: number; reviewCount: number } | null;
}): OgCardModel {
  const { business, customUrl, baseUrl, googleRating } = input;
  const host = baseUrl.replace(/^https?:\/\//, "").replace(/\/+$/, "");
  const name = business?.name?.trim() || "Payverge";
  const description = business?.description?.trim()
    ? truncateOgText(business.description, 120)
    : null;
  return {
    name,
    initial: (name[0] || "P").toUpperCase(),
    description,
    rating:
      googleRating && googleRating.ratingValue > 0
        ? { value: googleRating.ratingValue, count: googleRating.reviewCount }
        : null,
    slugPath: `${host}/b/${customUrl}`,
    bannerUrl: business ? parseBannerImages(business.banner_images)[0] ?? null : null,
    logoUrl: business?.logo?.trim() ? business.logo : null,
  };
}

/**
 * Origins the server may fetch for a stored absolute image URL.
 * `allowedOrigins`, when passed, replaces the config list (tests). Otherwise
 * the list is MEDIA_ORIGINS, the static image hosts, and — only when `url`
 * itself is own-media — the site origin.
 */
function ogImageAllowedOrigins(
  url: string,
  allowedOrigins: readonly string[] | undefined,
): Set<string> {
  if (allowedOrigins) return new Set(allowedOrigins);
  const origins = new Set<string>([
    ...parseMediaOrigins(getPublicConfig().mediaOrigins),
    ...imageCspOrigins(),
  ]);
  const siteOrigin = getSiteUrl();
  if (siteOrigin && isOwnMediaUrl(url, siteOrigin)) {
    try {
      origins.add(new URL(siteOrigin).origin);
    } catch {
      // A site URL that is not an origin adds nothing.
    }
  }
  return origins;
}

/**
 * The URL the Next server may fetch for a stored image.
 *
 * Relative own-media (`/media/<key>`) resolves against the server-side API
 * origin (INTERNAL_API_URL in Docker: the backend, which serves /media).
 * Null when that base is missing or not absolute http(s).
 *
 * Absolute URLs are returned only when they are credential-free https and
 * their origin is allowlisted. Link-local hosts, other origins, URL
 * credentials, and non-http(s) schemes are null so nothing fetches them.
 */
export function resolveImageFetchUrl(
  url: string,
  serverApiUrl: string = getServerApiUrl(),
  allowedOrigins?: readonly string[],
): string | null {
  if (isRelativeOwnMediaUrl(url)) {
    try {
      const base = new URL(serverApiUrl);
      if (base.protocol !== "http:" && base.protocol !== "https:") return null;
      return new URL(url, base.origin).toString();
    } catch {
      return null;
    }
  }

  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }
  if (parsed.protocol !== "https:") return null;
  if (parsed.username || parsed.password) return null;
  if (!ogImageAllowedOrigins(url, allowedOrigins).has(parsed.origin)) {
    return null;
  }
  return parsed.href;
}

/**
 * Read at most MAX_OG_IMAGE_BYTES. A missing body falls back to arrayBuffer
 * with the same cap. Over the cap, the reader is cancelled and `controller`
 * is aborted.
 */
async function readBoundedImageBody(
  res: Response,
  controller: AbortController,
): Promise<Uint8Array | null> {
  if (!res.body) {
    const buffer = await res.arrayBuffer();
    if (buffer.byteLength === 0 || buffer.byteLength > MAX_OG_IMAGE_BYTES) {
      if (buffer.byteLength > MAX_OG_IMAGE_BYTES) controller.abort();
      return null;
    }
    return new Uint8Array(buffer);
  }

  const reader = res.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value || value.byteLength === 0) continue;
      total += value.byteLength;
      if (total > MAX_OG_IMAGE_BYTES) {
        try {
          await reader.cancel();
        } finally {
          controller.abort();
        }
        return null;
      }
      chunks.push(value);
    }
  } catch {
    controller.abort();
    try {
      await reader.cancel();
    } catch {
      // The stream is already closed.
    }
    return null;
  }

  if (total === 0) return null;
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

/**
 * Fetch a remote image and inline it as a base64 data URL for satori.
 * Disallowed URLs, redirects, non-images, and bodies over 5MB fail softly
 * (null) so a broken or hostile link degrades the card to the brand gradient
 * instead of failing the whole image response.
 */
export async function fetchImageDataUrl(url: string): Promise<string | null> {
  const target = resolveImageFetchUrl(url);
  if (!target) return null;
  const controller = new AbortController();
  try {
    const res = await fetch(target, {
      cache: "no-store",
      redirect: "manual",
      signal: controller.signal,
    });
    if (!res.ok || res.status < 200 || res.status >= 300) return null;
    const rawLength = res.headers.get("content-length");
    if (rawLength) {
      const declared = Number(rawLength);
      if (Number.isFinite(declared) && declared > MAX_OG_IMAGE_BYTES) {
        controller.abort();
        try {
          await res.body?.cancel();
        } catch {
          // Already closing.
        }
        return null;
      }
    }
    const contentType = (res.headers.get("content-type") || "")
      .split(";")[0]
      .trim();
    if (!contentType.startsWith("image/")) return null;
    const bytes = await readBoundedImageBody(res, controller);
    if (!bytes) return null;
    return `data:${contentType};base64,${Buffer.from(bytes).toString("base64")}`;
  } catch {
    return null;
  }
}

export interface OgFonts {
  fonts: { name: string; data: ArrayBuffer; weight: 400; style: "normal" }[];
  serif: string;
  sans: string;
}

/**
 * Legacy-UA Google Fonts fetch → TTF bytes. Satori (bundled in next/og) parses
 * TTF/OTF/WOFF but NOT woff2, which is what a modern UA gets served — so the
 * css2 request goes out with an old Android UA and we parse the .ttf URL out.
 */
async function fetchGoogleFontBuffer(
  family: string,
  weight: number,
): Promise<ArrayBuffer | null> {
  try {
    const cssRes = await fetch(
      `https://fonts.googleapis.com/css2?family=${encodeURIComponent(family)}:wght@${weight}&display=swap`,
      {
        headers: {
          "User-Agent":
            "Mozilla/5.0 (Linux; Android 4.4.2; Nexus 4 Build/KOT49H)",
        },
        cache: "force-cache",
      },
    );
    if (!cssRes.ok) return null;
    const css = await cssRes.text();
    const url = /src:\s*url\((https:\/\/fonts\.gstatic\.com\/[^)]+)\)/.exec(
      css,
    )?.[1];
    if (!url) return null;
    const fontRes = await fetch(url, { cache: "force-cache" });
    if (!fontRes.ok) return null;
    return await fontRes.arrayBuffer();
  } catch {
    return null;
  }
}

/**
 * Brand fonts for the OG card (DM Serif Display for the business name, DM Sans
 * for supporting lines — matching the app's next/font pairing). Returns null
 * on any failure; the caller then omits the `fonts` option and next/og falls
 * back to its bundled default font, so a font outage never breaks the image.
 */
export async function loadOgFonts(): Promise<OgFonts | null> {
  const [serif, sans] = await Promise.all([
    fetchGoogleFontBuffer("DM Serif Display", 400),
    fetchGoogleFontBuffer("DM Sans", 400),
  ]);
  if (!serif || !sans) return null;
  return {
    fonts: [
      { name: "DM Serif Display", data: serif, weight: 400, style: "normal" },
      { name: "DM Sans", data: sans, weight: 400, style: "normal" },
    ],
    serif: "DM Serif Display",
    sans: "DM Sans",
  };
}
