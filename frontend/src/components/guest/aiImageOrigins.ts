import { getPublicConfig } from "@/config/publicConfig";
import { imageOrigins } from "@/config/imageOrigins";
import { isOwnMediaUrl } from "@/lib/media/ownMedia";

function hostOf(raw: string): string | null {
  try {
    const u = new URL(raw);
    if (u.protocol !== "http:" && u.protocol !== "https:") return null;
    if (!u.hostname) return null;
    return u.hostname.toLowerCase();
  } catch {
    // Menu image strings can be malformed or relative (placeholders, seed
    // data, fork thumbnails). That's expected here — this only computes the
    // AI image-allowlist — so skip silently rather than flooding the console.
    return null;
  }
}

function pathnameMatches(pathname: string, pattern: string): boolean {
  if (pattern === "/**") return pathname.startsWith("/");
  if (pattern.endsWith("/**")) {
    const prefix = pattern.slice(0, -3);
    return pathname === prefix || pathname.startsWith(`${prefix}/`);
  }
  return pathname === pattern;
}

function configuredOrigins(raw: string | undefined): URL[] {
  if (!raw) return [];
  return raw.split(",").flatMap((entry) => {
    const candidate = entry.trim();
    if (!candidate) return [];
    try {
      const url = new URL(
        candidate.includes("://") ? candidate : `https://${candidate}`,
      );
      return url.protocol === "https:" && !url.username && !url.password
        ? [url]
        : [];
    } catch {
      return [];
    }
  });
}

/**
 * Uploads served by this deployment: `/media/...` (relative) or
 * `${PUBLIC_URL}/media/...`. Dot segments are normalized first, so
 * `/media/../api/...` does not qualify.
 */
function isSameOriginMediaUrl(raw: string): boolean {
  const isRelative = raw.startsWith("/") && !raw.startsWith("//");
  const site = getPublicConfig().publicUrl;
  let url: URL;
  let siteUrl: URL;
  try {
    siteUrl = new URL(site);
    url = new URL(raw, siteUrl);
  } catch {
    return false;
  }
  if (!isRelative && raw.slice(0, siteUrl.origin.length + 1) !== `${siteUrl.origin}/`) {
    return false;
  }
  if (url.origin !== siteUrl.origin || url.username || url.password) return false;
  return url.pathname.startsWith("/media/") && url.pathname.length > "/media/".length;
}

/**
 * Strict trust policy for structured assistant entity media. Unlike the
 * legacy host helper below, live menu rows never grant trust: URLs must be
 * this deployment's own /media uploads, match the checked-in origins used by
 * Next Image, or an explicitly configured media origin (MEDIA_ORIGINS).
 */
export function isTrustedEntityImageUrl(
  raw: string,
  configuredEnv: string | undefined = getPublicConfig().mediaOrigins,
): boolean {
  if (!raw || raw !== raw.trim() || /[\\\u0000-\u001f\u007f]/u.test(raw)) {
    return false;
  }
  if (isSameOriginMediaUrl(raw)) return true;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return false;
  }
  if (
    url.protocol !== "https:" ||
    Boolean(url.username) ||
    Boolean(url.password)
  ) {
    return false;
  }

  if (
    imageOrigins.some(
      (origin) =>
        url.protocol === `${origin.protocol}:` &&
        url.hostname.toLowerCase() === origin.hostname.toLowerCase() &&
        !url.port &&
        pathnameMatches(url.pathname, origin.pathname),
    )
  ) {
    return true;
  }

  return configuredOrigins(configuredEnv).some(
    (origin) =>
      url.protocol === origin.protocol &&
      url.hostname.toLowerCase() === origin.hostname.toLowerCase() &&
      url.port === origin.port,
  );
}

function collectImageStrings(menuData: unknown): string[] {
  const out: string[] = [];
  if (!Array.isArray(menuData)) return out;
  for (const cat of menuData) {
    const items = (cat as { items?: unknown[] })?.items;
    if (!Array.isArray(items)) continue;
    for (const item of items) {
      const it = item as { image?: unknown; images?: unknown };
      if (typeof it.image === "string") out.push(it.image);
      if (Array.isArray(it.images)) {
        for (const img of it.images) if (typeof img === "string") out.push(img);
      }
    }
  }
  return out;
}

export function computeAllowedImageHosts(
  menuData: unknown,
  fallbackEnv: string | undefined = getPublicConfig().mediaOrigins,
): Set<string> {
  const hosts = new Set<string>();
  for (const s of collectImageStrings(menuData)) {
    const h = hostOf(s);
    if (h) hosts.add(h);
  }
  if (fallbackEnv) {
    for (const entry of fallbackEnv.split(",")) {
      const trimmed = entry.trim();
      if (!trimmed) continue;
      const h = hostOf(trimmed) ?? hostOf(`https://${trimmed}`);
      if (h) hosts.add(h);
    }
  }
  return hosts;
}

export function isAllowedImageUrl(
  url: string,
  allowedHosts: Set<string>,
): boolean {
  // This deployment's own uploads (/media/<key>) carry no bucket host.
  if (isOwnMediaUrl(url)) return true;
  const h = hostOf(url);
  return h !== null && allowedHosts.has(h);
}
